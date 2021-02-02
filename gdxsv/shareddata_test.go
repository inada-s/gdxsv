package main

import (
	"reflect"
	"testing"
	"time"
)

func TestSharedData_Sync(t *testing.T) {
	prepareLogger()
	battleCode := "012345"
	sessionID := "SESSION012"
	mcsAddr := "127.0.0.1:1234"
	conf.BattlePublicAddr = mcsAddr

	sd1 := SharedData{
		mcsUsers: map[string]*McsUser{},
		mcsGames: map[string]*McsGame{},
	}

	sd2 := SharedData{
		mcsUsers: map[string]*McsUser{},
		mcsGames: map[string]*McsGame{},
	}

	sd1.ShareMcsGame(&McsGame{
		BattleCode: "012345",
		McsAddr:    mcsAddr,
		GameDisk:   GameDiskDC2,
		Rule:       DefaultRule,
		State:      McsGameStateCreated,
		UpdatedAt:  time.Unix(0, 0),
	})

	sd1.ShareMcsUser(&McsUser{
		BattleCode:  battleCode,
		McsRegion:   "",
		UserID:      "USER01",
		Name:        "NAME01",
		PilotName:   "PILOT01",
		GameParam:   []byte{0, 1, 2, 3},
		Platform:    PlatformConsole,
		GameDisk:    GameDiskDC2,
		BattleCount: 1,
		WinCount:    2,
		LoseCount:   3,
		Team:        1,
		SessionID:   sessionID,
		State:       McsUserStateCreated,
		UpdatedAt:   time.Unix(0, 0),
	})

	sd2.SyncLbsToMcs(&LbsStatus{
		McsUsers: sd1.GetMcsUsers(),
		McsGames: sd1.GetMcsGames(),
	})

	if !reflect.DeepEqual(sd1.GetMcsUsers(), sd2.GetMcsUsers()) {
		t.Error("GetMcsUsers is different")
	}
	if !reflect.DeepEqual(sd1.GetMcsGames(), sd2.GetMcsGames()) {
		t.Error("GetMcsGames is different")
	}

	sd2.UpdateMcsGameState(battleCode, McsGameStateOpened)
	sd2.UpdateMcsUserState(sessionID, McsUserStateJoined)

	sd1.SyncMcsToLbs(&McsStatus{
		Region:     "",
		PublicAddr: mcsAddr,
		Users:      sd2.GetMcsUsers(),
		Games:      sd2.GetMcsGames(),
		UpdatedAt:  time.Unix(1, 0),
	})

	if !reflect.DeepEqual(sd1.GetMcsUsers(), sd2.GetMcsUsers()) {
		t.Error("GetMcsUsers is different")
	}

	if !reflect.DeepEqual(sd1.GetMcsGames(), sd2.GetMcsGames()) {
		t.Error("GetMcsGames is different")
	}

	sd2.SetMcsUserCloseReason(sessionID, "timeout")
	sd2.UpdateMcsUserState(sessionID, McsUserStateLeft)

	if len(sd2.GetMcsUsers()) != 1 {
		t.Error("McsUser should not be removed")
	}

	sd1.SyncMcsToLbs(&McsStatus{
		Region:     "",
		PublicAddr: mcsAddr,
		Users:      sd2.GetMcsUsers(),
		Games:      sd2.GetMcsGames(),
		UpdatedAt:  time.Unix(2, 0),
	})

	sd2.UpdateMcsGameState(battleCode, McsGameStateClosed)

	if len(sd2.GetMcsGames()) != 1 {
		t.Error("McsGame should not be removed")
	}

	sd1.SyncMcsToLbs(&McsStatus{
		Region:     "",
		PublicAddr: mcsAddr,
		Users:      sd2.GetMcsUsers(),
		Games:      sd2.GetMcsGames(),
		UpdatedAt:  time.Unix(3, 0),
	})

	sd1.RemoveStaleData()

	if len(sd1.GetMcsGames()) != 0 {
		t.Error("McsGame should be removed")
	}

	sd2.SyncLbsToMcs(&LbsStatus{
		McsUsers: sd1.GetMcsUsers(),
		McsGames: sd1.GetMcsGames(),
	})

	if len(sd2.GetMcsGames()) != 0 {
		t.Error("McsGame should be removed")
	}

	if len(sd2.GetMcsUsers()) != 0 {
		t.Error("McsUser should be removed")
	}
}
