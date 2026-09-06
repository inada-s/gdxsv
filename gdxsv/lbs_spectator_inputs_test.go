package main

import (
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	pb "google.golang.org/protobuf/proto"
)

func testSpectatorInputs(startFrame int32, count int) []uint64 {
	inputs := make([]uint64, count)
	for i := range inputs {
		inputs[i] = uint64(startFrame) + uint64(i)
	}
	return inputs
}

func TestSpectatorSession_PushInputs_RejectsInvalidRange(t *testing.T) {
	for _, tt := range []struct {
		name       string
		startFrame int32
		count      int
	}{
		{"negative", -1, 1},
		{"negative_crosses_zero", -1, 128},
		{"empty", 10, 0},
		{"first_outside_window", 266, 1},
		{"tail_outside_window", 265, 2},
		{"far_future", 1000000, 128},
		{"frame_overflow", math.MaxInt32 - 63, 128},
		{"ack_overflow", math.MaxInt32, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSpectatorSession()
			s.PushInputs(0, testSpectatorInputs(0, 10))
			s.PushInputs(15, testSpectatorInputs(15, 2))
			s.lastPushAt = time.Unix(100, 0)
			beforeLog := pb.Clone(s.log)
			beforePending := maps.Clone(s.pendingFrames)
			beforePushAt := s.lastPushAt

			ack, advanced := s.PushInputs(tt.startFrame, make([]uint64, tt.count))

			assertEq(t, int32(10), ack)
			assertEq(t, false, advanced)
			assertEq(t, true, pb.Equal(beforeLog, s.log))
			assertEq(t, beforePending, s.pendingFrames)
			assertEq(t, beforePushAt, s.lastPushAt)
		})
	}
}

func TestSpectatorSession_PushInputs_WindowFollowsFrontier(t *testing.T) {
	s := newTestSpectatorSession()
	ack, advanced := s.PushInputs(0, testSpectatorInputs(0, 10000))
	assertEq(t, int32(10000), ack)
	assertEq(t, true, advanced)

	// Old packets cannot overwrite the recorded inputs.
	ack, advanced = s.PushInputs(0, make([]uint64, 128))
	assertEq(t, int32(10000), ack)
	assertEq(t, false, advanced)
	assertEq(t, 0, len(s.pendingFrames))

	// The last eligible frame is 10255 while frame 10000 is missing.
	ack, advanced = s.PushInputs(10128, testSpectatorInputs(10128, 128))
	assertEq(t, int32(10000), ack)
	assertEq(t, false, advanced)
	assertEq(t, 128, len(s.pendingFrames))
	assertEq(t, uint64(10255), s.pendingFrames[10255])

	ack, advanced = s.PushInputs(10256, []uint64{10256})
	assertEq(t, int32(10000), ack)
	assertEq(t, false, advanced)
	assertEq(t, 128, len(s.pendingFrames))

	// Filling the gap also consumes the buffered block and moves the window.
	ack, advanced = s.PushInputs(10000, testSpectatorInputs(10000, 128))
	assertEq(t, int32(10256), ack)
	assertEq(t, true, advanced)
	assertEq(t, 0, len(s.pendingFrames))
	assertEq(t, true, slices.Equal(testSpectatorInputs(0, 10256), s.log.Inputs))

	ack, advanced = s.PushInputs(10511, []uint64{10511})
	assertEq(t, int32(10256), ack)
	assertEq(t, false, advanced)
	assertEq(t, 1, len(s.pendingFrames))
	assertEq(t, uint64(10511), s.pendingFrames[10511])

	ack, advanced = s.PushInputs(10512, []uint64{10512})
	assertEq(t, int32(10256), ack)
	assertEq(t, false, advanced)
	assertEq(t, 1, len(s.pendingFrames))
}

func TestSpectatorSession_PushInputs_FullPendingWindowStillFillsGap(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(1, testSpectatorInputs(1, 128))
	s.PushInputs(129, testSpectatorInputs(129, 127))
	assertEq(t, 255, len(s.pendingFrames))
	s.lastPushAt = time.Unix(100, 0)
	beforePushAt := s.lastPushAt

	// Disjoint future packets cannot grow a window stalled at frame zero.
	for i := 0; i < 100; i++ {
		start := int32(256 + i*128)
		ack, advanced := s.PushInputs(start, testSpectatorInputs(start, 128))
		assertEq(t, int32(0), ack)
		assertEq(t, false, advanced)
		assertEq(t, 255, len(s.pendingFrames))
		assertEq(t, beforePushAt, s.lastPushAt)
	}

	// Redundant peers cannot replace values already buffered in the window.
	s.PushInputs(1, make([]uint64, 128))
	assertEq(t, 255, len(s.pendingFrames))

	ack, advanced := s.PushInputs(0, []uint64{0})
	assertEq(t, int32(256), ack)
	assertEq(t, true, advanced)
	assertEq(t, 0, len(s.pendingFrames))
	assertEq(t, true, slices.Equal(testSpectatorInputs(0, 256), s.log.Inputs))
}

func TestSpectatorSession_PushInputs_BulkSeedPreservesPending(t *testing.T) {
	s := newTestSpectatorSession()
	s.PushInputs(100, []uint64{777, 888})

	// Fake-live can seed a contiguous history larger than a UDP packet.
	ack, advanced := s.PushInputs(0, testSpectatorInputs(0, 10000))
	assertEq(t, int32(10000), ack)
	assertEq(t, true, advanced)
	assertEq(t, 0, len(s.pendingFrames))
	assertEq(t, uint64(777), s.log.Inputs[100])
	assertEq(t, uint64(888), s.log.Inputs[101])

	// A retry overlapping the recorded tail contributes only its new suffix.
	retry := testSpectatorInputs(9990, 128)
	clear(retry[:10])
	ack, advanced = s.PushInputs(9990, retry)
	assertEq(t, int32(10118), ack)
	assertEq(t, true, advanced)
	assertEq(t, 0, len(s.pendingFrames))
	expected := testSpectatorInputs(0, 10118)
	expected[100], expected[101] = 777, 888
	assertEq(t, true, slices.Equal(expected, s.log.Inputs))
}
