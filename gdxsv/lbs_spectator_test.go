package main

import (
	"testing"

	"gdxsv/gdxsv/proto"
)

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

	ack := s.PushInputs(0, []uint64{1, 2, 3})
	assertEq(t, int32(3), ack)
	assertEq(t, []uint64{1, 2, 3}, s.log.Inputs)

	ack = s.PushInputs(3, []uint64{4, 5})
	assertEq(t, int32(5), ack)
	assertEq(t, []uint64{1, 2, 3, 4, 5}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_GapThenFill(t *testing.T) {
	s := newTestSpectatorSession()

	// Frames 0-2 arrive, then frames 5-6 arrive ahead of a gap (3-4 lost
	// so far). The ack frame must not advance past the gap.
	ack := s.PushInputs(0, []uint64{10, 11, 12})
	assertEq(t, int32(3), ack)

	ack = s.PushInputs(5, []uint64{15, 16})
	assertEq(t, int32(3), ack) // still stuck behind the 3-4 gap
	assertEq(t, []uint64{10, 11, 12}, s.log.Inputs)

	// The peer's resend-until-acked backlog (GGPO-style) re-delivers 3-6
	// on its next packet, since LBS never acked past 3.
	ack = s.PushInputs(3, []uint64{13, 14, 15, 16})
	assertEq(t, int32(7), ack)
	assertEq(t, []uint64{10, 11, 12, 13, 14, 15, 16}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_DedupsRedundantPeers(t *testing.T) {
	s := newTestSpectatorSession()

	// Two "different peers" send the same frame range redundantly.
	s.PushInputs(0, []uint64{1, 2, 3})
	ack := s.PushInputs(0, []uint64{999, 999, 999}) // must not overwrite

	assertEq(t, int32(3), ack)
	assertEq(t, []uint64{1, 2, 3}, s.log.Inputs)
}

func TestSpectatorSession_PushInputs_IgnoredAfterClose(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(0, []uint64{1})
	s.Close("game_end", -1)

	ack := s.PushInputs(1, []uint64{2, 3})
	assertEq(t, int32(1), ack)
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
