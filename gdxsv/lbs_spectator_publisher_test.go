package main

import (
	"net"
	"testing"
	"time"

	"gdxsv/gdxsv/proto"
	pb "google.golang.org/protobuf/proto"
)

type spectatorPublisherTest struct {
	t       *testing.T
	session *SpectatorSession
	server  *net.UDPConn
	peers   [2]*net.UDPConn
}

func newSpectatorPublisherTest(t *testing.T) *spectatorPublisherTest {
	t.Helper()
	listen := func() *net.UDPConn {
		conn, err := net.ListenUDP("udp4", testAddr(0))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	r := newTestSpectatorRegistry()
	saved := spectatorRegistry
	spectatorRegistry = r
	t.Cleanup(func() { spectatorRegistry = saved })
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s
	return &spectatorPublisherTest{t, s, listen(), [2]*net.UDPConn{listen(), listen()}}
}

func (h *spectatorPublisherTest) ack(peer int) *proto.SpectatorInputAck {
	h.t.Helper()
	conn := h.peers[peer]
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		h.t.Fatal(err)
	}
	buf := make([]byte, 256)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		h.t.Fatal(err)
	}
	var packet proto.Packet
	if err := pb.Unmarshal(buf[:n], &packet); err != nil {
		h.t.Fatal(err)
	}
	assertEq(h.t, proto.MessageType_SpectatorInputAckType, packet.Type)
	assertEq(h.t, h.session.battleCode, packet.GetSpectatorInputAckData().GetBattleCode())
	return packet.GetSpectatorInputAckData()
}

func (h *spectatorPublisherTest) inputs(peer int, frame int32, values ...uint64) int32 {
	h.t.Helper()
	handleSpectatorInputPush(h.server, h.peers[peer].LocalAddr().(*net.UDPAddr), &proto.SpectatorInputPush{
		BattleCode: h.session.battleCode, SessionId: h.session.sessionID,
		StartFrame: frame, Inputs: values,
	})
	return h.ack(peer).AckFrame
}

func (h *spectatorPublisherTest) start(peer int, frame int32, seed uint64) {
	h.t.Helper()
	handleSpectatorRoundEvent(h.server, h.peers[peer].LocalAddr().(*net.UDPAddr), &proto.SpectatorRoundEvent{
		BattleCode: h.session.battleCode, SessionId: h.session.sessionID,
		Frame: frame, RandomValue: seed,
	})
	assertEq(h.t, []int32{frame}, h.ack(peer).RoundEventAck)
}

func TestSpectatorUDP_FollowsFirstPublisher(t *testing.T) {
	h := newSpectatorPublisherTest(t)
	h.start(0, 0, 111)
	h.start(1, 0, 111)
	// P2 recorded the KeyMsg1 between rounds (0) that P1 skipped, so round 2
	// starts one input later in its own indexes.
	assertEq(t, int32(5), h.inputs(0, 0, 10, 11, 12, 20, 21))
	assertEq(t, int32(6), h.inputs(1, 0, 10, 11, 12, 0, 20, 21))
	h.start(1, 4, 222)
	h.start(0, 3, 222)
	// Published at once, in P1's indexes, whatever P2 reports or reports first.
	assertEq(t, []uint64{10, 11, 12, 20, 21}, h.session.log.Inputs)
	assertEq(t, []int32{0, 3}, h.session.log.StartMsgIndexes)
	assertEq(t, []uint64{111, 222}, h.session.log.StartMsgRandoms)
	// The others are acknowledged for all they sent, so they stop resending.
	assertEq(t, int32(6), h.inputs(1, 4, 20, 21))
	assertEq(t, int32(5), h.inputs(0, 3, 20, 21))
}

// The case that desynced live spectators: a longer previous round from one
// peer and an earlier round marker from another can no longer meet.
func TestSpectatorUDP_OtherParticipantsCannotMoveRoundStart(t *testing.T) {
	h := newSpectatorPublisherTest(t)
	h.start(1, 0, 111)
	h.inputs(1, 0, 10, 11, 12, 0) // the publisher has the longer round
	h.inputs(0, 0, 10, 11, 12)
	h.start(0, 3, 222)
	h.start(1, 4, 222)
	h.inputs(0, 3, 20, 21)
	h.inputs(1, 4, 20, 21)
	assertEq(t, []int32{0, 4}, h.session.log.StartMsgIndexes)
	assertEq(t, []uint64{10, 11, 12, 0, 20, 21}, h.session.log.Inputs)
}

func TestSpectatorUDP_InvalidInputDoesNotSelectPublisher(t *testing.T) {
	h := newSpectatorPublisherTest(t)
	assertEq(t, int32(0), h.inputs(0, -1, 99))
	assertEq(t, "", h.session.publisher)
	h.inputs(1, 0, 10)
	assertEq(t, h.peers[1].LocalAddr().String(), h.session.publisher)
}

func TestSpectatorUDP_RoundResultsStillUseAllParticipants(t *testing.T) {
	h := newSpectatorPublisherTest(t)
	for peer, outcome := range []int32{1, 2} {
		handleSpectatorRoundResult(h.server, h.peers[peer].LocalAddr().(*net.UDPAddr), &proto.SpectatorRoundResult{
			BattleCode: h.session.battleCode, SessionId: h.session.sessionID,
			RoundIndex: 0, Round: &proto.BattleLogRound{WinTeam: outcome, UsedMs: []int32{1, 2, 3, 4}},
		})
		assertEq(t, []int32{0}, h.ack(peer).RoundResultAck)
	}
	assertEq(t, "", h.session.publisher)
	h.start(0, 0, 111)
	assertEq(t, int32(-1), h.session.log.RoundData[0].WinTeam)
}

func TestSpectatorPublisher_LateInputsAfterCloseWithinGrace(t *testing.T) {
	s := newTestSpectatorSession()
	now := time.Now()
	assertEq(t, true, s.pushParticipantRoundEventAt("P1", 0, 111, now))
	s.pushParticipantInputsAt("P1", 0, []uint64{10, 11}, now)
	s.Close("game_end", -1)
	s.closedAt = now
	sub := &downlinkSubscriber{sentHeader: true, ackedFrame: 2, ackedRoundStateVersion: s.roundStateVersion}

	// Another participant's report closed the session before the publisher
	// drained. Its tail still lands, and spectators are not told to stop yet.
	s.pushParticipantInputsAt("P1", 2, []uint64{12}, now.Add(time.Second))
	assertEq(t, []uint64{10, 11, 12}, s.log.Inputs)
	assertEq(t, false, s.publisherFinishedLocked(now.Add(time.Second)))
	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, "", push.CloseReason)

	// After the grace nothing more is accepted, and close goes out.
	ack, advanced := s.pushParticipantInputsAt("P1", 3, []uint64{13}, now.Add(spectatorPublisherCloseGrace))
	assertEq(t, int32(3), ack)
	assertEq(t, false, advanced)
	assertEq(t, false, s.pushParticipantRoundEventAt("P1", 3, 222, now.Add(spectatorPublisherCloseGrace)))
	assertEq(t, true, s.pushParticipantRoundEventAt("P1", 0, 111, now.Add(spectatorPublisherCloseGrace)))
	assertEq(t, true, s.publisherFinishedLocked(now.Add(spectatorPublisherCloseGrace)))
}

func TestPickSpectatorUplink(t *testing.T) {
	peer := func(rtt string) *LbsPeer {
		return &LbsPeer{PlatformInfo: map[string]string{"asia-northeast1": rtt, "asia-east2": "1"}}
	}
	for _, tc := range []struct {
		name string
		rtts []string
		want int
	}{
		{"nearest", []string{"40", "12", "90", "12"}, 1},
		{"unmeasured last", []string{"", "0", "x", "200"}, 3},
		{"none measured", []string{"", "", "", ""}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var participants []*LbsPeer
			for _, rtt := range tc.rtts {
				participants = append(participants, peer(rtt))
			}
			assertEq(t, tc.want, pickSpectatorUplink(participants, "asia-northeast1"))
		})
	}
}

func TestMakeP2PMatchingMsg_AsksOneParticipantForUplink(t *testing.T) {
	saved := spectatorRegistry
	spectatorRegistry = newTestSpectatorRegistry()
	t.Cleanup(func() { spectatorRegistry = saved })
	savedRegion := conf.LobbyRegion
	conf.LobbyRegion = "asia-northeast1"
	t.Cleanup(func() { conf.LobbyRegion = savedRegion })

	peers := playerInfoTestMatch()
	for i, p := range peers {
		p.conn = &PipeConn{}
		p.PlatformInfo = map[string]string{"asia-northeast1": []string{"80", "30", "50", "60"}[i]}
	}
	lobby := &LbsLobby{Rule: DefaultRule}
	msgs, err := lobby.makeP2PMatchingMsg(peers[0].Battle, peers, nil)
	must(t, err)
	var uplinks []int32
	for _, msg := range msgs {
		matching := new(proto.P2PMatching)
		must(t, pb.Unmarshal(msg.Body, matching))
		if matching.SpectatorUplink {
			uplinks = append(uplinks, matching.PeerId)
		}
	}
	assertEq(t, []int32{1}, uplinks)
}
