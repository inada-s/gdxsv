package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

func newTestSpectatorRegistry() *SpectatorRegistry {
	return &SpectatorRegistry{
		sessions:        make(map[string]*SpectatorSession),
		subscriberIndex: make(map[string]*SpectatorSession),
		wake:            make(chan struct{}, 1),
	}
}

func testAddr(port int) *net.UDPAddr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
}

func newTestSpectatorSession() *SpectatorSession {
	matching := &proto.P2PMatching{
		BattleCode: "test-battle-code",
		SessionId:  42,
		RuleBin:    []byte{1, 2, 3},
		Users: []*proto.BattleLogUser{
			{UserId: "u1"}, {UserId: "u2"}, {UserId: "u3"}, {UserId: "u4"},
		},
	}
	return newSpectatorSession(matching, "dc2", nil)
}

func TestSpectatorSession_PushInputs_InOrder(t *testing.T) {
	s := newTestSpectatorSession()

	ack, advanced := s.PushInputs(0, []uint64{1, 2, 3})
	assertEq(t, int32(3), ack)
	assertEq(t, true, advanced)
	assertEq(t, []uint64{1, 2, 3}, s.log.Inputs)

	ack, advanced = s.PushInputs(3, []uint64{4, 5})
	assertEq(t, int32(5), ack)
	assertEq(t, true, advanced)
	assertEq(t, []uint64{1, 2, 3, 4, 5}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_GapThenFill(t *testing.T) {
	s := newTestSpectatorSession()

	// Frames 0-2 arrive, then frames 5-6 arrive ahead of a gap (3-4 lost
	// so far). The ack frame must not advance past the gap.
	ack, advanced := s.PushInputs(0, []uint64{10, 11, 12})
	assertEq(t, int32(3), ack)
	assertEq(t, true, advanced)

	ack, advanced = s.PushInputs(5, []uint64{15, 16})
	assertEq(t, int32(3), ack) // still stuck behind the 3-4 gap
	assertEq(t, false, advanced)
	assertEq(t, []uint64{10, 11, 12}, s.log.Inputs)

	// The peer's resend-until-acked backlog (GGPO-style) re-delivers 3-6
	// on its next packet, since LBS never acked past 3.
	ack, advanced = s.PushInputs(3, []uint64{13, 14, 15, 16})
	assertEq(t, int32(7), ack)
	assertEq(t, true, advanced)
	assertEq(t, []uint64{10, 11, 12, 13, 14, 15, 16}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_DedupsRedundantPeers(t *testing.T) {
	s := newTestSpectatorSession()

	// Two "different peers" send the same frame range redundantly.
	s.PushInputs(0, []uint64{1, 2, 3})
	ack, _ := s.PushInputs(0, []uint64{999, 999, 999}) // must not overwrite

	assertEq(t, int32(3), ack)
	assertEq(t, []uint64{1, 2, 3}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_IgnoredAfterClose(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1})
	s.Close("game_end", -1)

	ack, advanced := s.PushInputs(1, []uint64{2, 3})
	assertEq(t, int32(1), ack)
	assertEq(t, false, advanced)
	assertEq(t, []uint64{1}, s.log.Inputs)
}

func TestSpectatorSession_PushRoundEvent_DedupsByFrame(t *testing.T) {
	s := newTestSpectatorSession()

	s.PushRoundEvent(0, 111)
	s.PushRoundEvent(0, 999) // redundant peer, same frame: ignored
	s.PushRoundEvent(500, 222)

	assertEq(t, []int32{0, 500}, s.log.StartMsgIndexes)
	assertEq(t, []uint64{111, 222}, s.log.StartMsgRandoms)
	assertEq(t, 2, len(s.log.RoundData))
}

func TestSpectatorSession_PushRoundResult_FirstWinTeamWins(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushRoundEvent(0, 111) // round 0 exists

	s.PushRoundResult(0, &proto.BattleLogRound{WinTeam: 1, UsedMs: []int32{3}})
	s.PushRoundResult(0, &proto.BattleLogRound{WinTeam: 2, UsedMs: []int32{9}}) // redundant peer: ignored

	assertEq(t, int32(1), s.log.RoundData[0].WinTeam)
	assertEq(t, []int32{3}, s.log.RoundData[0].UsedMs)
}

func TestSpectatorSession_PushRoundResult_GrowsRoundData(t *testing.T) {
	s := newTestSpectatorSession()

	// Result arrives before any PushRoundEvent grew round_data (peer
	// reordering across the two message types); RoundData must still
	// grow to fit.
	s.PushRoundResult(2, &proto.BattleLogRound{WinTeam: 1})

	assertEq(t, 3, len(s.log.RoundData))
	assertEq(t, int32(1), s.log.RoundData[2].WinTeam)
}

func TestSpectatorSession_Close_IsIdempotent(t *testing.T) {
	s := newTestSpectatorSession()

	s.Close("game_end", -1)
	s.Close("player_disconnect peer=1", 1) // must not override the first reason

	assertEq(t, "game_end", s.log.CloseReason)
	assertEq(t, int32(-1), s.log.DisconnectUserIndex)
}

func TestSpectatorSession_Snapshot_IsIndependentCopy(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1, 2, 3})

	snap := s.Snapshot()
	assertEq(t, []uint64{1, 2, 3}, snap.Inputs)

	s.PushInputs(3, []uint64{4})
	assertEq(t, []uint64{1, 2, 3}, snap.Inputs) // snapshot unaffected by later pushes
	assertEq(t, []uint64{1, 2, 3, 4}, s.log.Inputs)
}

func TestSpectatorRegistry_Open_SkipsTrainingGames(t *testing.T) {
	r := &SpectatorRegistry{sessions: make(map[string]*SpectatorSession)}
	r.Open(&proto.P2PMatching{BattleCode: "training-code", IsTrainingGame: true}, "dc2", nil)

	_, ok := r.GetAny("training-code")
	assertEq(t, false, ok)
}

func TestSpectatorRegistry_Open_IsIdempotent(t *testing.T) {
	r := &SpectatorRegistry{sessions: make(map[string]*SpectatorSession)}
	matching := &proto.P2PMatching{BattleCode: "bc", SessionId: 1}

	r.Open(matching, "dc2", nil)
	s1, _ := r.GetAny("bc")
	s1.PushInputs(0, []uint64{1})

	r.Open(matching, "dc2", nil) // second call must not replace the session
	s2, _ := r.GetAny("bc")

	assertEq(t, s1, s2)
	assertEq(t, []uint64{1}, s2.log.Inputs)
}

func TestSpectatorRegistry_Get_RejectsWrongSessionID(t *testing.T) {
	r := &SpectatorRegistry{sessions: make(map[string]*SpectatorSession)}
	r.Open(&proto.P2PMatching{BattleCode: "bc", SessionId: 1}, "dc2", nil)

	_, ok := r.Get("bc", 2)
	assertEq(t, false, ok)

	_, ok = r.Get("bc", 1)
	assertEq(t, true, ok)
}

func TestSpectatorSession_Subscribe_RegistersDownlink(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)

	s.Subscribe(addr, 5)

	sub, ok := s.downlinks[addr.String()]
	assertEq(t, true, ok)
	assertEq(t, int32(5), sub.ackedFrame)
}

func TestSpectatorSession_Subscribe_FromFrameNeverMovesAckBackward(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)

	s.Subscribe(addr, 10)
	s.Subscribe(addr, 3) // stale/reordered resubscribe: must not undo progress

	assertEq(t, int32(10), s.downlinks[addr.String()].ackedFrame)
}

func TestSpectatorSession_Ack_AdvancesSubscriberAckedFrame(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)

	s.Ack(addr.String(), 7, 0)

	assertEq(t, int32(7), s.downlinks[addr.String()].ackedFrame)
}

func TestSpectatorSession_Ack_IgnoredForUnknownSubscriber(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)

	s.Ack(testAddr(50000).String(), 999, 0) // never subscribed

	assertEq(t, int32(0), s.downlinks[addr.String()].ackedFrame)
	_, ok := s.downlinks[testAddr(50000).String()]
	assertEq(t, false, ok)
}

func TestSpectatorSession_BuildPush_SlicesFromAckedFrame_CappedAtMaxFrames(t *testing.T) {
	s := newTestSpectatorSession()
	inputs := make([]uint64, maxSpectatorPushFrames+50)
	for i := range inputs {
		inputs[i] = uint64(i)
	}
	s.PushInputs(0, inputs)

	addr := testAddr(40000)
	s.Subscribe(addr, 10)
	sub := s.downlinks[addr.String()]
	sub.verified = true   // steady state: this subscriber has acked before
	sub.sentHeader = true // and was already sent the one-off header

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, int32(10), push.StartFrame)
	assertEq(t, maxSpectatorPushFrames, len(push.Inputs))
	assertEq(t, uint64(10), push.Inputs[0])
}

func TestSpectatorSession_BuildPush_NothingNewReturnsFalse(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1, 2, 3})

	addr := testAddr(40000)
	s.Subscribe(addr, 3) // already fully caught up
	sub := s.downlinks[addr.String()]
	sub.verified = true
	sub.sentHeader = true

	_, ok := s.buildPush(sub)
	assertEq(t, false, ok)
}

func TestSpectatorSession_BuildPush_IncludesRoundState_ThenStopsResendingUntilChanged(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushRoundEvent(0, 111)

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true   // round state is only sent to verified subscribers
	sub.sentHeader = true // header goes in its own push, before any of this

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, []int32{0}, push.StartMsgIndexes)
	assertEq(t, []uint64{111}, push.StartMsgRandoms)

	// Nothing changed since: round state must not be resent.
	_, ok = s.buildPush(sub)
	assertEq(t, false, ok)

	// A new round event bumps roundStateVersion: must be sent again.
	s.PushRoundEvent(100, 222)
	push, ok = s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, []int32{0, 100}, push.StartMsgIndexes)
}

func TestSpectatorSession_BuildPush_HoldsCloseUntilInputsDelivered(t *testing.T) {
	// A spectator that joins after the battle ended still has the whole match
	// to receive. It stops its downlink the moment it sees close, so close
	// must not ride along with a push that leaves inputs undelivered.
	s := newTestSpectatorSession()
	inputs := make([]uint64, maxSpectatorPushFrames*2)
	s.PushInputs(0, inputs)
	s.Close("game_end", -1)

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true
	sub.sentHeader = true

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, maxSpectatorPushFrames, len(push.Inputs))
	assertEq(t, "", push.CloseReason)

	// One chunk short: still no close.
	sub.ackedFrame = maxSpectatorPushFrames
	push, ok = s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, "", push.CloseReason)

	// Everything delivered: close goes out, on its own.
	sub.ackedFrame = int32(len(inputs))
	push, ok = s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, 0, len(push.Inputs))
	assertEq(t, "game_end", push.CloseReason)
	assertEq(t, int32(-1), push.DisconnectUserIndex)
}

func TestSpectatorSession_BuildPush_ResendsCloseWhileSubscribed(t *testing.T) {
	// Close is never acked, so there is no latch: every fanout offers it
	// again. That is the retry for a lost close packet. A spectator that got
	// it stops its keepalives and sweepSubscribersLocked drops it.
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true
	sub.sentHeader = true

	s.Close("game_end", -1)

	for i := 0; i < 3; i++ {
		push, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, "game_end", push.CloseReason)
	}
}

func TestSpectatorSession_SweepSubscribersLocked_DropsStale(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	s.downlinks[addr.String()].lastSeen = s.downlinks[addr.String()].lastSeen.Add(-2 * spectatorSubscriberTimeout)

	dropped := s.sweepSubscribersLocked(time.Now())

	assertEq(t, []string{addr.String()}, dropped)
	_, ok := s.downlinks[addr.String()]
	assertEq(t, false, ok)
}

func TestSpectatorRegistry_HandleSubscribe_RejectsUnknownBattleCode(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "bc", SessionId: 1}, "dc2", nil)

	// A spectator has no session_id (unlike a real battle participant), so
	// HandleSubscribe looks up purely by battle_code - it must still reject
	// a battle_code that doesn't exist.
	r.HandleSubscribe(testAddr(40000), &proto.SpectatorSubscribeRequest{BattleCode: "no-such-battle", FromFrame: 0})

	assertEq(t, 0, len(r.subscriberIndex))
}

func TestSpectatorRegistry_HandleSubscribe_RegistersInBothSessionAndIndex(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "bc", SessionId: 1}, "dc2", nil)
	addr := testAddr(40000)

	r.HandleSubscribe(addr, &proto.SpectatorSubscribeRequest{BattleCode: "bc", FromFrame: 5})

	s, _ := r.GetAny("bc")
	assertEq(t, 1, len(s.downlinks))
	assertEq(t, s, r.subscriberIndex[addr.String()])
}

func TestSpectatorRegistry_HandleAck_UnknownAddrIsNoop_DoesNotAffectOtherSubscribers(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "bc", SessionId: 1}, "dc2", nil)
	known := testAddr(40000)
	r.HandleSubscribe(known, &proto.SpectatorSubscribeRequest{BattleCode: "bc", FromFrame: 0})

	stranger := testAddr(50000)
	r.HandleAck(stranger, &proto.SpectatorInputAck{BattleCode: "bc", AckFrame: 999})

	s, _ := r.GetAny("bc")
	assertEq(t, int32(0), s.downlinks[known.String()].ackedFrame) // unaffected
	_, ok := s.downlinks[stranger.String()]
	assertEq(t, false, ok)
}

// A subscribe datagram's source address can be forged, so until a subscriber
// acks (proving it really receives what we send) it must not be usable as an
// amplifier - see maxUnverifiedPushFrames.

func TestSpectatorSession_BuildPush_UnverifiedSubscriberGetsSmallPushOnly(t *testing.T) {
	s := newTestSpectatorSession()
	inputs := make([]uint64, maxSpectatorPushFrames+50)
	for i := range inputs {
		inputs[i] = uint64(i)
	}
	s.PushInputs(0, inputs)
	s.PushRoundEvent(0, 111) // variable-size round state must be withheld too

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, maxUnverifiedPushFrames, len(push.Inputs))
	assertEq(t, 0, len(push.StartMsgIndexes))
}

func TestSpectatorSession_Ack_VerifiesSubscriberAndLiftsTheCap(t *testing.T) {
	s := newTestSpectatorSession()
	inputs := make([]uint64, maxSpectatorPushFrames+50)
	s.PushInputs(0, inputs)

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	assertEq(t, false, sub.verified)

	s.Ack(addr.String(), 0, 0)
	assertEq(t, true, sub.verified)

	// First push to a newly verified subscriber is the header.
	header, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, header.Header != nil)

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, maxSpectatorPushFrames, len(push.Inputs))
}

func TestSpectatorSession_Subscribe_SetsProbeDue_SoOnePushIsAllowed(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1, 2, 3})

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]

	// Set by the subscribe, and cleared by fanoutOnce after its one push, so
	// an unverified subscriber never enters the periodic resend loop.
	assertEq(t, true, sub.probeDue)
	assertEq(t, false, sub.verified)
}

func TestSpectatorSession_BuildPush_SendsHeaderOnceBeforeInputs(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1, 2, 3})
	s.PushRoundEvent(0, 111)

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true

	// Header first, carrying setup data but neither inputs nor round state.
	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, push.Header != nil)
	assertEq(t, "dc2", push.Header.GameDisk)
	assertEq(t, 0, len(push.Header.Inputs))
	assertEq(t, 0, len(push.Header.StartMsgIndexes))
	assertEq(t, 0, len(push.Inputs))

	// Then the normal stream, and the header is never resent.
	push, ok = s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, push.Header == nil)
	assertEq(t, 3, len(push.Inputs))
}

func TestSpectatorSession_BuildPush_UnverifiedSubscriberGetsNoHeader(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1, 2, 3})

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, push.Header == nil)
	assertEq(t, false, sub.sentHeader)
}

func TestSpectatorSession_BuildPush_UnverifiedGetsProbeEvenWithNothingToSend(t *testing.T) {
	// A spectator joining a battle with no input yet still needs something
	// to ack, or it can never be verified and never receives the header.
	s := newTestSpectatorSession()

	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, 0, len(push.Inputs))
	assertEq(t, true, push.Header == nil)
}

func TestSpectatorSession_Subscribe_ReArmsHeaderWhileBootstrapping(t *testing.T) {
	// The header is sent optimistically and never retransmitted with the
	// inputs, so a lost one has to be re-armed by the next bootstrap
	// subscribe or the subscriber is stranded.
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true

	_, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, sub.sentHeader)

	// Pretend it was lost: the client re-subscribes from 0 and must get another.
	s.Subscribe(addr, 0)
	assertEq(t, false, sub.sentHeader)

	// Once it has folded anything in it asks from a later frame, and the
	// header is not sent again.
	s.Subscribe(addr, 5)
	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, push.Header != nil) // the re-armed one from above
	s.Subscribe(addr, 9)
	assertEq(t, true, sub.sentHeader)
}

// Patches must reach the spectator or its simulation diverges, and they are
// big enough that sending them with the header would fragment the datagram -
// so they stream as their own acked chunks. See maxPatchChunkBytes.

func newTestSpectatorSessionWithPatches(n, codesEach int) *SpectatorSession {
	s := newTestSpectatorSession()
	for i := 0; i < n; i++ {
		p := &proto.GamePatch{GameDisk: "dc2", Name: fmt.Sprintf("patch%d", i)}
		for j := 0; j < codesEach; j++ {
			p.Codes = append(p.Codes, &proto.GamePatchCode{
				Size: 4, Address: uint32(0x0c000000 + j), Original: 1, Changed: 2,
			})
		}
		s.log.Patches = append(s.log.Patches, p)
	}
	return s
}

func verifiedSubWithHeaderSent(s *SpectatorSession, addr *net.UDPAddr) *downlinkSubscriber {
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true
	_, _ = s.buildPush(sub) // consume the header push
	return sub
}

func TestSpectatorSession_BuildPush_HeaderExcludesPatches(t *testing.T) {
	s := newTestSpectatorSessionWithPatches(3, 2)
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, true, push.Header != nil)
	// Stripped from the header - they arrive as chunks instead.
	assertEq(t, 0, len(push.Header.Patches))
}

func TestSpectatorSession_BuildPush_ChunksPatchesUnderByteBudget(t *testing.T) {
	// Each patch is far too big to share a datagram, so every chunk holds one.
	s := newTestSpectatorSessionWithPatches(3, 80)
	sub := verifiedSubWithHeaderSent(s, testAddr(40000))

	for i := 0; i < 3; i++ {
		push, ok := s.buildPush(sub)
		assertEq(t, true, ok)
		assertEq(t, int32(i), push.PatchStart)
		assertEq(t, int32(3), push.PatchTotal)
		assertEq(t, 1, len(push.Patches))
		if pb.Size(push) > 1400 {
			t.Fatalf("patch chunk %d is %d bytes, would fragment", i, pb.Size(push))
		}
		// Not acked yet, so the same chunk is what gets resent.
		again, _ := s.buildPush(sub)
		assertEq(t, int32(i), again.PatchStart)

		s.Ack(sub.remoteAddr.String(), 0, int32(i+1))
	}

	// All acked: back to normal input streaming.
	s.PushInputs(0, []uint64{1, 2, 3})
	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, 0, len(push.Patches))
	assertEq(t, 3, len(push.Inputs))
}

func TestSpectatorSession_BuildPush_PacksSmallPatchesTogether(t *testing.T) {
	s := newTestSpectatorSessionWithPatches(4, 1)
	sub := verifiedSubWithHeaderSent(s, testAddr(40000))

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, 4, len(push.Patches)) // all four fit in one datagram
}

func TestSpectatorSession_BuildPush_NoPatchesToUnverifiedSubscriber(t *testing.T) {
	s := newTestSpectatorSessionWithPatches(3, 2)
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, 0, len(push.Patches))
	assertEq(t, true, push.Header == nil)
}

// verifiedSub registers a subscriber and marks it verified, as one real ack
// would, so the taper tests start from a healthy steady state.
func verifiedSub(s *SpectatorSession, addr *net.UDPAddr) *downlinkSubscriber {
	s.Subscribe(addr, 0)
	s.Ack(addr.String(), 0, 0)
	sub := s.downlinks[addr.String()]
	sub.probeDue = false
	sub.sentHeader = true
	return sub
}

// fanoutStep mimics one fanout pass over a single subscriber: the taper gate,
// buildPush, then the taper bookkeeping. Reports whether a push went out.
func fanoutStep(s *SpectatorSession, sub *downlinkSubscriber, frame int32) bool {
	s.PushInputs(frame, []uint64{uint64(frame)})

	s.mtx.Lock()
	defer s.mtx.Unlock()

	if sub.shouldSkipPush() {
		return false
	}
	if _, ok := s.buildPush(sub); !ok {
		return false
	}
	sub.probeDue = false
	sub.notePushSent()
	return true
}

func TestSpectatorSession_SilentSubscriber_FullRateWithinGrace(t *testing.T) {
	s := newTestSpectatorSession()
	sub := verifiedSub(s, testAddr(30001))

	// A subscriber that never acks still gets silentPushGrace pushes back to
	// back, so ordinary packet loss is never tapered.
	for i := int32(0); i < silentPushGrace; i++ {
		if !fanoutStep(s, sub, i) {
			t.Fatalf("push %d was skipped inside the grace window", i)
		}
	}
	assertEq(t, int32(silentPushGrace), sub.silentPushes)
	assertEq(t, int32(2), sub.skipFanouts)
}

func TestSpectatorSession_SilentSubscriber_TapersAfterGrace(t *testing.T) {
	s := newTestSpectatorSession()
	sub := verifiedSub(s, testAddr(30002))

	var waits []int32
	for i := int32(0); i < 400; i++ {
		if fanoutStep(s, sub, i) {
			waits = append(waits, sub.skipFanouts)
		}
	}

	// The first silentPushGrace pushes are ungated; after that each further
	// unacked push doubles the wait until it holds at 1<<maxSilentBackoffShift.
	want := []int32{0, 0, 0, 0, 0, 0, 0, 2, 4, 8, 16, 32, 64, 64}
	if len(waits) < len(want) {
		t.Fatalf("only %d pushes in 400 fanouts, want at least %d", len(waits), len(want))
	}
	for i, w := range want {
		if waits[i] != w {
			t.Fatalf("push %d: skipFanouts = %d, want %d (got %v)", i+1, waits[i], w, waits[:len(want)])
		}
	}
}

func TestSpectatorSession_SilentSubscriber_TaperCutsTotalPushes(t *testing.T) {
	// The point of the taper: far fewer pushes at a peer that went quiet.
	const fanouts = 600 // 10s at 60/s, one spectatorSubscriberTimeout

	tapered := newTestSpectatorSession()
	tsub := verifiedSub(tapered, testAddr(30005))
	sent := 0
	for i := int32(0); i < fanouts; i++ {
		if fanoutStep(tapered, tsub, i) {
			sent++
		}
	}

	if sent >= 30 {
		t.Fatalf("tapered subscriber got %d pushes in %d fanouts, expected far fewer", sent, fanouts)
	}
	if sent < silentPushGrace {
		t.Fatalf("tapered subscriber got %d pushes, fewer than the %d grace pushes", sent, silentPushGrace)
	}
}

func TestSpectatorSession_SilentSubscriber_AckClearsTaper(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(30003)
	sub := verifiedSub(s, addr)

	for i := int32(0); i < silentPushGrace+3; i++ {
		fanoutStep(s, sub, i)
	}
	if sub.skipFanouts == 0 {
		t.Fatal("expected the subscriber to be tapered")
	}

	s.Ack(addr.String(), 1, 0)
	assertEq(t, int32(0), sub.skipFanouts)
	assertEq(t, int32(0), sub.silentPushes)
}

func TestSpectatorSession_SilentSubscriber_SubscribePunchesThroughTaper(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(30004)
	sub := verifiedSub(s, addr)

	for i := int32(0); i < silentPushGrace+3; i++ {
		fanoutStep(s, sub, i)
	}
	if sub.skipFanouts == 0 {
		t.Fatal("expected the subscriber to be tapered")
	}

	// A live spectator whose acks were lost keeps sending its keepalive. That
	// must still earn a push, or it could never ack its way back to full rate.
	s.Subscribe(addr, 0)
	if !fanoutStep(s, sub, silentPushGrace+3) {
		t.Fatal("a keepalive subscribe should push through the taper")
	}
}

func TestSpectatorRegistry_SweepDropsClosedSessionAfterRetention(t *testing.T) {
	r := newTestSpectatorRegistry()
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s

	s.Close("finished", 0)

	r.mtx.Lock()
	r.sweepLocked()
	r.mtx.Unlock()
	assertEq(t, 1, len(r.sessions)) // still inside the retention window

	s.mtx.Lock()
	s.closedAt = time.Now().Add(-spectatorSessionRetention - time.Minute)
	s.mtx.Unlock()

	r.mtx.Lock()
	r.sweepLocked()
	r.mtx.Unlock()
	assertEq(t, 0, len(r.sessions))
}

// A session only closes when a peer reports the battle ended. If every peer
// vanishes at once nobody reports, and the session must still be freed.
func TestSpectatorRegistry_SweepDropsSilentSessionThatNeverClosed(t *testing.T) {
	r := newTestSpectatorRegistry()
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s

	s.PushInputs(0, []uint64{1, 2, 3})

	r.mtx.Lock()
	r.sweepLocked()
	r.mtx.Unlock()
	assertEq(t, 1, len(r.sessions)) // pushing, so alive

	s.mtx.Lock()
	s.lastPushAt = time.Now().Add(-spectatorSessionIdleTimeout - time.Minute)
	s.mtx.Unlock()

	r.mtx.Lock()
	r.sweepLocked()
	r.mtx.Unlock()
	assertEq(t, 0, len(r.sessions))
	assertEq(t, false, s.closed) // dropped without ever having been closed
}

func TestSpectatorRegistry_SweepKeepsActiveSession(t *testing.T) {
	r := newTestSpectatorRegistry()
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s

	// Older than both windows, but still pushing: a long battle must survive.
	s.mtx.Lock()
	s.lastPushAt = time.Now().Add(-spectatorSessionIdleTimeout - time.Minute)
	s.mtx.Unlock()
	s.PushInputs(0, []uint64{1})

	r.mtx.Lock()
	r.sweepLocked()
	r.mtx.Unlock()
	assertEq(t, 1, len(r.sessions))
}

// A battle only streams if a peer is actually pushing. Not every player runs a
// build with the uplink, so "session exists" is not the same as "spectatable".
func TestSpectatorRegistry_LiveStatus(t *testing.T) {
	r := newTestSpectatorRegistry()
	s := newTestSpectatorSession()
	r.sessions[s.battleCode] = s

	live, subs := r.LiveStatus("no-such-battle")
	assertEq(t, false, live)
	assertEq(t, 0, subs)

	// Open, but no peer has pushed: nothing to watch yet.
	live, _ = r.LiveStatus(s.battleCode)
	assertEq(t, false, live)

	s.PushInputs(0, []uint64{1, 2, 3})
	live, subs = r.LiveStatus(s.battleCode)
	assertEq(t, true, live)
	assertEq(t, 0, subs)

	s.Subscribe(testAddr(20001), 0)
	_, subs = r.LiveStatus(s.battleCode)
	assertEq(t, 1, subs)

	// Once the battle ends it is no longer live, even though the assembled log
	// is still held for the retention window.
	s.Close("finished", 0)
	live, _ = r.LiveStatus(s.battleCode)
	assertEq(t, false, live)
}

func TestSpectatorsHandler(t *testing.T) {
	// The handler reads the package-level registry, so swap in a clean one.
	saved := spectatorRegistry
	spectatorRegistry = newTestSpectatorRegistry()
	defer func() { spectatorRegistry = saved }()

	get := func(query string) (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodGet, "/lbs/spectators"+query, nil)
		rec := httptest.NewRecorder()
		spectatorsHandler(rec, req)
		var body map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	code, _ := get("")
	assertEq(t, http.StatusBadRequest, code)

	// Unknown battle: answers rather than erroring, so a client can tell
	// "nothing to watch" from a failed request.
	code, body := get("?battle_code=nope")
	assertEq(t, http.StatusOK, code)
	assertEq(t, false, body["live"])
	assertEq(t, float64(0), body["spectators"])

	s := newTestSpectatorSession()
	spectatorRegistry.sessions[s.battleCode] = s

	// Open but nothing pushed yet: not live.
	_, body = get("?battle_code=" + s.battleCode)
	assertEq(t, false, body["live"])

	s.PushInputs(0, []uint64{1, 2, 3})
	s.Subscribe(testAddr(30001), 0)
	s.Subscribe(testAddr(30002), 0)

	code, body = get("?battle_code=" + s.battleCode)
	assertEq(t, http.StatusOK, code)
	assertEq(t, true, body["live"])
	assertEq(t, float64(2), body["spectators"])
	assertEq(t, s.battleCode, body["battle_code"])

	// Once the battle ends it is no longer live, even with subscribers still
	// attached and the log still held for the retention window.
	s.Close("finished", 0)
	_, body = get("?battle_code=" + s.battleCode)
	assertEq(t, false, body["live"])
}
