package battle

import (
	"fmt"
	"strconv"
	"sync"

	"gdxsv/gdxsv/proto"
)

type BasePeer struct {
	sessionID string
	userID    string
	position  int
	roomID    int
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

func (p *BasePeer) SetRoomID(id int) {
	p.roomID = id
}

func (p *BasePeer) RoomID() int {
	return p.roomID
}

type Peer interface {
	SetUserID(string)
	SetSessionID(string)
	UserID() string
	SessionID() string
	SetPosition(int)
	Position() int
	SetRoomID(int)
	RoomID() int
	AddSendData([]byte)
	AddSendMessage(*proto.BattleMessage)
	Address() string
	Close() error
}

type Logic struct {
	roomsMtx sync.Mutex
	rooms    []*Room
}

func NewLogic() *Logic {
	l := &Logic{}
	l.rooms = make([]*Room, 128)
	return l
}

func (m *Logic) Join(p Peer, sessionID string) *Room {
	/*
		user, ok := m.FindWaitingUser(sessionID)
		if !ok {
			return nil
		}

		p.SetUserID(user.UserID)
		p.SetSessionID(sessionID)
		info := m.GetBattleInfo(sessionID)
		m.ClearBattleInfo(sessionID)

		m.roomsMtx.Lock()
		room := m.rooms[info.RoomID]
		if room == nil {
			room = newRoom(info.RoomID)
			m.rooms[info.RoomID] = room
		}
		m.roomsMtx.Unlock()

		room.Join(p)
		return room
	*/
	return nil
}

func ParseSessionID(value string) (string, error) {
	if len(value) != 10 {
		return "", fmt.Errorf("Invalid value length")
	}
	var1, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "", err
	}
	var2 := fmt.Sprintf("%010d", var1-100001)
	return var2[1:5] + var2[6:], nil
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
