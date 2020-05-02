package main

import (
	"encoding/json"
	"fmt"
	"gdxsv/gdxsv/proto"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
)

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
	mtx     sync.Mutex
	updated time.Time
	rooms   map[string]*McsRoom
	delay   time.Duration
}

func NewMcs(delay time.Duration) *Mcs {
	return &Mcs{
		updated: time.Now(),
		rooms:   map[string]*McsRoom{},
		delay:   delay,
	}
}

func (mcs *Mcs) ListenAndServe(addr string) error {
	glog.Info("mcs.ListenAndServe", addr)
	tcpSv := NewTCPServer(mcs)
	return tcpSv.ListenAndServe(addr)
}

func (mcs *Mcs) DialAndSyncWithLbs(lobbyAddr string, battlePublicAddr string, battleRegion string) error {
	conn, err := net.Dial("tcp4", lobbyAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	status := McsStatus{
		PublicAddr: battlePublicAddr,
		Region:     battleRegion,
		Updated:    time.Now(),
		Users:      []McsUser{},
	}

	sendMcsStatus := func() error {
		glog.Info("Send Status", status)
		statusBin, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		buf := NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(statusBin).Msg().Serialize()
		for sum := 0; sum < len(buf); {
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			n, err := conn.Write(buf[sum:])
			if err != nil {
				return err
			}
			sum += n
		}
		return nil
	}

	err = sendMcsStatus()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		buf := make([]byte, 4096)
		data := make([]byte, 0)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := conn.Read(buf)
			if err != nil {
				glog.Error(err)
				return
			}
			data = append(data, buf[:n]...)

			for len(data) >= HeaderSize {
				n, msg := Deserialize(data)
				if n == 0 {
					// not enough data comming
					break
				}

				data = data[n:]
				if msg != nil {
					switch msg.Command {
					case lbsExtSyncSharedData:
						glog.Info("Recv lbsExtSyncSharedData")
						var lbsStatus LbsStatus
						json.Unmarshal(msg.Reader().ReadBytes(), &lbsStatus)
						SyncSharedDataLbsToMcs(&lbsStatus)
					}
				}
			}
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status.Updated = mcs.LastUpdated()
			status.Users = GetMcsUsers()
			err = sendMcsStatus()
			if err != nil {
				return err
			}

			if 15 <= time.Since(status.Updated).Minutes() && len(status.Users) == 0 {
				fmt.Println("mcs exit")
				return nil
			}
		}
	}
}

func (mcs *Mcs) Quit() {
	// TODO impl
}

func (mcs *Mcs) LastUpdated() time.Time {
	mcs.mtx.Lock()
	t := mcs.updated
	mcs.mtx.Unlock()
	return t
}

func (m *Mcs) Join(p McsPeer, sessionID string) *McsRoom {
	user, ok := getBattleUserInfo(sessionID)
	if !ok {
		return nil
	}

	p.SetUserID(user.UserID)
	p.SetSessionID(sessionID)

	m.mtx.Lock()
	m.updated = time.Now()
	room := m.rooms[user.BattleCode]
	if room == nil {
		room = newMcsRoom(m, user.BattleCode)
		m.rooms[user.BattleCode] = room
	}
	m.mtx.Unlock()
	room.Join(p)
	return room
}

func (m *Mcs) OnMcsRoomClose(room *McsRoom) {
	m.mtx.Lock()
	m.updated = time.Now()
	delete(m.rooms, room.battleCode)
	m.mtx.Unlock()
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
