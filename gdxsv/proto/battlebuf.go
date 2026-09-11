package proto

import (
	"sync"
)

const ringSize = 1024

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
		rbuf:  make([]*BattleMessage, ringSize),
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

// ApplySeqAck applies a cumulative ack received from the peer.
//
// seq/ack come straight from an unauthenticated UDP packet, so they are
// validated before touching the send window: only acks that refer to a
// message we have actually sent (ack < end) and that move the window
// forward are honoured. A bogus ack (ack >= end, or ack+1 wrapping to 0)
// used to set begin past end, making GetSendData hand out nil entries or
// up to ringSize-50 stale slots instead of the intended 50-message window.
func (b *BattleBuffer) ApplySeqAck(seq, ack uint32) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if ack < b.end && b.begin < ack+1 {
		b.begin = ack + 1
	}
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
	mf.SetAcceptIDs(acceptIDs)
	return mf
}

// SetAcceptIDs replaces the set of user IDs whose messages pass Filter.
// IDs that are not in acceptIDs are dropped, so a peer created with a
// placeholder ID (McsUDPPeer starts with "") stops accepting messages for
// that placeholder once the real user ID is set. IDs that are already
// accepted keep their receive state.
func (m *MessageFilter) SetAcceptIDs(acceptIDs []string) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	recvSeq := make(map[string]uint32, len(acceptIDs))
	for _, id := range acceptIDs {
		recvSeq[id] = m.recvSeq[id]
	}
	m.recvSeq = recvSeq
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
	// Seq 0 is never generated (GenerateMessage starts at 1). Accepting it
	// would store 0 as the last received seq, which is the "nothing received
	// yet" sentinel, leaving the filter accepting anything forever.
	if msg.GetSeq() == 0 {
		return false
	}
	if ack == 0 || msg.GetSeq() == ack+1 {
		m.recvSeq[msg.GetUserId()] = msg.GetSeq()
		return true
	}
	return false
}
