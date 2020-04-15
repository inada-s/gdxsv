package main

import (
	"encoding/hex"
	"sync"

	"github.com/golang/glog"

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

func (r *McsRoom) SendMessage(peer McsPeer, msg *proto.BattleMessage) {
	k := peer.Position()

	r.RLock()
	for i := 0; i < len(r.peers); i++ {
		if i == k {
			continue
		}

		other := r.peers[i]
		if other != nil {
			if glog.V(2) {
				glog.Infof("[ROOM] %v>%v %v", peer.UserID(), other.UserID(), hex.EncodeToString(msg.GetBody()))
			}
			other.AddSendMessage(msg)
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

	glog.Infof("leave peer %v", p.Address())
}
