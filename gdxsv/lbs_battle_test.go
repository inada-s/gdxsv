package main

import (
	"net"
	"testing"
	"unicode"
)

func Test_toIPPort(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    net.IP
		want1   uint16
		wantErr bool
	}{
		{"tcp4 addr", args{"192.168.1.10:1234"}, net.IPv4(192, 168, 1, 10), 1234, false},
		{"localhost", args{"localhost:1234"}, net.IPv4(127, 0, 0, 1), 1234, false},
		{"missing port", args{"192.168.1.10"}, nil, 0, true},
		{"bad port", args{"192.168.1.10:badport"}, nil, 0, true},
		{"empty string", args{""}, nil, 0, true},
		{"port out of range", args{"192.168.1.10:99999"}, nil, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := toIPPort(tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("toIPPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("toIPPort() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("toIPPort() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func newTestBattle() *LbsBattle {
	return &LbsBattle{
		Users:      make([]*DBUser, 0),
		UserRanks:  make([]int, 0),
		GameParams: make([][]byte, 0),
		RenpoIDs:   make([]string, 0),
		ZeonIDs:    make([]string, 0),
	}
}

func TestLbsBattle_Add_And_NumOfEntryUsers(t *testing.T) {
	b := newTestBattle()
	assertEq(t, uint16(0), b.NumOfEntryUsers())

	b.Add(&LbsPeer{
		DBUser:    DBUser{UserID: "R1"},
		Team:      TeamRenpo,
		GameParam: []byte{1, 2},
		Rank:      10,
	})
	assertEq(t, uint16(1), b.NumOfEntryUsers())
	assertEq(t, 1, len(b.RenpoIDs))
	assertEq(t, 0, len(b.ZeonIDs))

	b.Add(&LbsPeer{
		DBUser:    DBUser{UserID: "Z1"},
		Team:      TeamZeon,
		GameParam: []byte{3, 4},
		Rank:      20,
	})
	assertEq(t, uint16(2), b.NumOfEntryUsers())
	assertEq(t, 1, len(b.RenpoIDs))
	assertEq(t, 1, len(b.ZeonIDs))

	b.Add(&LbsPeer{
		DBUser:    DBUser{UserID: "R2"},
		Team:      TeamRenpo,
		GameParam: []byte{5, 6},
		Rank:      30,
	})
	b.Add(&LbsPeer{
		DBUser:    DBUser{UserID: "Z2"},
		Team:      TeamZeon,
		GameParam: []byte{7, 8},
		Rank:      40,
	})
	assertEq(t, uint16(4), b.NumOfEntryUsers())
	assertEq(t, 2, len(b.RenpoIDs))
	assertEq(t, 2, len(b.ZeonIDs))
	assertEq(t, 4, len(b.Users))
	assertEq(t, 4, len(b.GameParams))
	assertEq(t, 4, len(b.UserRanks))
}

func TestLbsBattle_GetUserTeam(t *testing.T) {
	b := newTestBattle()
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R1"}, Team: TeamRenpo})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "Z1"}, Team: TeamZeon})

	assertEq(t, uint16(1), b.GetUserTeam("R1"))
	assertEq(t, uint16(2), b.GetUserTeam("Z1"))
	assertEq(t, uint16(0), b.GetUserTeam("UNKNOWN"))
}

func TestLbsBattle_GetPosition(t *testing.T) {
	b := newTestBattle()

	// Empty battle
	assertEq(t, byte(0), b.GetPosition("R1"))

	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R1"}, Team: TeamRenpo})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R2"}, Team: TeamRenpo})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "Z1"}, Team: TeamZeon})

	// 1-indexed positions
	assertEq(t, byte(1), b.GetPosition("R1"))
	assertEq(t, byte(2), b.GetPosition("R2"))
	assertEq(t, byte(3), b.GetPosition("Z1"))

	// Not found
	assertEq(t, byte(0), b.GetPosition("UNKNOWN"))
}

func TestLbsBattle_GetUserByPos(t *testing.T) {
	b := newTestBattle()
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R1"}, Team: TeamRenpo})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "Z1"}, Team: TeamZeon})

	// pos=1 → first user
	u := b.GetUserByPos(1)
	assertEq(t, "R1", u.UserID)

	// pos=2 → second user
	u = b.GetUserByPos(2)
	assertEq(t, "Z1", u.UserID)

	// pos=0 → after decrement becomes 255 (underflow), should return nil due to bounds check
	// Note: pos-- makes 0 become 255 (byte underflow), so len(Users) < 255 → nil
	u = b.GetUserByPos(0)
	assertEq(t, (*DBUser)(nil), u)

	// pos=3 with 2 users: after decrement pos=2, len(Users)=2, out of bounds → nil
	u = b.GetUserByPos(3)
	assertEq(t, (*DBUser)(nil), u)
}

func TestLbsBattle_GetGameParamByPos(t *testing.T) {
	b := newTestBattle()
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R1"}, Team: TeamRenpo, GameParam: []byte{0xAA}})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "Z1"}, Team: TeamZeon, GameParam: []byte{0xBB}})

	assertEq(t, []byte{0xAA}, b.GetGameParamByPos(1))
	assertEq(t, []byte{0xBB}, b.GetGameParamByPos(2))

	// pos=3 with 2 params: out of bounds → nil
	assertEq(t, ([]byte)(nil), b.GetGameParamByPos(3))
}

func TestLbsBattle_GetUserRankByPos(t *testing.T) {
	b := newTestBattle()
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "R1"}, Team: TeamRenpo, Rank: 5})
	b.Add(&LbsPeer{DBUser: DBUser{UserID: "Z1"}, Team: TeamZeon, Rank: 15})

	assertEq(t, 5, b.GetUserRankByPos(1))
	assertEq(t, 15, b.GetUserRankByPos(2))

	// pos=3 with 2 ranks: out of bounds → 0
	assertEq(t, 0, b.GetUserRankByPos(3))
}

func Test_genBattleCode(t *testing.T) {
	code := genBattleCode()

	// Must be exactly 13 characters
	assertEq(t, BattleCodeLength, len(code))

	// Must contain only digits
	for _, c := range code {
		if !unicode.IsDigit(c) {
			t.Errorf("genBattleCode() contains non-digit character: %c", c)
		}
	}

	// Calling twice should produce different codes (or at least valid ones)
	code2 := genBattleCode()
	assertEq(t, BattleCodeLength, len(code2))
}
