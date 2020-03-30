package battle

import (
	"sync"

	"github.com/golang/glog"

	"gdxsv/gdxsv/proto"
)

type Room struct {
	sync.RWMutex

	id    int
	peers []Peer // 追加はappend 削除はnil代入 インデックスがposと一致するように維持
}

func newRoom(id int) *Room {
	return &Room{id: id}
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

func (r *Room) Clear() {
	r.Lock()
	for i := 0; i < len(r.peers); i++ {
		r.peers[i] = nil
	}
	r.peers = r.peers[:0]
	r.Unlock()
}

func (r *Room) Join(p Peer) {
	p.SetRoomID(r.id)
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
		r.Clear()
	}

	glog.Infof("leave peer %v", p.Address())
}
