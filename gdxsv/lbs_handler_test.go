package main

import (
	"bytes"
	"compress/zlib"
	"math"
	"testing"
	"time"

	pb "gdxsv/gdxsv/proto"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

	// A legacy peer on the other team reports the opposite winner for a
	// timeout. Reconcile stored rounds rather than replacing the first report.
	user2, cancel2 := prepareLoggedInUser(t, lbs, PlatformConsole, GameDiskDC2, DBUser{
		UserID: "U2",
		Name:   "N2",
	})
	defer cancel2()
	report.RoundData[0].WinTeam = 2
	bin, err = proto.Marshal(report)
	must(t, err)
	buf.Reset()
	zw = zlib.NewWriter(&buf)
	_, err = zw.Write(bin)
	must(t, err)
	must(t, zw.Close())
	user2.MustWriteMessage(&LbsMessage{
		Command:  lbsP2PMatchingReport,
		BodySize: uint16(buf.Len()),
		Body:     buf.Bytes(),
	})
	waitFor(t, 2*time.Second, func() bool {
		rec, err := getDB().GetBattleRecordUser(battleCode, "U2")
		return err == nil && rec.RoundWin == "-1,1"
	})
	for _, uid := range []string{"U1", "U2"} {
		rec, err := getDB().GetBattleRecordUser(battleCode, uid)
		must(t, err)
		assertEq(t, "-1,1", rec.RoundWin)
	}
	replays, err = getDB().FindReplay(q)
	must(t, err)
	assertEq(t, 1, replays[0].RenpoWin)
	assertEq(t, 0, replays[0].ZeonWin)
}

func Test_p2pMatchingReportFields(t *testing.T) {
	report := &pb.P2PMatchingReport{
		BattleCode:        "testreporttiming",
		CloseReason:       "game_end",
		PlayerCount:       2,
		InputDelay:        2,
		Render:            &pb.RenderSettings{Vsync: true, Renderer: 2},
		RollbackDepthHist: []int32{0, 120, 30, 4},
		LateFrames_1_5:    7,
		RemotePeers:       []*pb.RemotePeerReport{{PeerId: 1, RecvLoss: 4, PingMedianMs: 52, Route: "peer:2"}},
		TimesyncFrames:    []int32{300, 1800},
		PresentInterval:   &pb.TimingStats{Count: 5400, MeanUs: 16683, P99Us: 17100, OffPeriod: 3},
		RollbackFrameTime: &pb.TimingStats{Count: 154, MeanUs: 3900, P50Us: 3700, P99Us: 6100, MaxUs: 7400},
		FrameTiming: &pb.FrameTiming{PeriodUs: 16683, OverPeriod: 12, OverPeriodDepthHist: []int32{0, 1, 2, 9},
			Work:         &pb.TimingStats{Count: 5400, P50Us: 6100, P99Us: 15200, MaxUs: 21000},
			RollbackRoom: &pb.TimingStats{Count: 5400, P1Us: -800, P5Us: 4200, P50Us: 9500},
			RenderWait:   &pb.TimingStats{Count: 5380, P50Us: 900, MaxUs: 30500}},
	}
	// Through the wire format, as the handler gets it.
	bin, err := proto.Marshal(report)
	must(t, err)
	var got pb.P2PMatchingReport
	must(t, proto.Unmarshal(bin, &got))

	core, logs := observer.New(zap.InfoLevel)
	zap.New(core).Info("P2PMatchingReport", p2pMatchingReportFields(&got)...)
	f := logs.All()[0].ContextMap()
	assertEq(t, int32(2), f["input_delay"])
	assertEq(t, map[string]interface{}{"threaded_rendering": false, "vsync": true, "renderer": int32(2)}, f["render"])
	assertEq(t, []interface{}{int32(0), int32(120), int32(30), int32(4)}, f["rollback_depth_hist"])
	assertEq(t, int32(7), f["late_frames_1_5"])
	assertEq(t, []interface{}{int32(300), int32(1800)}, f["timesync_frames"])
	assertEq(t, []interface{}{map[string]interface{}{"peer_id": int32(1), "recv_loss": int32(4), "send_loss": int32(0),
		"ping_min_ms": int32(0), "ping_median_ms": int32(52), "ping_max_ms": int32(0), "route": "peer:2"}}, f["remote_peers"])
	present := f["present_interval"].(map[string]interface{})
	assertEq(t, int32(16683), present["mean_us"])
	assertEq(t, int32(17100), present["p99_us"])
	assertEq(t, int32(3), present["off_period"])
	assertEq(t, map[string]interface{}{"count": int32(154), "mean_us": int32(3900), "min_us": int32(0), "p1_us": int32(0),
		"p5_us": int32(0), "p50_us": int32(3700), "p99_us": int32(6100), "max_us": int32(7400), "off_period": int32(0)},
		f["rollback_frame_time"])
	ft := f["frame_timing"].(map[string]interface{})
	assertEq(t, int32(16683), ft["period_us"])
	assertEq(t, int32(12), ft["over_period"])
	assertEq(t, []interface{}{int32(0), int32(1), int32(2), int32(9)}, ft["over_period_depth_hist"])
	assertEq(t, int32(15200), ft["work"].(map[string]interface{})["p99_us"])
	assertEq(t, int32(-800), ft["rollback_room"].(map[string]interface{})["p1_us"])
	assertEq(t, int32(30500), ft["render_wait"].(map[string]interface{})["max_us"])
	// A wait this client did not have (threaded rendering on: no Present wait): not logged.
	_, ok := ft["present_wait"]
	assertEq(t, false, ok)
	// Not sent (an older client): not logged.
	_, ok = f["socket_read_gap"]
	assertEq(t, false, ok)
}
