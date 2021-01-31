package main

import (
	"encoding/hex"
	"fmt"
	pb "github.com/golang/protobuf/proto"
	"go.uber.org/zap"
	"os"
	"path"
	"sync"
	"time"

	"gdxsv/gdxsv/proto"
)

type McsRoom struct {
	sync.RWMutex

	mcs       *Mcs
	game      *McsGame
	peers     []McsPeer
	battleLog *proto.BattleLogFile
}

func newMcsRoom(mcs *Mcs, gameInfo *McsGame) *McsRoom {
	return &McsRoom{
		mcs:  mcs,
		game: gameInfo,
		battleLog: &proto.BattleLogFile{
			LogFileVersion: 20201212,
			GameDisk:       gameInfo.GameDisk,
			GdxsvVersion:   gdxsvVersion,
			BattleCode:     gameInfo.BattleCode,
			RuleBin:        SerializeRule(&gameInfo.Rule),
			StartAt:        time.Now().UnixNano(),
		},
	}
}

func (r *McsRoom) PeerCount() int {
	r.RLock()
	n := len(r.peers)
	r.RUnlock()
	return n
}

func (r *McsRoom) IsClosing() bool {
	ret := false
	r.RLock()
	for i := 0; i < len(r.peers); i++ {
		if r.peers[i] == nil {
			ret = true
		}
	}
	r.RUnlock()
	return ret
}

func (r *McsRoom) SendMessage(peer McsPeer, msg *proto.BattleMessage) {
	logMsg := &proto.BattleLogMessage{
		UserId:    peer.UserID(),
		Body:      msg.Body,
		Seq:       msg.Seq,
		Timestamp: time.Now().UnixNano(),
	}

	k := peer.Position()
	r.RLock()
	for i := 0; i < len(r.peers); i++ {
		if i == k {
			continue
		}

		other := r.peers[i]
		if other != nil {
			other.AddSendMessage(msg)

			if ce := logger.Check(zap.DebugLevel, ""); ce != nil {
				logger.Debug("relay",
					zap.String("from_user", peer.UserID()),
					zap.String("to_user", other.UserID()),
					zap.Uint32("seq", msg.GetSeq()),
					zap.String("data", hex.EncodeToString(msg.GetBody())))
			}
		}
	}
	r.RUnlock()

	r.Lock()
	r.battleLog.BattleData = append(r.battleLog.BattleData, logMsg)
	r.Unlock()
}

func (r *McsRoom) saveBattleLogLocked(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	bytes, err := pb.Marshal(r.battleLog)
	if err != nil {
		return err
	}

	for p := 0; p < len(bytes); {
		n, err := f.Write(bytes[p:])
		if err != nil {
			return err
		}
		p += n
	}

	return nil
}

func (r *McsRoom) Finalize() {
	r.Lock()
	r.battleLog.EndAt = time.Now().UnixNano()

	fileName := fmt.Sprintf("disk%v-%v.pb", r.battleLog.GameDisk, r.battleLog.BattleCode)
	err := r.saveBattleLogLocked(path.Join(conf.BattleLogPath, fileName))
	if err != nil {
		logger.Error("Failed to save battle log", zap.Error(err))
	}
	mcs := r.mcs
	r.mcs = nil
	r.peers = nil
	r.battleLog = nil
	r.Unlock()
	mcs.OnMcsRoomClose(r)
}

func (r *McsRoom) Join(p McsPeer, u *McsUser) {
	p.SetMcsRoomID(r.game.BattleCode)
	r.Lock()
	r.battleLog.Users = append(r.battleLog.Users, &proto.BattleLogUser{
		UserId:      u.UserID,
		UserName:    u.Name,
		PilotName:   u.PilotName,
		GameParam:   u.GameParam,
		BattleCount: int32(u.BattleCount),
		WinCount:    int32(u.WinCount),
		LoseCount:   int32(u.LoseCount),
	})
	p.SetPosition(len(r.peers))
	r.peers = append(r.peers, p)
	r.Unlock()
}

func (r *McsRoom) Leave(p McsPeer) {
	pos := p.Position()
	sessionID := p.SessionID()

	r.Lock()
	if pos < len(r.peers) {
		r.peers[pos] = nil
	}
	empty := true
	for i := 0; i < len(r.peers); i++ {
		if r.peers[i] != nil {
			empty = false
			break
		}
	}
	r.Unlock()

	r.mcs.OnUserLeft(r, sessionID, p.GetCloseReason())

	if empty {
		r.Finalize()
	}
}
