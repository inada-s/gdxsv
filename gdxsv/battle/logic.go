package battle

import (
	"sync"
	"time"

	"gdxsv/gdxsv/proto"
)

// Note: Shareing data between lobby server
// In zdxsv, lobby and battle server processes are separeted, and they communicated using RPC.
// However, after all, only one battle server is used.
// It just got complicated.
// So here, let's simply share global variable.
var lobbySharedData struct {
	sync.Mutex
	battleUsers map[string]BattleUserInfo
}

func init() {
	lobbySharedData.battleUsers = map[string]BattleUserInfo{}
	go func() {
		for {
			removeZombieUserInfo()
			time.Sleep(time.Minute)
		}
	}()
}

type BattleUserInfo struct {
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
	lobbySharedData.battleUsers[sessionID] = BattleUserInfo{
		BattleCode: battleCode,
		UserID:     userID,
		Name:       name,
		Side:       side,
		SessionID:  sessionID,
		AddTime:    time.Now(),
	}
}

func GetInBattleUsers() []BattleUserInfo {
	lobbySharedData.Lock()
	defer lobbySharedData.Unlock()
	ret := []BattleUserInfo{}
	for _, u := range lobbySharedData.battleUsers {
		ret = append(ret, u)
	}
	return ret
}

func getBattleUserInfo(sessionID string) (BattleUserInfo, bool) {
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

type BasePeer struct {
	sessionID string
	userID    string
	roomID    string
	position  int
}

func (p *BasePeer) SetUserID(userID string) {
	p.userID = userID
}

func (p *BasePeer) SetSessionID(sessionID string) {
	p.sessionID = sessionID
}

func (p *BasePeer) SessionID() string {
	return p.sessionID
}

func (p *BasePeer) UserID() string {
	return p.userID
}

func (p *BasePeer) SetPosition(pos int) {
	p.position = pos
}

func (p *BasePeer) Position() int {
	return p.position
}

func (p *BasePeer) SetRoomID(id string) {
	p.roomID = id
}

func (p *BasePeer) RoomID() string {
	return p.roomID
}

type Peer interface {
	SetUserID(string)
	SetSessionID(string)
	UserID() string
	SessionID() string
	SetPosition(int)
	Position() int
	SetRoomID(string)
	RoomID() string
	AddSendData([]byte)
	AddSendMessage(*proto.BattleMessage)
	Address() string
	Close() error
}

type Logic struct {
	roomsMtx sync.Mutex
	rooms    map[string]*Room
}

func NewLogic() *Logic {
	l := &Logic{}
	l.rooms = map[string]*Room{}
	return l
}

func (m *Logic) Join(p Peer, sessionID string) *Room {
	user, ok := getBattleUserInfo(sessionID)
	if !ok {
		return nil
	}

	p.SetUserID(user.UserID)
	p.SetSessionID(sessionID)

	m.roomsMtx.Lock()
	room := m.rooms[user.BattleCode]
	if room == nil {
		room = newRoom(m, user.BattleCode)
		m.rooms[user.BattleCode] = room
	}
	m.roomsMtx.Unlock()
	room.Join(p)
	return room
}

func (m *Logic) OnRoomClose(room *Room) {
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
