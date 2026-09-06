package main

import (
	"math"
	"net"
	"testing"
	"time"

	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

func TestSpectatorSession_RoundAckRejectsInvalidVersions(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1})
	s.PushRoundEvent(0, 123)
	addr := testAddr(40000)
	subscribeTestSpectator(s, addr, 0)
	sub := s.downlinks[addr.String()]
	sub.skipFanouts = 64
	lastSeen := sub.lastSeen
	for _, version := range []int32{-1, 2, math.MaxInt32} {
		s.Ack(addr.String(), 1, 0, version)
		assertEq(t, int32(0), sub.ackedFrame)
		assertEq(t, int32(0), sub.ackedRoundStateVersion)
		assertEq(t, int32(64), sub.skipFanouts)
		assertEq(t, lastSeen, sub.lastSeen)
	}
}

func TestSpectatorSession_RoundResultBeforeStart(t *testing.T) {
	s := newTestSpectatorSession()
	assertEq(t, true, s.PushRoundResult(0, &proto.BattleLogRound{WinTeam: 2}))
	assertEq(t, true, s.PushRoundEvent(0, 123))
	assertEq(t, 1, len(s.log.RoundData))
	assertEq(t, int32(2), s.log.RoundData[0].WinTeam)
	assertEq(t, true, s.PushRoundEvent(500, 456))
	assertEq(t, false, s.PushRoundEvent(250, 999))
	assertEq(t, 2, len(s.log.RoundData))
	assertEq(t, int32(0), s.log.RoundData[1].WinTeam)
}

func TestSpectatorSession_RoundUpdatesAreBoundedAndIdempotent(t *testing.T) {
	s := newTestSpectatorSession()
	assertEq(t, false, s.PushRoundEvent(-1, 1))
	assertEq(t, false, s.PushRoundResult(0, &proto.BattleLogRound{}))
	for i := int32(0); i < maxSpectatorRounds; i++ {
		assertEq(t, true, s.PushRoundEvent(i*100, uint64(i)))
		assertEq(t, true, s.PushRoundResult(i, &proto.BattleLogRound{WinTeam: 1}))
	}
	assertEq(t, false, s.PushRoundEvent(1, 999))    // out of order
	assertEq(t, false, s.PushRoundEvent(1000, 999)) // eleventh round
	assertEq(t, maxSpectatorRounds, len(s.roundEventSeen))
	s.Close("game_end", -1)
	before := pb.Clone(s.log)
	version := s.roundStateVersion
	for i := int32(0); i < maxSpectatorRounds; i++ {
		// Lost ACKs can still be recovered after another participant closes.
		assertEq(t, true, s.PushRoundEvent(i*100, uint64(i)))
		assertEq(t, true, s.PushRoundResult(i, &proto.BattleLogRound{WinTeam: 1}))
	}
	assertEq(t, version, s.roundStateVersion)
	assertEq(t, true, pb.Equal(before, s.log))
}

func TestSpectatorSession_ClosedBootstrapDeliversWholeRecording(t *testing.T) {
	for _, patchCount := range []int{0, 3} {
		s := newTestSpectatorSessionWithPatches(patchCount, 10)
		// Same shape as the preferred four-player, five-round replay.
		inputs := make([]uint64, 33499)
		for i := range inputs {
			inputs[i] = uint64(i) + 1
		}
		s.PushInputs(0, inputs)
		for i := int32(0); i < 5; i++ {
			s.PushRoundEvent(i*6000, uint64(i)+123)
			s.PushRoundResult(i, &proto.BattleLogRound{WinTeam: i%2 + 1, UsedMs: []int32{1, 2, 3, 4}})
		}
		s.Close("game_end", -1)
		addr := testAddr(40000)
		subscribeTestSpectator(s, addr, 0)
		sub := s.downlinks[addr.String()]
		header, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, "", header.Header.CloseReason)
		assertEq(t, int32(0), header.Header.DisconnectUserIndex)
		assertEq(t, int32(patchCount), header.PatchTotal)
		log := header.Header
		for len(log.Patches) < patchCount {
			push, ok := s.buildPush(sub)
			assertEq(t, true, ok)
			log.Patches = append(log.Patches, push.Patches...)
			s.Ack(addr.String(), 0, int32(len(log.Patches)), 0)
		}
		// Drop every round snapshot while receiving all inputs. Input ACKs
		// alone must neither retire round state nor permit a close push.
		for len(log.Inputs) < len(inputs) {
			push, ok := s.buildPush(sub)
			assertEq(t, true, ok)
			assertEq(t, "", push.CloseReason)
			assertEq(t, s.roundStateVersion, push.RoundStateVersion)
			assertEq(t, int32(len(log.Inputs)), push.StartFrame)
			log.Inputs = append(log.Inputs, push.Inputs...)
			s.Ack(addr.String(), int32(len(log.Inputs)), int32(patchCount), 0)
		}
		push, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, "", push.CloseReason)
		assertEq(t, 0, len(push.Inputs))
		log.StartMsgIndexes = push.StartMsgIndexes
		log.StartMsgRandoms = push.StartMsgRandoms
		log.RoundData = push.RoundData
		// Lose the first round ACK: the complete snapshot must repeat.
		retry, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, true, pb.Equal(push, retry))
		s.Ack(addr.String(), int32(len(inputs)), int32(patchCount), push.RoundStateVersion)
		closePush, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, "game_end", closePush.CloseReason)
		assertEq(t, s.roundStateVersion, closePush.RoundStateVersion)
		log.CloseReason = closePush.CloseReason
		log.DisconnectUserIndex = closePush.DisconnectUserIndex
		assertEq(t, true, pb.Equal(s.log, log))
	}
}

func TestSpectatorRoundUDP_ReacksLostAcknowledgements(t *testing.T) {
	listen := func() *net.UDPConn {
		conn, err := net.ListenUDP("udp4", testAddr(0))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	server, client := listen(), listen()
	r := newTestSpectatorRegistry()
	saved := spectatorRegistry
	spectatorRegistry = r
	t.Cleanup(func() { spectatorRegistry = saved })
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s
	addr := client.LocalAddr().(*net.UDPAddr)
	readAck := func() *proto.SpectatorInputAck {
		buf := make([]byte, 256)
		n, _, err := client.ReadFromUDP(buf)
		if err != nil {
			t.Fatal(err)
		}
		pkt := &proto.Packet{}
		if err := pb.Unmarshal(buf[:n], pkt); err != nil {
			t.Fatal(err)
		}
		assertEq(t, proto.MessageType_SpectatorInputAckType, pkt.Type)
		assertEq(t, s.battleCode, pkt.GetSpectatorInputAckData().GetBattleCode())
		return pkt.SpectatorInputAckData
	}
	for attempt := 0; attempt < 3; attempt++ {
		// Attempt 0's ACK is discarded by the caller; attempt 2 is after close.
		handleSpectatorRoundEvent(server, addr, &proto.SpectatorRoundEvent{
			BattleCode: s.battleCode, SessionId: s.sessionID, Frame: 0, RandomValue: 123,
		})
		ack := readAck()
		assertEq(t, []int32{0}, ack.RoundEventAck)
		assertEq(t, 0, len(ack.RoundResultAck))
		handleSpectatorRoundResult(server, addr, &proto.SpectatorRoundResult{
			BattleCode: s.battleCode, SessionId: s.sessionID, RoundIndex: 0,
			Round: &proto.BattleLogRound{WinTeam: 1},
		})
		ack = readAck()
		assertEq(t, []int32{0}, ack.RoundResultAck)
		assertEq(t, 0, len(ack.RoundEventAck))
		assertEq(t, int32(2), s.roundStateVersion)
		if attempt == 1 {
			s.Close("game_end", -1)
		}
	}
}
