package main

import (
	"bytes"
	"compress/zlib"
	"math"
	"testing"
	"time"

	pb "gdxsv/gdxsv/proto"

	"google.golang.org/protobuf/proto"
)

func Test_decideGrade(t *testing.T) {
	tests := []struct {
		name     string
		winCount int
		rank     int
		want     uint8
	}{
		{"rank=0 returns 0", 1000, 0, 0},
		{"winCount=0 returns 0", 0, 1, 0},
		{"winCount=99 returns 0", 99, 1, 0},
		{"winCount=100 returns 1", 100, 1, 1},
		{"winCount=500 returns 5", 500, 50, 5},
		{"winCount=1099 returns 10", 1099, 1, 10},
		{"winCount=1100 returns 11 (grade<12 no rank check)", 1100, 100, 11},
		{"winCount=1199 returns 11", 1199, 100, 11},
		{"winCount=1200 rank=1 returns 14 (大将)", 1200, 1, 14},
		{"winCount=1200 rank=5 returns 14 (大将)", 1200, 5, 14},
		{"winCount=1200 rank=6 returns 13 (中将)", 1200, 6, 13},
		{"winCount=1200 rank=10 returns 13 (中将)", 1200, 10, 13},
		{"winCount=1200 rank=20 returns 13 (中将)", 1200, 20, 13},
		{"winCount=1200 rank=21 returns 12 (少将)", 1200, 21, 12},
		{"winCount=1200 rank=30 returns 12 (少将)", 1200, 30, 12},
		{"winCount=1200 rank=50 returns 12 (少将)", 1200, 50, 12},
		{"winCount=1200 rank=51 returns 11 (大佐)", 1200, 51, 11},
		{"winCount=1200 rank=100 returns 11 (大佐)", 1200, 100, 11},
		{"winCount=1500 rank=1 returns 14 (capped)", 1500, 1, 14},
		{"winCount=2000 rank=1 returns 14 (capped)", 2000, 1, 14},
		{"winCount=1500 rank=999 returns 11 (大佐)", 1500, 999, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideGrade(tt.winCount, tt.rank)
			assertEq(t, tt.want, got)
		})
	}
}

func Test_r16(t *testing.T) {
	tests := []struct {
		name string
		a    int
		want uint16
	}{
		{"zero", 0, 0},
		{"normal value", 1000, 1000},
		{"max uint16", math.MaxUint16, math.MaxUint16},
		{"max uint16 + 1 clamps", math.MaxUint16 + 1, math.MaxUint16},
		{"large value clamps", 100000, math.MaxUint16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r16(tt.a)
			assertEq(t, tt.want, got)
		})
	}
}

func Test_isOldFlycastVersion(t *testing.T) {
	type args struct {
		userVersion string
	}
	tests := []struct {
		name            string
		requiredVersion string
		args            args
		want            bool
	}{
		{
			name:            "same version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v0.7.0"},
			want:            false,
		},
		{
			name:            "old version ng",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v0.6.9"},
			want:            true,
		},
		{
			name:            "new version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "v9.9.9"},
			want:            false,
		},
		{
			name:            "gdxsv prefix version ok",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "gdxsv-0.7.0"},
			want:            false,
		},
		{
			name:            "gdxsv prefix version ng",
			requiredVersion: "v0.7.0",
			args:            args{userVersion: "gdxsv-0.6.9"},
			want:            true,
		},
		{
			name:            "real semver dev version is old",
			requiredVersion: "v1.0.4",
			args:            args{userVersion: "v1.0.4-dev.3+100e01d4"},
			want:            true,
		},
	}

	backup := requiredFlycastVersion
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredFlycastVersion = tt.requiredVersion
			if got := isOldFlycastVersion(tt.args.userVersion, requiredFlycastVersion); got != tt.want {
				t.Errorf("isOldFlycastVersion() = %v, want %v", got, tt.want)
			}
		})
	}
	requiredFlycastVersion = backup
}

func TestLbs_P2PMatchingReport(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	user1, cancel1 := prepareLoggedInUser(t, lbs, PlatformConsole, GameDiskDC2, DBUser{
		UserID: "U1",
		Name:   "N1",
	})
	defer cancel1()

	battleCode := "testreport"
	// Setup 2 players in database
	for i, uid := range []string{"U1", "U2"} {
		must(t, getDB().AddBattleRecord(&BattleRecord{
			BattleCode: battleCode,
			UserID:     uid,
			Pos:        i + 1,
			Players:    2,
			Team:       i + 1,
			ReplayURL:  "http://example.com/replay",
		}))
	}

	report := &pb.P2PMatchingReport{
		BattleCode:  battleCode,
		CloseReason: "game_end",
		PlayerCount: 2,
		RoundData: []*pb.BattleLogRound{
			{WinTeam: 1, UsedMs: []int32{10, 20}}, // U1 used MS 10, U2 used MS 20
			{WinTeam: 1, UsedMs: []int32{11, 21}},
		},
	}

	bin, _ := proto.Marshal(report)
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, err := zw.Write(bin)
	must(t, err)
	zw.Close()

	user1.MustWriteMessage(&LbsMessage{
		Command:  lbsP2PMatchingReport,
		BodySize: uint16(buf.Len()),
		Body:     buf.Bytes(),
	})

	// Wait for async processing to complete
	waitFor(t, 2*time.Second, func() bool {
		rec, err := getDB().GetBattleRecordUser(battleCode, "U1")
		return err == nil && rec.RoundWin != ""
	})

	// Verify database
	for _, uid := range []string{"U1", "U2"} {
		rec, err := getDB().GetBattleRecordUser(battleCode, uid)
		must(t, err)
		assertEq(t, "1,1", rec.RoundWin)
		if uid == "U1" {
			assertEq(t, "10,11", rec.UsedMsList)
			assertEq(t, uint64((1<<10)|(1<<11)), rec.UsedMsMask)
		} else {
			assertEq(t, "20,21", rec.UsedMsList)
			assertEq(t, uint64((1<<20)|(1<<21)), rec.UsedMsMask)
		}
	}

	// Verify FindReplay uses RoundWin for scoring
	q := NewFindReplayQuery()
	q.BattleCode = battleCode
	replays, err := getDB().FindReplay(q)
	must(t, err)
	assertEq(t, 1, len(replays))
	assertEq(t, 2, replays[0].RenpoWin) // 2 rounds won by team 1
	assertEq(t, 0, replays[0].ZeonWin)
}

