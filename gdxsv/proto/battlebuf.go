package proto

import (
	"sync"
)

const ringSize = 4096

type BattleBuffer struct {
	mtx   sync.Mutex
	id    string
	ack   uint32           //相手から最後に受信したシーケンス番号
	begin uint32           //まだ相手の応答がない開始のシーケンス番号
	end   uint32           //次に割り振るシーケンス番号
	rbuf  []*BattleMessage //リングバッファ
}

func NewBattleBuffer(id string) *BattleBuffer {
	return &BattleBuffer{
		id:    id,
		ack:   0,
		begin: 1,
		end:   1,
		rbuf:  make([]*BattleMessage, ringSize, ringSize),
	}
}

func (b *BattleBuffer) SetID(id string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.id = id
}

func (b *BattleBuffer) GetID() string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.id
}

func (b *BattleBuffer) PushBattleMessage(msg *BattleMessage) {
	b.mtx.Lock()
	index := b.end
	b.rbuf[index%ringSize] = msg
	b.end++
	b.mtx.Unlock()
}

func (b *BattleBuffer) GetSendData() ([]*BattleMessage, uint32, uint32) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	l := b.begin % ringSize
	e := b.end
	if b.begin+50 < e {
		e = b.begin + 50
	}
	r := e % ringSize
	if l <= r {
		return b.rbuf[l:r], e - 1, b.ack
	} else {
		var tmp []*BattleMessage
		tmp = append(tmp, b.rbuf[l:]...)
		tmp = append(tmp, b.rbuf[:r]...)
		return tmp, e - 1, b.ack
	}
}

func (b *BattleBuffer) ApplySeqAck(seq, ack uint32) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.begin = ack + 1
	b.ack = seq
}

type MessageFilter struct {
	mtx     sync.Mutex
	seq     uint32
	recvSeq map[string]uint32
}

func NewMessageFilter(acceptIDs []string) *MessageFilter {
	mf := &MessageFilter{
		seq:     1,
		recvSeq: map[string]uint32{},
	}
	for _, id := range acceptIDs {
		mf.recvSeq[id] = 0
	}
	return mf
}

func (m *MessageFilter) GenerateMessage(userID string, data []byte) *BattleMessage {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	msg := GetBattleMessage()
	msg.Seq = m.seq
	msg.UserId = userID
	msg.Body = data
	m.seq++
	return msg
}

func (m *MessageFilter) Filter(msg *BattleMessage) bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	ack, ok := m.recvSeq[msg.GetUserId()]
	if !ok {
		return false
	}
	if ack == 0 || msg.GetSeq() == ack+1 {
		m.recvSeq[msg.GetUserId()] = msg.GetSeq()
		return true
	}
	return false
}
