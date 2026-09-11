package proto

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// newFilledBuffer returns a BattleBuffer holding messages with seq 1..n
// (body "msg-<seq>"), and the filter that generated them. Messages are
// acked incrementally while filling so the ring never overflows; on return
// the send window is [begin, n+1).
func newFilledBuffer(t *testing.T, n int, begin uint32) (*BattleBuffer, *MessageFilter) {
	t.Helper()
	b := NewBattleBuffer("a")
	f := NewMessageFilter([]string{"b"})
	for i := 1; i <= n; i++ {
		b.PushBattleMessage(f.GenerateMessage("a", []byte(fmt.Sprintf("msg-%d", i))))
		// Keep the unacked window well inside the ring while filling.
		if i%(ringSize/2) == 0 && uint32(i) < begin {
			b.ApplySeqAck(0, uint32(i))
		}
	}
	if begin > 1 {
		b.ApplySeqAck(0, begin-1)
	}
	if b.begin != begin || b.end != uint32(n)+1 {
		t.Fatalf("setup: begin=%d end=%d, want begin=%d end=%d", b.begin, b.end, begin, n+1)
	}
	return b, f
}

// checkWindow asserts the invariants every GetSendData result must satisfy:
// no nil entries, at most 50 messages, begin <= end, and the returned
// messages are exactly the contiguous run of seqs starting at begin.
func checkWindow(t *testing.T, b *BattleBuffer, data []*BattleMessage, seq, ack uint32) {
	t.Helper()
	if b.begin > b.end {
		t.Fatalf("begin=%d > end=%d", b.begin, b.end)
	}
	if len(data) > 50 {
		t.Fatalf("GetSendData returned %d messages, want <= 50", len(data))
	}
	want := b.end - b.begin
	if want > 50 {
		want = 50
	}
	if uint32(len(data)) != want {
		t.Fatalf("GetSendData returned %d messages, want %d (begin=%d end=%d)", len(data), want, b.begin, b.end)
	}
	for i, msg := range data {
		if msg == nil {
			t.Fatalf("GetSendData returned nil at index %d (begin=%d end=%d)", i, b.begin, b.end)
		}
		if wantSeq := b.begin + uint32(i); msg.GetSeq() != wantSeq {
			t.Fatalf("data[%d].Seq=%d, want %d (begin=%d end=%d)", i, msg.GetSeq(), wantSeq, b.begin, b.end)
		}
		if wantBody := fmt.Sprintf("msg-%d", msg.GetSeq()); string(msg.GetBody()) != wantBody {
			t.Fatalf("data[%d].Body=%q, want %q", i, msg.GetBody(), wantBody)
		}
	}
	if seq != b.begin+uint32(len(data))-1 {
		t.Fatalf("GetSendData seq=%d, want %d", seq, b.begin+uint32(len(data))-1)
	}
	if ack != b.ack {
		t.Fatalf("GetSendData ack=%d, want %d", ack, b.ack)
	}
}

type windowCase struct {
	name  string
	n     int
	begin uint32
}

// windowCases returns (message count, begin) pairs that exercise both the
// l <= r branch and the ring wraparound (l > r) branch of GetSendData.
func windowCases() []windowCase {
	return []windowCase{
		{"empty", 0, 1},
		{"few-no-wrap", 3, 1},
		{"full-window-no-wrap", 120, 30},
		{"window-straddles-ring-end", ringSize + 20, ringSize - 10},
		{"window-starts-at-ring-end", ringSize + 60, ringSize},
		{"begin-past-first-lap", 2*ringSize + 5, 2*ringSize - 3},
	}
}

func TestBattleBufferGetSendDataNeverReturnsNil(t *testing.T) {
	for _, wc := range windowCases() {
		t.Run(wc.name, func(t *testing.T) {
			end := uint32(wc.n) + 1
			acks := []struct {
				name string
				ack  uint32
			}{
				{"ack=0", 0},
				{"ack=end-1", end - 1},
				{"ack=end", end},
				{"ack=end+1", end + 1},
				{"ack=MaxUint32", math.MaxUint32},
			}
			for _, ac := range acks {
				t.Run(ac.name, func(t *testing.T) {
					b, _ := newFilledBuffer(t, wc.n, wc.begin)
					b.ApplySeqAck(7, ac.ack)
					data, seq, ack := b.GetSendData()
					checkWindow(t, b, data, seq, ack)
					if ack != 7 {
						t.Fatalf("ack=%d, want 7", ack)
					}
				})
			}
		})
	}
}

func TestBattleBufferGetSendDataCap(t *testing.T) {
	const n = 200
	b, _ := newFilledBuffer(t, n, 1)
	for _, ack := range []uint32{0, 1, 49, 50, 51, 149, n, n + 1, n + 1000, math.MaxUint32} {
		b.ApplySeqAck(0, ack)
		data, seq, gotAck := b.GetSendData()
		checkWindow(t, b, data, seq, gotAck)
		if len(data) > 50 {
			t.Fatalf("ack=%d: got %d messages, want <= 50", ack, len(data))
		}
	}
}

func TestBattleBufferApplySeqAck(t *testing.T) {
	const n = 10 // end == 11
	tests := []struct {
		name      string
		begin     uint32
		ack       uint32
		wantBegin uint32
	}{
		{"ack=0 leaves begin at 1", 1, 0, 1},
		{"ack advances begin", 1, 3, 4},
		{"ack=end-1 drains window", 1, n, n + 1},
		{"ack=end is ignored", 1, n + 1, 1},
		{"ack=end+1 is ignored", 1, n + 2, 1},
		{"ack=MaxUint32 is ignored", 1, math.MaxUint32, 1},
		{"ack=MaxUint32 with advanced begin", 5, math.MaxUint32, 5},
		{"ack behind begin is ignored", 5, 2, 5},
		{"ack==begin-1 is a no-op", 5, 4, 5},
		{"ack ahead of begin advances", 5, 8, 9},
		{"ack=end with advanced begin is ignored", 5, n + 1, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newFilledBuffer(t, n, tt.begin)
			b.ApplySeqAck(42, tt.ack)
			if b.begin != tt.wantBegin {
				t.Fatalf("begin=%d, want %d", b.begin, tt.wantBegin)
			}
			if b.begin > b.end {
				t.Fatalf("begin=%d > end=%d", b.begin, b.end)
			}
			if b.ack != 42 {
				t.Fatalf("ack=%d, want 42", b.ack)
			}
			data, seq, ack := b.GetSendData()
			checkWindow(t, b, data, seq, ack)
		})
	}
}

// TestBattleBufferRingWraparound drives a sender/receiver pair through
// several laps of the ring, acking incrementally and with some datagrams
// "lost", and asserts the receiver sees every body exactly once, in order.
func TestBattleBufferRingWraparound(t *testing.T) {
	const total = 3*ringSize + 123
	rng := rand.New(rand.NewSource(409))

	sender := NewBattleBuffer("a")
	sf := NewMessageFilter([]string{"b"})
	rf := NewMessageFilter([]string{"a"})

	pushed := 0
	var received []string
	nextSeq := uint32(1)
	for pushed < total || sender.begin < sender.end {
		// Push a small batch, never letting the unacked window exceed the ring.
		batch := rng.Intn(40)
		for i := 0; i < batch && pushed < total && sender.end-sender.begin < ringSize; i++ {
			pushed++
			sender.PushBattleMessage(sf.GenerateMessage("a", []byte(fmt.Sprintf("msg-%d", pushed))))
		}

		data, seq, ack := sender.GetSendData()
		checkWindow(t, sender, data, seq, ack)

		// Drop ~1/4 of the datagrams: the sender must resend from begin.
		if rng.Intn(4) == 0 {
			continue
		}
		for _, msg := range data {
			if rf.Filter(msg) {
				if msg.GetSeq() != nextSeq {
					t.Fatalf("received seq %d, want %d", msg.GetSeq(), nextSeq)
				}
				nextSeq++
				received = append(received, string(msg.GetBody()))
			}
		}
		// The receiver acks the last seq it has; sometimes an older ack
		// (reordered datagram) arrives, which must not move begin backwards.
		lastRecv := nextSeq - 1
		if rng.Intn(5) == 0 && lastRecv > 0 {
			lastRecv -= uint32(rng.Intn(int(lastRecv) + 1))
		}
		sender.ApplySeqAck(0, lastRecv)
		if sender.begin > sender.end {
			t.Fatalf("begin=%d > end=%d", sender.begin, sender.end)
		}
	}

	if len(received) != total {
		t.Fatalf("received %d bodies, want %d", len(received), total)
	}
	for i, body := range received {
		if want := fmt.Sprintf("msg-%d", i+1); body != want {
			t.Fatalf("received[%d]=%q, want %q", i, body, want)
		}
	}
	if sender.begin != uint32(total)+1 || sender.end != uint32(total)+1 {
		t.Fatalf("final begin=%d end=%d, want both %d", sender.begin, sender.end, total+1)
	}
}

// TestBattleBufferRandomizedInvariants is a deterministic randomized loop
// that interleaves pushes, sends and (mostly bogus) acks and checks the
// send-window invariants after every step.
func TestBattleBufferRandomizedInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := NewBattleBuffer("a")
	f := NewMessageFilter([]string{"b"})
	pushed := 0

	for i := 0; i < 20000; i++ {
		switch rng.Intn(3) {
		case 0:
			n := rng.Intn(8)
			for j := 0; j < n && b.end-b.begin < ringSize; j++ {
				pushed++
				b.PushBattleMessage(f.GenerateMessage("a", []byte(fmt.Sprintf("msg-%d", pushed))))
			}
		case 1:
			prevBegin := b.begin
			var ack uint32
			switch rng.Intn(4) {
			case 0: // legitimate cumulative ack inside the window
				if b.end > b.begin {
					ack = b.begin + uint32(rng.Intn(int(b.end-b.begin)))
				}
			case 1: // stale ack
				ack = uint32(rng.Intn(int(b.begin)))
			case 2: // ack for messages never sent
				ack = b.end + uint32(rng.Intn(3000))
			case 3: // extreme values
				ack = []uint32{0, math.MaxUint32, math.MaxUint32 - 1, b.end, b.end - 1}[rng.Intn(5)]
			}
			b.ApplySeqAck(rng.Uint32(), ack)
			if b.begin > b.end {
				t.Fatalf("step %d: ack=%d: begin=%d > end=%d", i, ack, b.begin, b.end)
			}
			if b.begin < prevBegin {
				t.Fatalf("step %d: ack=%d moved begin backwards %d -> %d", i, ack, prevBegin, b.begin)
			}
			if ack < b.end && ack+1 > prevBegin && b.begin != ack+1 {
				t.Fatalf("step %d: valid ack=%d not applied: begin=%d", i, ack, b.begin)
			}
		}
		data, seq, ack := b.GetSendData()
		checkWindow(t, b, data, seq, ack)
	}
}

func FuzzBattleBufferApplySeqAck(f *testing.F) {
	f.Add(uint8(3), uint32(0), uint32(math.MaxUint32))
	f.Add(uint8(3), uint32(0), uint32(10))
	f.Add(uint8(0), uint32(0), uint32(0))
	f.Add(uint8(255), uint32(100), uint32(50))
	f.Add(uint8(255), uint32(200), uint32(300))
	f.Fuzz(func(t *testing.T, n uint8, firstAck, secondAck uint32) {
		b := NewBattleBuffer("a")
		mf := NewMessageFilter([]string{"b"})
		for i := 1; i <= int(n); i++ {
			b.PushBattleMessage(mf.GenerateMessage("a", []byte(fmt.Sprintf("msg-%d", i))))
		}
		for _, ack := range []uint32{firstAck, secondAck} {
			b.ApplySeqAck(1, ack)
			data, seq, gotAck := b.GetSendData()
			checkWindow(t, b, data, seq, gotAck)
		}
	})
}

func TestMessageFilterSetAcceptIDsReplaces(t *testing.T) {
	// McsUDPPeer creates its filter with a placeholder "" ID and later
	// calls SetAcceptIDs with the real user ID; "" must stop being accepted.
	f := NewMessageFilter([]string{""})
	f.SetAcceptIDs([]string{"user-A"})

	if f.Filter(&BattleMessage{UserId: "", Seq: 1, Body: []byte("x")}) {
		t.Fatal("Filter accepted a message with an empty user_id after SetAcceptIDs")
	}
	if !f.Filter(&BattleMessage{UserId: "user-A", Seq: 1, Body: []byte("x")}) {
		t.Fatal("Filter rejected the first message from the accepted user")
	}
	if f.Filter(&BattleMessage{UserId: "user-B", Seq: 1, Body: []byte("x")}) {
		t.Fatal("Filter accepted a message from a user that was never accepted")
	}

	// Re-applying the same ID must keep its receive state (no replay window
	// reset), and dropping it must reject it.
	f.SetAcceptIDs([]string{"user-A"})
	if f.Filter(&BattleMessage{UserId: "user-A", Seq: 1}) {
		t.Fatal("SetAcceptIDs with the same ID reset the receive state (seq 1 replayed)")
	}
	if !f.Filter(&BattleMessage{UserId: "user-A", Seq: 2}) {
		t.Fatal("Filter rejected the next in-order message after SetAcceptIDs")
	}
	f.SetAcceptIDs([]string{"user-B"})
	if f.Filter(&BattleMessage{UserId: "user-A", Seq: 3}) {
		t.Fatal("Filter accepted a message from a user removed by SetAcceptIDs")
	}
	if !f.Filter(&BattleMessage{UserId: "user-B", Seq: 1}) {
		t.Fatal("Filter rejected the first message from the newly accepted user")
	}
}

type filterStep struct {
	seq  uint32
	want bool
}

func TestMessageFilterRejectsSeqZero(t *testing.T) {
	tests := []struct {
		name  string
		steps []filterStep
	}{
		{"seq 0 first, then in-order", []filterStep{
			{0, false}, {1, true}, {9999, false}, {0, false}, {2, true},
		}},
		{"seq 0 after a valid message", []filterStep{
			{1, true}, {0, false}, {9999, false}, {2, true}, {2, false}, {3, true},
		}},
		{"first seen seq starts the window", []filterStep{
			{5, true}, {0, false}, {7, false}, {6, true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewMessageFilter([]string{"user-A"})
			for i, m := range tt.steps {
				got := f.Filter(&BattleMessage{UserId: "user-A", Seq: m.seq, Body: []byte("x")})
				if got != m.want {
					t.Fatalf("steps[%d] seq=%d: Filter=%v, want %v", i, m.seq, got, m.want)
				}
			}
		})
	}
}
