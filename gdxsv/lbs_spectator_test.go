package main

import (
	"fmt"
	"net"
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

func TestSpectatorSession_BuildPush_IncludesCloseFields_OnceClosed_ThenStopsResending(t *testing.T) {
	s := newTestSpectatorSession()
	addr := testAddr(40000)
	s.Subscribe(addr, 0)
	sub := s.downlinks[addr.String()]
	sub.verified = true
	sub.sentHeader = true

	s.Close("game_end", -1)

	push, ok := s.buildPush(sub)
	assertEq(t, true, ok)
	assertEq(t, "game_end", push.CloseReason)

	_, ok = s.buildPush(sub)
	assertEq(t, false, ok) // already sent the close signal; nothing new
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
