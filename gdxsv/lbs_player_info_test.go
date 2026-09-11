package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gdxsv/gdxsv/proto"

	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"
)

func playerInfoTestMatch() []*LbsPeer {
	battle := &LbsBattle{Rule: &DefaultRule}
	var peers []*LbsPeer
	for i, rank := range []int{1, 6, 21, 51} {
		p := &LbsPeer{
			DBUser: DBUser{
				UserID:      fmt.Sprintf("USER%02d", i+1),
				SessionID:   fmt.Sprintf("SESSION%d", i+1),
				Name:        "日本",
				BattleCount: 99999,
				WinCount:    70000,
				LoseCount:   29998,
			},
			Battle:    battle,
			Team:      uint16(i/2 + 1),
			Rank:      rank,
			GameParam: hexbytes("00ff0180"),
			logger:    zap.NewNop(),
		}
		battle.Add(p)
		peers = append(peers, p)
	}
	return peers
}

func TestLbsAskPlayerInfo32Positions(t *testing.T) {
	assertEq(t, CmdID(0x9966), lbsAskPlayerInfo32)
	assertEq(t, "lbsAskPlayerInfo32", lbsAskPlayerInfo32.String())
	peers := playerInfoTestMatch()
	for i, p := range peers {
		for j, target := range peers {
			t.Run(fmt.Sprintf("requester%d/player%d", i+1, j+1), func(t *testing.T) {
				request := NewClientQuestion(lbsAskPlayerInfo).Writer().Write8(byte(j + 1)).Msg()
				request.Seq = []uint16{0, 1, 0x1234, 0xffff}[i]
				legacy := playerInfoTestReply(t, p, request)
				request.Command = lbsAskPlayerInfo32
				reply := playerInfoTestReply(t, p, request)
				assertEq(t, StatusSuccess, reply.Status)
				assertEq(t, len(legacy.Body)+12, len(reply.Body))
				assertEq(t, legacy.Body, reply.Body[:len(legacy.Body)])
				r := reply.Reader()
				assertEq(t, byte(j+1), r.Read8())
				assertEq(t, target.UserID, r.ReadString())
				assertEq(t, hexbytes("93fa967b"), r.ReadBytes())
				assertEq(t, target.GameParam, r.ReadBytes())
				assertEq(t, uint16(14-j), r.Read16()) // rank from this match
				assertEq(t, uint16(65535), r.Read16())
				assertEq(t, uint16(29998), r.Read16())
				assertEq(t, uint16(0), r.Read16()) // draw count
				assertEq(t, uint16(1), r.Read16()) // invalid count
				assertEq(t, uint16(0), r.Read16())
				assertEq(t, target.Team, r.Read16())
				assertEq(t, uint16(0), r.Read16())
				assertEq(t, uint32(99999), r.Read32())
				assertEq(t, uint32(70000), r.Read32())
				assertEq(t, uint32(29998), r.Read32())
				assertEq(t, 0, r.Remaining())
			})
		}
	}
}

func TestLbsAskPlayerInfo32Wire(t *testing.T) {
	p := playerInfoTestMatch()[0]
	p.LoseCount = 29999
	_, request := Deserialize(hexbytes("810199660001abcd00ffffff01"))
	reply := playerInfoTestReply(t, p, request)
	assertEq(t, hexbytes("180299660031abcd00ffffff"+
		"010006555345523031000493fa967b000400ff0180"+
		"000effff752f00000000000000010000"+
		"0001869f000111700000752f"), p.outbuf)
	assertEq(t, StatusSuccess, reply.Status)

	// Exercise the high byte of body_size as well as the nonzero sequence bytes.
	p.Battle.GameParams[0] = make([]byte, 256)
	reply = playerInfoTestReply(t, p, request)
	assertEq(t, hexbytes("18029966012dabcd00ffffff"), p.outbuf[:HeaderSize])
	assertEq(t, 301, len(reply.Body))
}

func TestLbsAskPlayerInfo32Counts(t *testing.T) {
	type counts struct {
		battles, wins, losses int64
	}
	var cases []counts
	for _, n := range []int64{0, 9999, 10000, 65535, 65536, 70000, 99999} {
		// Each legacy count (wins, losses, invalid) must saturate independently.
		cases = append(cases, counts{n, n, 0}, counts{n, 0, n}, counts{n, 0, 0})
	}
	cases = append(cases,
		counts{99999, 70000, 29999},
		counts{99999, 70000, 29998}, // battles are not reconstructed from wins/losses
		counts{math.MaxInt32, math.MaxInt32, 0},
		counts{math.MaxInt32 + 1, math.MaxInt32 + 1, 0},
		counts{math.MaxUint32 - 1, 0, math.MaxUint32 - 1},
		counts{math.MaxUint32, math.MaxUint32, 0},
		counts{math.MaxUint32, 0, math.MaxUint32},
		counts{math.MaxUint32, 0, 0},
		counts{math.MaxUint32, 0x80000000, 0x7fffffff},
		counts{math.MaxUint32, 0x90abcdef, 0x12345678},
	)
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/%d/%d", tc.battles, tc.wins, tc.losses), func(t *testing.T) {
			if int64(int(tc.battles)) != tc.battles {
				t.Skip("DBUser int cannot hold this boundary on a 32-bit host")
			}
			// In-memory source records exercise the entire wire range without DB limits.
			p := playerInfoTestMatch()[0]
			p.BattleCount, p.WinCount, p.LoseCount = int(tc.battles), int(tc.wins), int(tc.losses)
			before := p.DBUser
			request := NewClientQuestion(lbsAskPlayerInfo32).Writer().Write8(1).Msg()
			reply := playerInfoTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			assertEq(t, hexbytes(fmt.Sprintf("%08x%08x%08x", tc.battles, tc.wins, tc.losses)), reply.Body[len(reply.Body)-12:])
			fixed := reply.Body[len(reply.Body)-28 : len(reply.Body)-12]
			for i, n := range []int64{tc.wins, tc.losses, 0, tc.battles - tc.wins - tc.losses} {
				want := uint16(65535)
				if n <= 65535 {
					want = uint16(n)
				}
				assertEq(t, want, binary.BigEndian.Uint16(fixed[2+i*2:]))
			}
			assertEq(t, before, p.DBUser) // saturation must never mutate shared counters
			request.Command = lbsAskPlayerInfo
			legacy := playerInfoTestReply(t, p, request)
			assertEq(t, legacy.Body, reply.Body[:len(reply.Body)-12])
		})
	}
}

func TestLbsAskPlayerInfo32Errors(t *testing.T) {
	tests := []struct {
		name   string
		change func(*LbsPeer, *LbsMessage)
	}{
		{"no match", func(p *LbsPeer, m *LbsMessage) { p.Battle = nil }},
		{"empty match", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users = nil }},
		{"missing position", func(p *LbsPeer, m *LbsMessage) { m.Body = nil; m.BodySize = 0 }},
		{"extra position", func(p *LbsPeer, m *LbsMessage) { m.Body = []byte{1, 2}; m.BodySize = 2 }},
		{"truncated body", func(p *LbsPeer, m *LbsMessage) { m.Body = nil }},
		{"wrong body size", func(p *LbsPeer, m *LbsMessage) { m.BodySize = 2 }},
		{"wrong direction", func(p *LbsPeer, m *LbsMessage) { m.Direction = ServerToClient }},
		{"wrong category", func(p *LbsPeer, m *LbsMessage) { m.Category = CategoryNotice }},
		{"zero position", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 0 }},
		{"position five", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 5 }},
		{"position 255", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 255 }},
		{"position outside current match", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users = p.Battle.Users[:2] }},
		{"nil player", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3] = nil }},
		{"requester outside match", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[0] = nil }},
		{"different session", func(p *LbsPeer, m *LbsMessage) {
			u := p.DBUser
			u.SessionID = "another session"
			p.Battle.Users[0] = &u
		}},
		{"negative battles", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3].BattleCount = -1 }},
		{"negative wins", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3].WinCount = -1 }},
		{"negative losses", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3].LoseCount = -1 }},
		{"negative invalid results", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3].BattleCount = 1 }},
		{"oversized response", func(p *LbsPeer, m *LbsMessage) { p.Battle.GameParams[3] = make([]byte, math.MaxUint16) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := playerInfoTestMatch()[0]
			request := NewClientQuestion(lbsAskPlayerInfo32).Writer().Write8(4).Msg()
			request.Seq = 0xabcd
			tt.change(p, request)
			reply := playerInfoTestReply(t, p, request)
			assertEq(t, StatusError, reply.Status)
			assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
			assertEq(t, hexbytes("ffffffff"), p.outbuf[8:12])
		})
	}

	for _, count := range []int64{math.MaxUint32 + 1, math.MaxInt64, math.MinInt64} {
		for _, field := range []string{"battles", "wins", "losses"} {
			t.Run(fmt.Sprintf("%s/%d", field, count), func(t *testing.T) {
				if int64(int(count)) != count {
					t.Skip("source boundary requires a 64-bit int")
				}
				p := playerInfoTestMatch()[0]
				switch field {
				case "battles":
					p.BattleCount = int(count)
				case "wins":
					p.WinCount = int(count)
				case "losses":
					p.LoseCount = int(count)
				}
				request := NewClientQuestion(lbsAskPlayerInfo32).Writer().Write8(1).Msg()
				reply := playerInfoTestReply(t, p, request)
				assertEq(t, StatusError, reply.Status)
				assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
			})
		}
	}
}

func TestLbsAskPlayerInfo32MatchingAndReplay(t *testing.T) {
	peers := playerInfoTestMatch()
	for _, p := range peers {
		p.conn = &PipeConn{}
		request := NewClientQuestion(lbsAskPlayerInfo32).Writer().Write8(1).Msg()
		assertEq(t, StatusSuccess, playerInfoTestReply(t, p, request).Status)
	}
	lobby := &LbsLobby{Rule: DefaultRule}
	msgs, err := lobby.makeP2PMatchingMsg(peers[0].Battle, peers, nil)
	must(t, err)
	for _, msg := range msgs {
		matching := new(proto.P2PMatching)
		must(t, pb.Unmarshal(msg.Body, matching))
		assertEq(t, 4, len(matching.Users))
		for _, u := range matching.Users {
			assertEq(t, int32(99999), u.BattleCount)
			assertEq(t, int32(70000), u.WinCount)
			assertEq(t, int32(29998), u.LoseCount)
		}
	}

	// Replay fields remain signed int32; this checks five-digit persistence only.
	room := newMcsRoom(nil, &McsGame{BattleCode: "player-info32"})
	for _, p := range peers {
		room.Join(&McsTCPPeer{}, &McsUser{
			UserID: p.UserID, BattleCount: p.BattleCount, WinCount: p.WinCount, LoseCount: p.LoseCount,
		})
	}
	path := filepath.Join(t.TempDir(), "battle.pb")
	must(t, room.saveBattleLogLocked(path))
	data, err := os.ReadFile(path)
	must(t, err)
	replay := new(proto.BattleLogFile)
	must(t, pb.Unmarshal(data, replay))
	assertEq(t, 4, len(replay.Users))
	for _, u := range replay.Users {
		assertEq(t, int32(99999), u.BattleCount)
		assertEq(t, int32(70000), u.WinCount)
		assertEq(t, int32(29998), u.LoseCount)
	}
}

func playerInfoTestReply(t *testing.T, p *LbsPeer, request *LbsMessage) *LbsMessage {
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

func TestLbsAskPlayerInfoLegacy(t *testing.T) {
	for _, disk := range []string{GameDiskDC1, GameDiskDC2, GameDiskPS2} {
		t.Run(disk, func(t *testing.T) {
			p := playerInfoTestMatch()[0]
			p.GameDisk = disk
			request := NewClientQuestion(lbsAskPlayerInfo).Writer().Write8(1).Msg()
			request.Seq = 0x1234
			reply := playerInfoTestReply(t, p, request)
			assertEq(t, StatusSuccess, reply.Status)
			// Position, ID, Shift-JIS name, raw game parameters, then the original
			// grade/wins/losses/draws/invalid/reserved/team/reserved fields.
			assertEq(t, hexbytes("180269130025123400ffffff"+
				"010006555345523031000493fa967b000400ff0180"+
				"000effff752e00000001000000010000"), p.outbuf)

			// Retain even the old handler's treatment of inconsistent data.
			p.BattleCount = 0
			p.WinCount = 1
			p.LoseCount = 0
			reply = playerInfoTestReply(t, p, request)
			assertEq(t, hexbytes("0000000100000000ffff000000010000"), reply.Body[len(reply.Body)-16:])

			p.Battle = nil
			reply = playerInfoTestReply(t, p, request)
			assertEq(t, NewServerAnswer(request).SetErr().Body, reply.Body)
			assertEq(t, StatusError, reply.Status)
		})
	}
}

// A client can send any pos byte; the legacy command must answer with an error
// for anything that is not a participant instead of dereferencing nil (which
// used to panic and, with no recover in eventLoop, kill the whole server).
func TestLbsAskPlayerInfoErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(*LbsPeer, *LbsMessage)
	}{
		{"no match", func(p *LbsPeer, m *LbsMessage) { p.Battle = nil }},
		{"empty match", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users = nil }},
		{"missing position", func(p *LbsPeer, m *LbsMessage) { m.Body = nil; m.BodySize = 0 }},
		{"zero position", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 0 }},
		{"position five", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 5 }},
		{"position 255", func(p *LbsPeer, m *LbsMessage) { m.Body[0] = 255 }},
		{"position outside current match", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users = p.Battle.Users[:2] }},
		{"nil player", func(p *LbsPeer, m *LbsMessage) { p.Battle.Users[3] = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := playerInfoTestMatch()[0]
			request := NewClientQuestion(lbsAskPlayerInfo).Writer().Write8(4).Msg()
			request.Seq = 0xabcd
			tt.change(p, request)
			reply := playerInfoTestReply(t, p, request)
			assertEq(t, StatusError, reply.Status)
			assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
			assertEq(t, hexbytes("ffffffff"), p.outbuf[8:12])
		})
	}
}

// Sweep every possible pos byte against matches of every size: exactly the
// participant positions succeed and every other value is a clean error.
func TestLbsAskPlayerInfoEveryPosition(t *testing.T) {
	for _, cmd := range []CmdID{lbsAskPlayerInfo, lbsAskPlayerInfo32} {
		for players := 0; players <= 4; players++ {
			t.Run(fmt.Sprintf("%s/%dplayers", cmd, players), func(t *testing.T) {
				p := playerInfoTestMatch()[0]
				p.Battle.Users = p.Battle.Users[:players]
				for pos := 0; pos <= math.MaxUint8; pos++ {
					request := NewClientQuestion(cmd).Writer().Write8(byte(pos)).Msg()
					reply := playerInfoTestReply(t, p, request)
					if 1 <= pos && pos <= players {
						assertEq(t, StatusSuccess, reply.Status)
						assertEq(t, byte(pos), reply.Reader().Read8())
					} else {
						assertEq(t, StatusError, reply.Status)
						assertEq(t, NewServerAnswer(request).SetErr().Serialize(), p.outbuf)
					}
				}
			})
		}
	}
}

// writePlayerInfo is shared by both commands, so it guards the nil player
// itself rather than trusting every caller to remember.
func TestWritePlayerInfoGuardsMissingPlayer(t *testing.T) {
	p := playerInfoTestMatch()[0]
	b := p.Battle
	for _, tc := range []struct {
		name string
		b    *LbsBattle
		pos  byte
	}{
		{"nil battle", nil, 1},
		{"zero position", b, 0},
		{"position five", b, 5},
		{"position 255", b, 255},
		{"empty match", &LbsBattle{Rule: &DefaultRule}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewServerAnswer(NewClientQuestion(lbsAskPlayerInfo)).Writer()
			got, ok := writePlayerInfo(w, tc.b, tc.pos, 1, 2, 3)
			assertEq(t, false, ok)
			assertEq(t, w, got)
			assertEq(t, 0, w.BodyLen()) // nothing was written
		})
	}

	t.Run("nil player", func(t *testing.T) {
		b.Users[2] = nil
		w := NewServerAnswer(NewClientQuestion(lbsAskPlayerInfo)).Writer()
		_, ok := writePlayerInfo(w, b, 3, 1, 2, 3)
		assertEq(t, false, ok)
		assertEq(t, 0, w.BodyLen())
		_, ok = writePlayerInfo(w, b, 4, 1, 2, 3)
		assertEq(t, true, ok)
		assertEq(t, byte(4), w.Msg().Reader().Read8())
	})
}
