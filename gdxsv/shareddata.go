package main

import (
	"sync"
	"time"
)

// sharing temporary data between lbs and mcs

var sharedData struct {
	sync.Mutex
	battleUsers map[string]McsUser
}

func init() {
	sharedData.battleUsers = map[string]McsUser{}
	go func() {
		for {
			removeZombieUserInfo()
			time.Sleep(time.Minute)
		}
	}()
}

type McsUser struct {
	BattleCode string    `json:"battle_code,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	Name       string    `json:"name,omitempty"`
	Side       uint16    `json:"side,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	AddTime    time.Time `json:"add_time,omitempty"`
	InBattle   bool      `json:"in_battle,omitempty"`
}

type McsStatus struct {
	Region     string    `json:"region,omitempty"`
	PublicAddr string    `json:"public_addr,omitempty"`
	Updated    time.Time `json:"updated,omitempty"`
	Users      []McsUser `json:"users,omitempty"`
}

func AddUserWhoIsGoingTobattle(battleCode string, userID string, name string, side uint16, sessionID string) {
	sharedData.Lock()
	defer sharedData.Unlock()
	sharedData.battleUsers[sessionID] = McsUser{
		BattleCode: battleCode,
		UserID:     userID,
		Name:       name,
		Side:       side,
		SessionID:  sessionID,
		AddTime:    time.Now(),
	}
}

func GetInBattleUsers() []McsUser {
	sharedData.Lock()
	defer sharedData.Unlock()
	ret := []McsUser{}
	for _, u := range sharedData.battleUsers {
		ret = append(ret, u)
	}
	return ret
}

func getBattleUserInfo(sessionID string) (McsUser, bool) {
	sharedData.Lock()
	defer sharedData.Unlock()
	u, ok := sharedData.battleUsers[sessionID]
	return u, ok
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
