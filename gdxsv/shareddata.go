package main

import (
	"bytes"
	"compress/gzip"
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

	lbsStatusCacheTime time.Time
	lbsStatusCache     []byte
}

func init() {
	sharedData.battleUsers = map[string]McsUser{}
	sharedData.battleGames = map[string]McsGame{}
	go func() {
		for {
			removeOldSharedData()
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
	InBattle    bool      `json:"in_battle,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type McsGame struct {
	BattleCode string    `json:"battle_code,omitempty"`
	GameDisk   int       `json:"game_disk"`
	Rule       Rule      `json:"rule,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type McsStatus struct {
	Region     string    `json:"region,omitempty"`
	PublicAddr string    `json:"public_addr,omitempty"`
	Users      []McsUser `json:"users,omitempty"`
	Games      []McsGame `json:"games,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
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
			u.UpdatedAt = status.UpdatedAt
			sharedData.battleUsers[u.SessionID] = u
		}
	}

	for _, g := range status.Games {
		_, ok := sharedData.battleGames[g.BattleCode]
		if ok {
			g.UpdatedAt = status.UpdatedAt
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

func GetMcsUsers() []McsUser {
	sharedData.Lock()
	defer sharedData.Unlock()

	var ret []McsUser

	for _, u := range sharedData.battleUsers {
		ret = append(ret, u)
	}

	return ret
}

func GetSerializedLbsStatus() []byte {
	sharedData.Lock()
	defer sharedData.Lock()

	if 1 <= time.Since(sharedData.lbsStatusCacheTime).Seconds() {
		st := new(LbsStatus)
		for _, u := range sharedData.battleUsers {
			st.Users = append(st.Users, u)
		}

		for _, g := range sharedData.battleGames {
			st.Games = append(st.Games, g)
		}

		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		jw := json.NewEncoder(zw)

		err := jw.Encode(st)
		if err != nil {
			logger.Error("jw.Encode", zap.Error(err))
			return nil
		}

		err = zw.Close()
		if err != nil {
			logger.Error("zw.Close", zap.Error(err))
			return nil
		}

		if (1 << 16) <= buf.Len() {
			logger.Error("too large data", zap.Int("size", buf.Len()))
			return nil
		}

		sharedData.lbsStatusCache = buf.Bytes()
		sharedData.lbsStatusCacheTime = time.Now()
	}

	return sharedData.lbsStatusCache
}

func NotifyLatestLbsStatus(mcs *LbsPeer) {
	mcs.SendMessage(NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(GetSerializedLbsStatus()).Msg())
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

func removeOldSharedData() {
	sharedData.Lock()
	defer sharedData.Unlock()

	for key, u := range sharedData.battleUsers {
		if 1.0 <= time.Since(u.UpdatedAt).Minutes() {
			delete(sharedData.battleUsers, key)
		}
	}

	for key, g := range sharedData.battleGames {
		if 1.0 <= time.Since(g.UpdatedAt).Minutes() {
			delete(sharedData.battleGames, key)
		}
	}
}
