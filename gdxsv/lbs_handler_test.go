package main

import (
	"bytes"
	"compress/zlib"
	"testing"
	"time"

	pb "gdxsv/gdxsv/proto"

	"google.golang.org/protobuf/proto"
)

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

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify database
	for _, uid := range []string{"U1", "U2"} {
		rec, err := getDB().GetBattleRecordUser(battleCode, uid)
		must(t, err)
		assertEq(t, "1,1", rec.RoundWin)
		if uid == "U1" {
			assertEq(t, "10,11", rec.UsedMsList)
			assertEq(t, (1<<10)|(1<<11), rec.UsedMsMask)
		} else {
			assertEq(t, "20,21", rec.UsedMsList)
			assertEq(t, (1<<20)|(1<<21), rec.UsedMsMask)
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

