package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"gdxsv/gdxsv/proto"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
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
	GetCloseReason() string
	SetCloseReason(string)
	Logger() *zap.Logger
}

type BaseMcsPeer struct {
	sessionID   string
	userID      string
	roomID      string
	position    int
	closeReason string
	logger      *zap.Logger
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

func (p *BaseMcsPeer) SetCloseReason(reason string) {
	if p.closeReason == "" {
		p.closeReason = reason
	}
}

func (p *BaseMcsPeer) GetCloseReason() string {
	if p.closeReason != "" {
		return p.closeReason
	}
	return "unknown"
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

func (mcs *Mcs) ListenAndServe(addr string) {
	logger.Info("mcs.ListenAndServe", zap.String("addr", addr))
	tcpSv := NewTCPServer(mcs)
	udpSv := NewUDPServer(mcs)

	go func(addr string) {
		err := tcpSv.ListenAndServe(addr)
		if err != nil {
			logger.Fatal("tcpSv.ListenAndServe", zap.Error(err))
		}
	}(addr)

	go func(addr string) {
		err := udpSv.ListenAndServe(addr)
		if err != nil {
			logger.Fatal("udpSv.ListenAndServe", zap.Error(err))
		}
	}(addr)
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
		UpdatedAt:  time.Now(),
		Users:      []*McsUser{},
		Games:      []*McsGame{},
	}

	var sendStatusBuf bytes.Buffer
	sendMcsStatus := func() error {
		sendStatusBuf.Reset()
		gw := gzip.NewWriter(&sendStatusBuf)
		jw := json.NewEncoder(gw)
		err := jw.Encode(status)
		if err != nil {
			logger.Error("json.Encode", zap.Error(err))
			return err
		}

		err = gw.Close()
		if err != nil {
			logger.Error("GzipWriter.Close", zap.Error(err))
			return err
		}

		buf := NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(sendStatusBuf.Bytes()).Msg().Serialize()
		for sum := 0; sum < len(buf); {
			err = conn.SetWriteDeadline(time.Now().Add(time.Second))
			if err != nil {
				logger.Warn("SetWriteDeadline failed", zap.Error(err))
			}
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
					// not enough data coming
					break
				}

				data = data[n:]
				if msg != nil {
					switch msg.Command {
					case lbsExtSyncSharedData:
						body := msg.Reader().ReadBytes()
						gr, err := gzip.NewReader(bytes.NewReader(body))
						if err != nil {
							logger.Error("gzip.NewReader", zap.Error(err), zap.Binary("body", body))
							continue
						}

						var lbsStatus LbsStatus
						jr := json.NewDecoder(gr)
						err = jr.Decode(&lbsStatus)
						if err != nil {
							logger.Error("jr.Decode", zap.Error(err), zap.Binary("body", body))
							continue
						}

						logger.Info("lbs_status updated", zap.Any("lbs_status", &lbsStatus))

						sharedData.SyncLbsToMcs(&lbsStatus)
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
			status.UpdatedAt = mcs.LastUpdated()
			status.Users = sharedData.GetMcsUsers()
			status.Games = sharedData.GetMcsGames()
			err = sendMcsStatus()
			if err != nil {
				logger.Warn("failed to send mcsStatus", zap.Error(err))
				// Don't return here not to quit running games when lbs restarted.
			}

			sharedData.RemoveStaleData()

			if 15 <= time.Since(status.UpdatedAt).Minutes() && len(status.Users) == 0 {
				logger.Info("mcs exit", zap.String("mcs-metrics", mcsMetrics.String()))
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
	user, ok := sharedData.GetBattleUserInfo(sessionID)
	if !ok {
		return nil
	}

	game, ok := sharedData.GetBattleGameInfo(user.BattleCode)
	if !ok {
		return nil
	}

	p.SetUserID(user.UserID)
	p.SetSessionID(sessionID)

	mcs.mtx.Lock()
	mcs.updated = time.Now()
	room := mcs.rooms[user.BattleCode]
	created := false
	if room == nil {
		room = newMcsRoom(mcs, game)
		mcs.rooms[user.BattleCode] = room
		created = true
	}
	mcs.mtx.Unlock()

	if created {
		logger.Info("new McsRoom", zap.String("battle_code", user.BattleCode), zap.Any("game", game))
	}

	room.Join(p, user)

	sharedData.UpdateMcsUserState(sessionID, McsUserStateJoined)
	sharedData.UpdateMcsGameState(user.BattleCode, McsGameStateOpened)

	return room
}

func (mcs *Mcs) OnUserLeft(room *McsRoom, sessionID string, closeReason string) {
	sharedData.SetMcsUserCloseReason(sessionID, closeReason)
	sharedData.UpdateMcsUserState(sessionID, McsUserStateLeft)
}

func (mcs *Mcs) OnMcsRoomClose(room *McsRoom) {
	mcs.mtx.Lock()
	mcs.updated = time.Now()
	delete(mcs.rooms, room.game.BattleCode)
	mcs.mtx.Unlock()
	logger.Info("mcs room closed", zap.String("mcs-metrics", mcsMetrics.String()))
	sharedData.UpdateMcsGameState(room.game.BattleCode, McsGameStateClosed)
}
