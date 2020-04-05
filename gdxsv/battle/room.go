package battle

import (
	"sync"

	"github.com/golang/glog"

	"gdxsv/gdxsv/proto"
)

type Room struct {
	sync.RWMutex

	logic      *Logic
	battleCode string
	peers      []Peer // 追加はappend 削除はnil代入 インデックスがposと一致するように維持
}

func newRoom(logic *Logic, battleCode string) *Room {
	return &Room{logic: logic, battleCode: battleCode}
}

func (r *Room) SendMessage(peer Peer, msg *proto.BattleMessage) {
	k := peer.Position()

	r.RLock()
	for i := 0; i < len(r.peers); i++ {
		if i == k {
			continue
		}

		other := r.peers[i]
		if other != nil {
			if glog.V(2) {
				glog.Infof("[ROOM] %v>%v %v", peer.UserID(), other.UserID(), msg.GetBody())
			}
			other.AddSendMessage(msg)
		}
	}
	r.RUnlock()
}

func (r *Room) Dispose() {
	r.Lock()
	logic := r.logic
	r.logic = nil
	r.peers = nil
	r.Unlock()
	logic.OnRoomClose(r)
}

func (r *Room) Join(p Peer) {
	p.SetRoomID(r.battleCode)
	r.Lock()
	p.SetPosition(len(r.peers))
	r.peers = append(r.peers, p)
	r.Unlock()
}

func (r *Room) Leave(p Peer) {
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
