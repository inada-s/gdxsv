package main

import (
	"encoding/json"
	"go.uber.org/zap"
	"sync"
	"time"
)

// sharing temporary data between lbs and mcs

var sharedData struct {
	sync.Mutex
	battleUsers map[string]McsUser // session_id -> user info
	battleGames map[string]McsGame // battle_code -> game info
}

func init() {
	sharedData.battleUsers = map[string]McsUser{}
	sharedData.battleGames = map[string]McsGame{}
	go func() {
		for {
			removeZombieUserInfo()
			time.Sleep(time.Minute)
		}
	}()
}

type McsUser struct {
	BattleCode  string    `json:"battle_code,omitempty"`
	McsRegion   string    `json:"mcs_region,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	PilotName   string    `json:"pilot_name,omitempty"`
	GameParam   []byte    `json:"game_param,omitempty"`
	BattleCount int       `json:"battle_count,omitempty"`
	WinCount    int       `json:"win_count,omitempty"`
	LoseCount   int       `json:"lose_count,omitempty"`
	Side        uint16    `json:"side,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	AddTime     time.Time `json:"add_time,omitempty"`
	InBattle    bool      `json:"in_battle,omitempty"`
}

type McsGame struct {
	BattleCode string `json:"battle_code,omitempty"`
	GameDisk   int    `json:"game_disk"`
	Rule       Rule   `json:"rule,omitempty"`
}

type McsStatus struct {
	Region     string    `json:"region,omitempty"`
	PublicAddr string    `json:"public_addr,omitempty"`
	Updated    time.Time `json:"updated,omitempty"`
	Users      []McsUser `json:"users,omitempty"`
	Games      []McsGame `json:"games,omitempty"`
}

type LbsStatus struct {
	Users []McsUser `json:"users,omitempty"`
	Games []McsGame `json:"games,omitempty"`
}

func ShareMcsGame(g McsGame) {
	sharedData.Lock()
	defer sharedData.Unlock()
	sharedData.battleGames[g.BattleCode] = g
}

func ShareUserWhoIsGoingToBattle(u McsUser) {
	sharedData.Lock()
	defer sharedData.Unlock()
	sharedData.battleUsers[u.SessionID] = u
}

func SyncSharedDataMcsToLbs(status *McsStatus) {
	sharedData.Lock()
	defer sharedData.Unlock()

	for _, u := range status.Users {
		_, ok := sharedData.battleUsers[u.SessionID]
		if ok {
			sharedData.battleUsers[u.SessionID] = u
		}
	}

	for _, g := range status.Games {
		_, ok := sharedData.battleGames[g.BattleCode]
		if ok {
			sharedData.battleGames[g.BattleCode] = g
		}
	}
}

func SyncSharedDataLbsToMcs(status *LbsStatus) {
	sharedData.Lock()
	defer sharedData.Unlock()

	for _, u := range status.Users {
		_, ok := sharedData.battleUsers[u.SessionID]
		if !ok {
			sharedData.battleUsers[u.SessionID] = u
		}
	}

	for _, g := range status.Games {
		_, ok := sharedData.battleGames[g.BattleCode]
		if !ok {
			sharedData.battleGames[g.BattleCode] = g
		}
	}
}

func GetLbsStatus() *LbsStatus {
	sharedData.Lock()
	defer sharedData.Unlock()

	ret := new(LbsStatus)
	for _, u := range sharedData.battleUsers {
		ret.Users = append(ret.Users, u)
	}

	for _, g := range sharedData.battleGames {
		ret.Games = append(ret.Games, g)
	}

	return ret
}

func NotifyLatestLbsStatus(mcs *LbsPeer) {
	lbsStatusBin, err := json.Marshal(GetLbsStatus())
	if err != nil {
		logger.Error("json.Marshal", zap.Error(err))
		return
	}
	mcs.SendMessage(NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(lbsStatusBin).Msg())
}

func getBattleGameInfo(battleCode string) (McsGame, bool) {
	sharedData.Lock()
	defer sharedData.Unlock()
	g, ok := sharedData.battleGames[battleCode]
	return g, ok
}

func getBattleUserInfo(sessionID string) (McsUser, bool) {
	sharedData.Lock()
	defer sharedData.Unlock()
	u, ok := sharedData.battleUsers[sessionID]
	return u, ok
}

func removeBattleGameInfo(battleCode string) {
	sharedData.Lock()
	defer sharedData.Unlock()
	delete(sharedData.battleGames, battleCode)
}

func removeBattleUserInfo(battleCode string) {
	sharedData.Lock()
	defer sharedData.Unlock()
	for key, u := range sharedData.battleUsers {
		if u.BattleCode == battleCode {
			delete(sharedData.battleUsers, key)
		}
	}
}

func removeZombieUserInfo() {
	sharedData.Lock()
	defer sharedData.Unlock()
	zombie := []string{}
	for key, u := range sharedData.battleUsers {
		if 1.0 <= time.Since(u.AddTime).Hours() {
			zombie = append(zombie, key)
		}
	}
	for _, key := range zombie {
		delete(sharedData.battleUsers, key)
	}
}
