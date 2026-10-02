package main

import (
	"os"
	"time"

	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

// openSpectatorTestSession opens a live spectator session for flycast's local
// rollback test (gdxsv:rbk_test with TEST_SPECTATOR_LBS), which has no lobby to
// open one. The users, rule and patches come from a replay of the same test, so
// spectators boot the match exactly as the peers did. The assembled recording
// is written next to it as <path>.live.pb every second, for comparison with the
// peers' own replays.
func openSpectatorTestSession(path string, sessionID int32) {
	bin, err := os.ReadFile(path)
	if err != nil {
		logger.Fatal("spectator_test_session: read", zap.Error(err))
	}
	var header proto.BattleLogFile
	if err := pb.Unmarshal(bin, &header); err != nil {
		logger.Fatal("spectator_test_session: unmarshal", zap.Error(err))
	}
	matching := &proto.P2PMatching{
		BattleCode: header.GetBattleCode(),
		SessionId:  sessionID,
		RuleBin:    header.GetRuleBin(),
		Users:      header.GetUsers(),
	}
	spectatorRegistry.Open(matching, header.GetGameDisk(), &proto.GamePatchList{Patches: header.GetPatches()})
	logger.Info("spectator test session opened",
		zap.String("battle_code", header.GetBattleCode()), zap.Int32("session_id", sessionID))

	go func() {
		for range time.Tick(time.Second) {
			s, ok := spectatorRegistry.GetAny(header.GetBattleCode())
			if !ok {
				continue
			}
			s.mtx.RLock()
			out, err := pb.Marshal(s.log)
			s.mtx.RUnlock()
			if err == nil {
				_ = os.WriteFile(path+".live.pb", out, 0o644)
			}
		}
	}()
}
