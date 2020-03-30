package proto

import "sync"

var (
	packetPool        = sync.Pool{New: func() interface{} { return new(Packet) }}
	battleMessagePool = sync.Pool{New: func() interface{} { return new(BattleMessage) }}
)

func GetPacket() *Packet {
	return packetPool.Get().(*Packet)
}

func PutPacket(pkt *Packet) {
	pkt.Reset()
	packetPool.Put(pkt)
}

func GetBattleMessage() *BattleMessage {
	return battleMessagePool.Get().(*BattleMessage)
}

func PutBattleMessage(msg *BattleMessage) {
	msg.Reset()
	battleMessagePool.Put(msg)
}
