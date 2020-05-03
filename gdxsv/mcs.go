package main

import (
	"encoding/json"
	"gdxsv/gdxsv/proto"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"

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
	Logger() *zap.Logger
}

type BaseMcsPeer struct {
	sessionID string
	userID    string
	roomID    string
	position  int
	logger    *zap.Logger
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

func (p *BaseMcsPeer) Logger() *zap.Logger {
	return p.logger
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
	logger.Info("mcs.ListenAndServe", zap.String("addr", addr))
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
		statusBin, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			zap.Error(err)
			return err
		}
		logger.Info("send status to lbs", zap.ByteString("status", statusBin))
		buf := NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(statusBin).Msg().Serialize()
		for sum := 0; sum < len(buf); {
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			n, err := conn.Write(buf[sum:])
			if err != nil {
				logger.Error("send status to lbs failed", zap.Error(err))
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
				logger.Error("read from lbs failed", zap.Error(err))
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
						lbsStatusBin := msg.Reader().ReadBytes()
						logger.Info("recv lbs status", zap.ByteString("status", lbsStatusBin))
						var lbsStatus LbsStatus
						err = json.Unmarshal(lbsStatusBin, &lbsStatus)
						if err != nil {
							logger.Error("json.Unmarshal", zap.Error(err))
							continue
						}
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
				logger.Info("mcs exit")
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

func (mcs *Mcs) Join(p McsPeer, sessionID string) *McsRoom {
	user, ok := getBattleUserInfo(sessionID)
	if !ok {
		return nil
	}

	p.SetUserID(user.UserID)
	p.SetSessionID(sessionID)

	mcs.mtx.Lock()
	mcs.updated = time.Now()
	room := mcs.rooms[user.BattleCode]
	if room == nil {
		room = newMcsRoom(mcs, user.BattleCode)
		mcs.rooms[user.BattleCode] = room
	}
	mcs.mtx.Unlock()
	room.Join(p)
	return room
}

func (mcs *Mcs) OnMcsRoomClose(room *McsRoom) {
	mcs.mtx.Lock()
	mcs.updated = time.Now()
	delete(mcs.rooms, room.battleCode)
	mcs.mtx.Unlock()
	removeBattleUserInfo(room.battleCode)
}
