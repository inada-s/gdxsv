package main

import (
	"encoding/hex"
	"fmt"
	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"
	"os"
	"path"
	"sync"
	"time"

	"gdxsv/gdxsv/proto"
)

type McsRoom struct {
	mtx sync.RWMutex

	mcs   *Mcs
	game  *McsGame
	peers []McsPeer

	logMtx    sync.RWMutex
	battleLog *proto.BattleLogFile
}

func newMcsRoom(mcs *Mcs, gameInfo *McsGame) *McsRoom {
	room := &McsRoom{
		mcs:  mcs,
		game: gameInfo,
		battleLog: &proto.BattleLogFile{
			LogFileVersion: 20210702,
			GameDisk:       gameInfo.GameDisk,
			GdxsvVersion:   gdxsvVersion,
			BattleCode:     gameInfo.BattleCode,
			RuleBin:        SerializeRule(&gameInfo.Rule),
			Patches:        gameInfo.PatchList.GetPatches(),
			StartAt:        time.Now().UnixNano(),
			BattleData:     make([]*proto.BattleLogMessage, 0, 65536 * 4), // 5Game * 210sec * 60fps = 63000
		},
	}
	return room
}

func (r *McsRoom) PeerCount() int {
	r.mtx.RLock()
	n := len(r.peers)
	r.mtx.RUnlock()
	return n
}

func (r *McsRoom) IsClosing() bool {
	ret := false
	r.mtx.RLock()
	for i := 0; i < len(r.peers); i++ {
		if r.peers[i] == nil {
			ret = true
		}
	}
	r.mtx.RUnlock()
	return ret
}

func (r *McsRoom) SendMessage(peer McsPeer, msg *proto.BattleMessage) {
	ts := time.Now().UnixNano()
	k := peer.Position()

	r.mtx.RLock()
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
	r.mtx.RUnlock()

	logMsg := &proto.BattleLogMessage{
		UserId:    peer.UserID(),
		Body:      msg.Body,
		Seq:       msg.Seq,
		Timestamp: ts,
	}
	r.logMtx.Lock()
	r.battleLog.BattleData = append(r.battleLog.BattleData, logMsg)
	r.logMtx.Unlock()
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
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.logMtx.Lock()
	defer r.logMtx.Unlock()

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
	mcs.OnMcsRoomClose(r)
}

func (r *McsRoom) Join(p McsPeer, u *McsUser) {
	p.SetMcsRoomID(r.game.BattleCode)

	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.logMtx.Lock()
	defer r.logMtx.Unlock()

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
}

func (r *McsRoom) Leave(p McsPeer) {
	pos := p.Position()
	sessionID := p.SessionID()

	r.mtx.Lock()
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
	r.mtx.Unlock()

	r.mcs.OnUserLeft(r, sessionID, p.GetCloseReason())

	if empty {
		go r.Finalize()
	}
}
