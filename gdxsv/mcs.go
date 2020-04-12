package main

import (
	"gdxsv/gdxsv/proto"
	"sync"
	"time"

	"github.com/golang/glog"
)

// Note: Shareing data between lobby server
// In zdxsv, lobby and battle server processes are separeted, and they communicated using RPC.
// However, after all, only one battle server is used.
// It just got complicated.
// So here, let's simply share global variable.
var lobbySharedData struct {
	sync.Mutex
	battleUsers map[string]McsUser
}

func init() {
	lobbySharedData.battleUsers = map[string]McsUser{}
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

func AddUserWhoIsGoingTobattle(battleCode string, userID string, name string, side uint16, sessionID string) {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	lobbySharedData.battleUsers[sessionID] = McsUser{
		BattleCode: battleCode,
		UserID:     userID,
		Name:       name,
		Side:       side,
		SessionID:  sessionID,
		AddTime:    time.Now(),
	}
}

func GetInBattleUsers() []McsUser {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	ret := []McsUser{}
	for _, u := range lobbySharedData.battleUsers {
		ret = append(ret, u)
	}
	return ret
}

func getBattleUserInfo(sessionID string) (McsUser, bool) {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	u, ok := lobbySharedData.battleUsers[sessionID]
	return u, ok
}

func removeBattleUserInfo(battleCode string) {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	for key, u := range lobbySharedData.battleUsers {
		if u.BattleCode == battleCode {
			delete(lobbySharedData.battleUsers, key)
		}
	}
}

func removeZombieUserInfo() {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	zombie := []string{}
	for key, u := range lobbySharedData.battleUsers {
		if 1.0 <= time.Since(u.AddTime).Hours() {
			zombie = append(zombie, key)
		}
	}
	for _, key := range zombie {
		delete(lobbySharedData.battleUsers, key)
	}
}

type McsPeer interface {
	SetUserID(string)
	SetSessionID(string)
	UserID() string
	SessionID() string
	SetPosition(int)
	Position() int
	SetMcsRoomID(string)
	McsRoomID() string
	AddSendData([]byte)
	AddSendMessage(*proto.BattleMessage)
	Address() string
	Close() error
}

type BaseMcsPeer struct {
	sessionID string
	userID    string
	roomID    string
	position  int
}

func (p *BaseMcsPeer) SetUserID(userID string) {
	p.userID = userID
}

func (p *BaseMcsPeer) SetSessionID(sessionID string) {
	p.sessionID = sessionID
}

func (p *BaseMcsPeer) SessionID() string {
	return p.sessionID
}

func (p *BaseMcsPeer) UserID() string {
	return p.userID
}

func (p *BaseMcsPeer) SetPosition(pos int) {
	p.position = pos
}

func (p *BaseMcsPeer) Position() int {
	return p.position
}

func (p *BaseMcsPeer) SetMcsRoomID(id string) {
	p.roomID = id
}

func (p *BaseMcsPeer) McsRoomID() string {
	return p.roomID
}

type Mcs struct {
	roomsMtx sync.Mutex
	rooms    map[string]*McsRoom
}

func NewMcs() *Mcs {
	l := &Mcs{}
	l.rooms = map[string]*McsRoom{}
	return l
}

func (mcs *Mcs) ListenAndServe(addr string) error {
	glog.Info("ListenAndServeBattle", addr)

	tcpSv := NewTCPServer(mcs)
	return tcpSv.ListenAndServe(addr)
}

func (mcs *Mcs) Quit() {
	// TODO impl
}

func (m *Mcs) Join(p McsPeer, sessionID string) *McsRoom {
	user, ok := getBattleUserInfo(sessionID)
	if !ok {
		return nil
	}

	p.SetUserID(user.UserID)
	p.SetSessionID(sessionID)

	m.roomsMtx.Lock()
	room := m.rooms[user.BattleCode]
	if room == nil {
		room = newMcsRoom(m, user.BattleCode)
		m.rooms[user.BattleCode] = room
	}
	m.roomsMtx.Unlock()
	room.Join(p)
	return room
}

func (m *Mcs) OnMcsRoomClose(room *McsRoom) {
	m.roomsMtx.Lock()
	delete(m.rooms, room.battleCode)
	m.roomsMtx.Unlock()
	removeBattleUserInfo(room.battleCode)
}

func IsFinData(buf []byte) bool {
	if len(buf) == 4 &&
		buf[0] == 4 &&
		buf[1] == 240 &&
		buf[2] == 0 &&
		buf[3] == 0 {
		return true
	}
	return false
}
