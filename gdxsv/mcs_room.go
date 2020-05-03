package main

import (
	"go.uber.org/zap"
	"sync"

	"gdxsv/gdxsv/proto"
)

type McsRoom struct {
	sync.RWMutex

	mcs        *Mcs
	battleCode string
	peers      []McsPeer // 追加はappend 削除はnil代入 インデックスがposと一致するように維持
}

func newMcsRoom(mcs *Mcs, battleCode string) *McsRoom {
	return &McsRoom{mcs: mcs, battleCode: battleCode}
}

func (r *McsRoom) PeerCount() int {
	r.RLock()
	n := len(r.peers)
	r.RUnlock()
	return n
}

func (r *McsRoom) SendMessage(peer McsPeer, msg *proto.BattleMessage) {
	k := peer.Position()

	r.RLock()
	for i := 0; i < len(r.peers); i++ {
		if i == k {
			continue
		}

		other := r.peers[i]
		if other != nil {
			other.AddSendMessage(msg)
			logger.Debug("relay",
				zap.String("from_user", peer.UserID()),
				zap.String("to_user", other.UserID()),
				zap.Uint32("seq", msg.GetSeq()),
				zap.Binary("data", msg.GetBody()))
		}
	}
	r.RUnlock()
}

func (r *McsRoom) Dispose() {
	r.Lock()
	mcs := r.mcs
	r.mcs = nil
	r.peers = nil
	r.Unlock()
	mcs.OnMcsRoomClose(r)
}

func (r *McsRoom) Join(p McsPeer) {
	p.SetMcsRoomID(r.battleCode)
	r.Lock()
	p.SetPosition(len(r.peers))
	r.peers = append(r.peers, p)
	r.Unlock()
}

func (r *McsRoom) Leave(p McsPeer) {
	pos := p.Position()

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
	if empty {
		r.Dispose()
	}

	logger.Info("leave peer")
}

