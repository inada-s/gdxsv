package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"go.uber.org/zap"
)

func winLoseTestPeer() *LbsPeer {
	return &LbsPeer{
		DBUser: DBUser{UserID: "WIN001", BattleCount: 99999, WinCount: 88888, LoseCount: 11111},
		Rank:   1,
		logger: zap.NewNop(),
	}
}

func winLoseTestReply(t *testing.T, p *LbsPeer, request *LbsMessage) *LbsMessage {
	t.Helper()
	p.outbuf = nil
	defaultLbsHandlers[request.Command](p, request)
	n, reply := Deserialize(p.outbuf)
	if reply == nil || n != len(p.outbuf) {
		t.Fatalf("expected one complete reply, got %x", p.outbuf)
	}
	assertEq(t, ServerToClient, reply.Direction)
	assertEq(t, CategoryAnswer, reply.Category)
	assertEq(t, request.Command, reply.Command)
	assertEq(t, request.Seq, reply.Seq)
	assertEq(t, len(reply.Body), int(reply.BodySize))
	return reply
}

func TestLbsWinLoseLegacy(t *testing.T) {
	for _, disk := range []string{GameDiskDC1, GameDiskDC2, GameDiskPS2} {
		t.Run(disk, func(t *testing.T) {
			p := winLoseTestPeer()
			p.GameDisk = disk
			request := NewClientQuestion(lbsWinLose).Writer().Write8(0).Msg()
			request.Seq = 0xabcd
			reply := winLoseTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			assertEq(t, hexbytes("180261450012abcd00ffffff"+
				"000effff2b67000000000000000000000000"), p.outbuf)

			// Preserve legacy handling of missing/extra bytes and other categories.
			for _, body := range [][]byte{nil, {0, 1}} {
				request.Body, request.BodySize = body, uint16(len(body))
				assertEq(t, reply.Body, winLoseTestReply(t, p, request).Body)
			}
			for _, category := range []byte{1, 255} {
				request.Body, request.BodySize = []byte{category}, 1
				reply = winLoseTestReply(t, p, request)
				assertEq(t, StatusSuccess, reply.Status)
				assertEq(t, hexbytes("000100640064006400000000000100000001"), reply.Body)
			}

			// The new API's validation must not change old negative-count behavior.
			p.BattleCount, p.WinCount, p.LoseCount = 0, 1, 0
			request.Body = []byte{0}
			reply = winLoseTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			assertEq(t, hexbytes("0000000100000000ffff0000000000000000"), reply.Body)
		})
	}
}

func TestLbsWinLose32Wire(t *testing.T) {
	assertEq(t, CmdID(0x9967), lbsWinLose32)
	assertEq(t, "lbsWinLose32", lbsWinLose32.String())
	p := winLoseTestPeer()
	_, request := Deserialize(hexbytes("810199670001abcd00ffffff00"))
	reply := winLoseTestReply(t, p, request)
	assertEq(t, StatusSuccess, reply.Status)
	assertEq(t, hexbytes("180299670022abcd00ffffff"+
		"000effff2b67000000000000000000000000"+
		"00015b3800002b670000000000000000"), p.outbuf)

	for _, seq := range []uint16{0, 1, 0xffff} {
		request.Seq = seq
		reply = winLoseTestReply(t, p, request)
		assertEq(t, StatusSuccess, reply.Status)
		assertEq(t, hexbytes(fmt.Sprintf("180299670022%04x00ffffff", seq)), p.outbuf[:HeaderSize])
		assertEq(t, uint16(34), reply.BodySize)
	}
}

func TestLbsWinLose32Grade(t *testing.T) {
	for _, tc := range []struct {
		rank  int
		grade uint16
	}{{0, 0}, {1, 14}, {6, 13}, {21, 12}, {51, 11}} {
		t.Run(fmt.Sprint(tc.rank), func(t *testing.T) {
			p := winLoseTestPeer()
			p.Rank = tc.rank
			request := NewClientQuestion(lbsWinLose32).Writer().Write8(0).Msg()
			reply := winLoseTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			assertEq(t, tc.grade, binary.BigEndian.Uint16(reply.Body))
			request.Command = lbsWinLose
			legacy := winLoseTestReply(t, p, request)
			assertEq(t, legacy.Body, reply.Body[:18])
		})
	}
}

func TestLbsWinLose32Counts(t *testing.T) {
	type counts struct{ wins, losses, invalid int64 }
	var cases []counts
	for _, n := range []int64{0, 9999, 10000, 65535, 65536, 77777, 88888, 99999,
		math.MaxInt32, math.MaxInt32 + 1, math.MaxUint32 - 1, math.MaxUint32} {
		cases = append(cases, counts{n, 0, 0}, counts{0, n, 0}, counts{0, 0, n})
	}
	cases = append(cases,
		counts{88888, 11111, 0},
		counts{70000, 40000, 12},
		counts{36536, 25267, 9},
		counts{math.MaxUint32, math.MaxUint32, math.MaxUint32},
		counts{0x90abcdef, 0x12345678, 0xabcdef01},
	)
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/%d/%d", tc.wins, tc.losses, tc.invalid), func(t *testing.T) {
			battles := tc.wins + tc.losses + tc.invalid
			if int64(int(battles)) != battles {
				t.Skip("source boundary requires a 64-bit int")
			}
			// Exercise the wire range without database limitations, including totals
			// above uint32 whose individual counters are each still representable.
			p := winLoseTestPeer()
			p.BattleCount, p.WinCount, p.LoseCount = int(battles), int(tc.wins), int(tc.losses)
			before := p.DBUser
			request := NewClientQuestion(lbsWinLose32).Writer().Write8(0).Msg()
			reply := winLoseTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			assertEq(t, uint16(34), reply.BodySize)
			assertEq(t, hexbytes(fmt.Sprintf("%08x%08x00000000%08x", tc.wins, tc.losses, tc.invalid)), reply.Body[18:])
			for i, n := range []int64{tc.wins, tc.losses, 0, tc.invalid} {
				want := uint16(65535)
				if n <= 65535 {
					want = uint16(n)
				}
				assertEq(t, want, binary.BigEndian.Uint16(reply.Body[2+i*2:]))
			}
			assertEq(t, make([]byte, 8), reply.Body[10:18]) // battle points unchanged
			assertEq(t, before, p.DBUser)
			request.Command = lbsWinLose
			legacy := winLoseTestReply(t, p, request)
			assertEq(t, StatusSuccess, legacy.Status)
			assertEq(t, legacy.Body, reply.Body[:18])
		})
	}
}

func TestLbsWinLose32RequestErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*LbsMessage)
	}{
		{"missing category", func(m *LbsMessage) { m.Body, m.BodySize = nil, 0 }},
		{"extra byte", func(m *LbsMessage) { m.Body, m.BodySize = []byte{0, 0}, 2 }},
		{"truncated body", func(m *LbsMessage) { m.Body = nil }},
		{"wrong body size", func(m *LbsMessage) { m.BodySize = 2 }},
		{"unsupported category one", func(m *LbsMessage) { m.Body[0] = 1 }},
		{"unsupported category 255", func(m *LbsMessage) { m.Body[0] = 255 }},
		{"wrong direction", func(m *LbsMessage) { m.Direction = ServerToClient }},
		{"wrong command category", func(m *LbsMessage) { m.Category = CategoryNotice }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := winLoseTestPeer()
			request := NewClientQuestion(lbsWinLose32).Writer().Write8(0).Msg()
			request.Seq = 0xabcd
			tt.change(request)
			reply := winLoseTestReply(t, p, request)
			assertEq(t, StatusError, reply.Status)
			assertEq(t, hexbytes("ffffffff"), p.outbuf[8:12])
			assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
		})
	}
}

func TestLbsWinLose32CounterErrors(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		battles, wins, losses int64
	}{
		{"negative battles", -1, 0, 0},
		{"negative wins", 0, -1, 0},
		{"negative losses", 0, 0, -1},
		{"negative invalid", 10, 6, 5},
		{"wins overflow", math.MaxUint32 + 1, math.MaxUint32 + 1, 0},
		{"losses overflow", math.MaxUint32 + 1, 0, math.MaxUint32 + 1},
		{"invalid overflow", math.MaxUint32 + 1, 0, 0},
		{"large negative invalid", 0, math.MaxUint32, math.MaxUint32},
		{"maximum signed battles", math.MaxInt64, math.MaxUint32, math.MaxUint32},
		{"maximum signed wins", math.MaxInt64, math.MaxInt64, 0},
		{"maximum signed losses", math.MaxInt64, 0, math.MaxInt64},
		{"minimum signed battles", math.MinInt64, 1, 1},
		{"minimum signed wins", 0, math.MinInt64, 0},
		{"minimum signed losses", 0, 0, math.MinInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range []int64{tc.battles, tc.wins, tc.losses} {
				if int64(int(n)) != n {
					t.Skip("source boundary requires a 64-bit int")
				}
			}
			p := winLoseTestPeer()
			p.BattleCount, p.WinCount, p.LoseCount = int(tc.battles), int(tc.wins), int(tc.losses)
			before := p.DBUser
			request := NewClientQuestion(lbsWinLose32).Writer().Write8(0).Msg()
			request.Seq = 0xffff
			reply := winLoseTestReply(t, p, request)
			assertEq(t, StatusError, reply.Status)
			assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
			assertEq(t, before, p.DBUser)
		})
	}
}

func TestLbsWinLose32Flow(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	for i, user := range []DBUser{
		{UserID: "WIN001", BattleCount: 99999, WinCount: 88888, LoseCount: 11111},
		{UserID: "WIN002", BattleCount: 77777, WinCount: 0, LoseCount: 77777},
	} {
		cli, cancel := prepareLoggedInUser(t, lbs, PlatformEmuX8664, GameDiskDC2, user)
		defer cancel()
		var legacyBody []byte
		// Exercise both commands through normal LBS framing and dispatch, using
		// each requester's personal statistics without requiring a current match.
		for _, cmd := range []CmdID{lbsWinLose, lbsWinLose32, lbsWinLose} {
			request := NewClientQuestion(cmd).Writer().Write8(0).Msg()
			request.Seq = uint16(0x1234 + i)
			cli.MustWriteMessage(request)
			reply := cli.MustReadMessageSkipNotice()
			assertEq(t, ServerToClient, reply.Direction)
			assertEq(t, CategoryAnswer, reply.Category)
			assertEq(t, cmd, reply.Command)
			assertEq(t, request.Seq, reply.Seq)
			assertEq(t, StatusSuccess, reply.Status)
			if cmd == lbsWinLose {
				assertEq(t, uint16(18), reply.BodySize)
				if legacyBody == nil {
					legacyBody = reply.Body
				} else {
					assertEq(t, legacyBody, reply.Body)
				}
			} else {
				assertEq(t, uint16(34), reply.BodySize)
				assertEq(t, legacyBody, reply.Body[:18])
				assertEq(t, hexbytes(fmt.Sprintf("%08x%08x0000000000000000", user.WinCount, user.LoseCount)), reply.Body[18:])
			}
		}
	}
}
