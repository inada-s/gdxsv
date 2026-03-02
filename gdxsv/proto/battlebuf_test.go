package proto

import (
	"bytes"
	"testing"
)

func assertT(t *testing.T, f bool, msg string) {
	t.Helper()
	if !f {
		t.Fatal(msg)
	}
}

func TestMessageSeq(t *testing.T) {
	f := NewMessageFilter([]string{"hoge"})
	msg := f.GenerateMessage("hoge", []byte("hoge"))
	assertT(t, msg.GetSeq() == 1, "expected seq 1")
	msg = f.GenerateMessage("hoge", []byte("hoge"))
	assertT(t, msg.GetSeq() == 2, "expected seq 2")
	msg = f.GenerateMessage("hoge", []byte("hoge"))
	assertT(t, msg.GetSeq() == 3, "expected seq 3")
}

func TestProtoRUDP(t *testing.T) {
	a := NewBattleBuffer("a")
	af := NewMessageFilter([]string{"b"})

	b := NewBattleBuffer("b")
	bf := NewMessageFilter([]string{"a"})

	assertT(t, a.begin == 1, "a.begin should be 1")
	assertT(t, a.end == 1, "a.end should be 1")
	assertT(t, af.seq == 1, "af.seq should be 1")

	a.PushBattleMessage(af.GenerateMessage(a.GetID(), []byte("abc")))

	assertT(t, a.end == 2, "a.end should be 2 after push")
	senddata, seq, ack := a.GetSendData()
	t.Log(senddata, seq, ack)

	assertT(t, b.ack == 0, "b.ack should be 0")
	b.ApplySeqAck(seq, ack)
	assertT(t, bf.Filter(senddata[0]) == true, "bf.Filter should return true")
	recvdata := senddata[0]
	t.Log("recv data", senddata[0])
	assertT(t, string(recvdata.GetBody()) == "abc", "body should be abc")
	assertT(t, recvdata.GetUserId() == "a", "user_id should be a")
	assertT(t, recvdata.GetSeq() == 1, "seq should be 1")
	assertT(t, b.ack == 1, "b.ack should be 1")
	t.Log("A send data before", b.begin, b.end)

	assertT(t, a.end == 2, "a.end should still be 2")
	a.PushBattleMessage(af.GenerateMessage(a.GetID(), []byte("def")))
	assertT(t, a.end == 3, "a.end should be 3")
	a.PushBattleMessage(af.GenerateMessage(a.GetID(), []byte("ghi")))
	assertT(t, a.end == 4, "a.end should be 4")
	senddata, seq, ack = a.GetSendData()
	t.Log("B send data before", b.begin, b.end)

	assertT(t, b.ack == 1, "b.ack should still be 1")
	buf := make([]byte, 0)
	for _, msg := range senddata {
		if bf.Filter(msg) {
			buf = append(buf, msg.GetBody()...)
		}
	}
	assertT(t, bytes.Equal(buf, []byte("defghi")), "buf should be defghi")
	buf = buf[:0]
	b.ApplySeqAck(seq, ack)
	assertT(t, b.ack == 3, "b.ack should be 3")
	t.Log("C send data before", b.begin, b.end)

	assertT(t, a.ack == 0, "a.ack should be 0")
	senddata, _, _ = a.GetSendData()
	for _, msg := range senddata {
		if bf.Filter(msg) {
			buf = append(buf, msg.GetBody()...)
		}
	}
	assertT(t, bytes.Equal(buf, []byte("")), "buf should be empty")
	assertT(t, a.ack == 0, "a.ack should still be 0")

	t.Log("D send data before", b.begin, b.end)

	t.Log("add send data before", b.begin, b.end)
	var data []*BattleMessage

	b.PushBattleMessage(bf.GenerateMessage(b.GetID(), []byte("hoge")))
	senddata, _, _ = b.GetSendData()
	data = append(data, senddata...)
	senddata, _, _ = b.GetSendData()
	data = append(data, senddata...)
	senddata, _, _ = b.GetSendData()
	data = append(data, senddata...)

	b.PushBattleMessage(bf.GenerateMessage(b.GetID(), []byte("piyo")))
	senddata, _, _ = b.GetSendData()
	data = append(data, senddata...)
	senddata, _, _ = b.GetSendData()
	data = append(data, senddata...)
	senddata, seq, ack = b.GetSendData()
	data = append(data, senddata...)
	t.Log("add send data after", b.begin, b.end)

	a.ApplySeqAck(seq, ack)
	buf = buf[:0]
	for _, msg := range data {
		if af.Filter(msg) {
			buf = append(buf, msg.GetBody()...)
		}
	}
	t.Log(buf)
	assertT(t, bytes.Equal(buf, []byte("hogepiyo")), "buf should be hogepiyo")
	assertT(t, a.ack == 2, "a.ack should be 2")
}
