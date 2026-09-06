package main

import (
	"fmt"
	"testing"

	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

func TestSpectatorSession_BuildPush_HeaderPreservesMetadata(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("closed=%t", closed), func(t *testing.T) {
			s := newTestSpectatorSessionWithPatches(2, 2)
			s.log.GdxsvVersionDeprecated = "legacy-version"
			s.log.StartAt = 1234567890
			s.log.Users[0].UserName = "player one"
			s.log.Users[0].GameParam = []byte{4, 5, 6}
			s.log.Users[0].UserNameSjis = []byte{7, 8}
			s.log.BattleData = []*proto.BattleMessage{
				{UserId: "u1", Body: []byte{9, 10}, Seq: 11},
			}
			s.PushInputs(0, []uint64{1, 2, 3})
			s.PushRoundEvent(0, 111)
			s.PushRoundResult(0, &proto.BattleLogRound{WinTeam: 1, UsedMs: []int32{3}})
			if closed {
				s.Close("player_disconnect peer=1", 1)
			}

			before := pb.Clone(s.log).(*proto.BattleLogFile)
			// The old clone-and-strip implementation defines the wire contents
			// to preserve, including close metadata for finished sessions.
			want := pb.Clone(s.log).(*proto.BattleLogFile)
			want.Inputs = nil
			want.StartMsgIndexes = nil
			want.StartMsgRandoms = nil
			want.RoundData = nil
			want.Patches = nil

			addr := testAddr(40000)
			subscribeTestSpectator(s, addr, 0)
			sub := s.downlinks[addr.String()]
			push, ok := s.buildPush(sub)
			assertEq(t, true, ok)
			assertEq(t, true, sub.sentHeader)
			if !pb.Equal(want, push.Header) {
				t.Fatalf("header metadata changed:\nwant: %v\ngot: %v", want, push.Header)
			}
			assertEq(t, true, pb.Equal(&proto.SpectatorInputPush{
				BattleCode: s.battleCode,
				Header:     want,
			}, push))
			assertEq(t, true, pb.Equal(before, s.log))

			// Returned metadata remains a deep copy, including nested bytes
			// and the repeated-message slices themselves.
			push.Header.RuleBin[0] = 99
			push.Header.Users[0].UserName = "changed"
			push.Header.Users[0].GameParam[0] = 99
			push.Header.Users[0].UserNameSjis[0] = 99
			push.Header.Users[0] = &proto.BattleLogUser{UserId: "replacement"}
			push.Header.BattleData[0].Body[0] = 99
			push.Header.BattleData[0] = &proto.BattleMessage{UserId: "replacement"}
			assertEq(t, true, pb.Equal(before, s.log))

			// A bootstrap retry must carry the same intact header again.
			subscribeTestSpectator(s, addr, 0)
			assertEq(t, false, sub.sentHeader)
			retry, ok := s.buildPush(sub)
			assertEq(t, true, ok)
			assertEq(t, true, pb.Equal(want, retry.Header))
		})
	}
}

func BenchmarkSpectatorSession_BuildPush_Header(b *testing.B) {
	for _, frames := range []int{0, 10000, 100000} {
		b.Run(fmt.Sprintf("%d_frames", frames), func(b *testing.B) {
			s := newTestSpectatorSessionWithPatches(3, 10)
			s.PushInputs(0, make([]uint64, frames))
			s.PushRoundEvent(0, 111)
			s.PushRoundResult(0, &proto.BattleLogRound{WinTeam: 1, UsedMs: []int32{3}})
			sub := &downlinkSubscriber{}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sub.sentHeader = false
				push, ok := s.buildPush(sub)
				if !ok || push.Header == nil {
					b.Fatal("expected a header push")
				}
			}
		})
	}
}
