package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strings"
	"sync/atomic"

	"github.com/golang/glog"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const (
	HeaderSize       = 12
	ServerToClient   = 0x18
	ClientToServer   = 0x81
	CategoryQuestion = 0x01
	CategoryAnswer   = 0x02
	CategoryNotice   = 0x10
	CategoryCustom   = 0xFF
	StatusError      = 0xFFFFFFFF
	StatusSuccess    = 0x00FFFFFF
)

func sequenceGenerator() func() uint16 {
	var n int32 = 1
	return func() uint16 {
		return uint16(atomic.AddInt32(&n, 1) % 0xFFFF)
	}
}

var nextSeq func() uint16

func init() {
	nextSeq = sequenceGenerator()
}

type Message struct {
	Direction byte
	Category  byte
	Command   uint16
	BodySize  uint16
	Seq       uint16
	Status    uint32
	Body      []byte
}

func (m *Message) String() string {
	b := new(bytes.Buffer)
	switch m.Direction {
	case ClientToServer:
		b.WriteString("C->S")
	case ServerToClient:
		b.WriteString("C<-S")
	}

	switch m.Category {
	case CategoryQuestion:
		b.WriteString(" [Q]")
	case CategoryAnswer:
		b.WriteString(" [A]")
	case CategoryNotice:
		b.WriteString(" [N]")
	case CategoryCustom:
		b.WriteString(" [C]")
	}

	fmt.Fprintf(b, " ID:0x%X", m.Command)
	fmt.Fprintf(b, " Seq:%v", m.Seq)
	fmt.Fprintf(b, " Body(%d bytes):\n", len(m.Body))
	b.WriteString(hex.Dump(m.Body))
	_ = hex.Dump
	return b.String()
}

func (m *Message) Serialize() []byte {
	w := new(bytes.Buffer)
	m.BodySize = uint16(len(m.Body))
	binary.Write(w, binary.BigEndian, m.Direction)
	binary.Write(w, binary.BigEndian, m.Category)
	binary.Write(w, binary.BigEndian, m.Command)
	binary.Write(w, binary.BigEndian, m.BodySize)
	binary.Write(w, binary.BigEndian, m.Seq)
	binary.Write(w, binary.BigEndian, m.Status)
	binary.Write(w, binary.BigEndian, m.Body)
	return w.Bytes()
}

func Deserialize(data []byte) (int, *Message) {
	if len(data) < HeaderSize {
		return 0, nil
	}

	m := Message{}
	r := bytes.NewReader(data)
	binary.Read(r, binary.BigEndian, &m.Direction)
	binary.Read(r, binary.BigEndian, &m.Category)
	binary.Read(r, binary.BigEndian, &m.Command)
	binary.Read(r, binary.BigEndian, &m.BodySize)
	binary.Read(r, binary.BigEndian, &m.Seq)
	binary.Read(r, binary.BigEndian, &m.Status)

	if len(data) < HeaderSize+int(m.BodySize) {
		return 0, nil
	}
	m.Body = data[HeaderSize : HeaderSize+m.BodySize]

	return int(HeaderSize + m.BodySize), &m
}

func NewServerQuestion(command uint16) *Message {
	return &Message{
		Direction: ServerToClient,
		Category:  CategoryQuestion,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewServerAnswer(request *Message) *Message {
	return &Message{
		Direction: ServerToClient,
		Category:  CategoryAnswer,
		Command:   request.Command,
		Status:    StatusSuccess,
		Seq:       request.Seq,
	}
}

func NewServerNotice(command uint16) *Message {
	return &Message{
		Direction: ServerToClient,
		Category:  CategoryNotice,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewClientQuestion(command uint16) *Message {
	return &Message{
		Direction: ClientToServer,
		Category:  CategoryQuestion,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewClientAnswer(request *Message) *Message {
	return &Message{
		Direction: ClientToServer,
		Category:  CategoryAnswer,
		Command:   request.Command,
		Status:    StatusSuccess,
		Seq:       request.Seq,
	}
}

func NewClientNotice(command uint16) *Message {
	return &Message{
		Direction: ClientToServer,
		Category:  CategoryNotice,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

type MessageBodyReader struct {
	seq uint16
	r   *bytes.Reader
}

func (msg *Message) Reader() *MessageBodyReader {
	return &MessageBodyReader{
		seq: msg.Seq,
		r:   bytes.NewReader(msg.Body),
	}
}

func (m *MessageBodyReader) Remaining() int {
	return m.r.Len()
}

func (m *MessageBodyReader) Read8() byte {
	var ret byte
	binary.Read(m.r, binary.BigEndian, &ret)
	return ret
}

func (m *MessageBodyReader) Read16() uint16 {
	var ret uint16
	binary.Read(m.r, binary.BigEndian, &ret)
	return ret
}

func (m *MessageBodyReader) Read32() uint32 {
	var ret uint32
	binary.Read(m.r, binary.BigEndian, &ret)
	return ret
}

func (m *MessageBodyReader) ReadString() string {
	if m.r.Len() == 0 {
		return ""
	}
	size := m.Read16()
	buf := make([]byte, size, size)
	m.r.Read(buf)
	return string(bytes.Trim(buf, "\x00"))
}

func (m *MessageBodyReader) ReadEncryptedString() string {
	if m.r.Len() == 0 {
		return ""
	}
	size := m.Read16()
	if size <= 2 {
		return ""
	}
	buf := make([]byte, size-2, size-2)
	chksum := m.Read16()
	n, err := m.r.Read(buf)
	if err != nil {
		glog.Errorln("read encstr faild. read error:", err)
		return ""
	}
	if n != int(size-2) {
		glog.Errorln("read encstr faild. mismatch read size")
		return ""
	}
	sum := uint16(0)
	p := byte(m.seq)
	fixval := [...]byte{21, 23, 10, 17, 23, 19, 6, 13}
	masks := [...]byte{0x33, 0x30, 0x3c, 0x34, 0x2d, 0x30, 0x3c, 0x34}
	for j, x := range buf {
		i := byte(j)
		buf[i] = x ^ (fixval[i&7] - (i & 0xf8) - p + ((p-9+i)&masks[i&7])*2)
		sum += uint16(buf[i])
	}
	if sum != chksum {
		glog.Errorln("decrypt faild mismatch checksum")
		return ""
	}

	ret, err := ioutil.ReadAll(transform.NewReader(bytes.NewReader(buf), japanese.ShiftJIS.NewDecoder()))
	if err != nil {
		glog.Errorln(err)
	}
	return string(bytes.Trim(ret, "\x00"))
}

type MessageBodyWriter struct {
	msg *Message
	buf *bytes.Buffer
}

func (msg *Message) Writer() *MessageBodyWriter {
	return &MessageBodyWriter{
		msg: msg,
		buf: new(bytes.Buffer),
	}
}

func (m *MessageBodyWriter) BodyLen() int {
	return len(m.msg.Body)
}

func (m *MessageBodyWriter) Write(v []byte) *MessageBodyWriter {
	m.buf.Write(v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write8(v byte) *MessageBodyWriter {
	binary.Write(m.buf, binary.BigEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write8LE(v byte) *MessageBodyWriter {
	binary.Write(m.buf, binary.LittleEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write16(v uint16) *MessageBodyWriter {
	binary.Write(m.buf, binary.BigEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write16LE(v uint16) *MessageBodyWriter {
	binary.Write(m.buf, binary.LittleEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write32(v uint32) *MessageBodyWriter {
	binary.Write(m.buf, binary.BigEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Write32LE(v uint32) *MessageBodyWriter {
	binary.Write(m.buf, binary.LittleEndian, v)
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) WriteString(v string) *MessageBodyWriter {
	ret, err := ioutil.ReadAll(transform.NewReader(strings.NewReader(v), japanese.ShiftJIS.NewEncoder()))
	if err != nil {
		glog.Errorln(err)
	}
	binary.Write(m.buf, binary.BigEndian, uint16(len(ret)))
	m.buf.WriteString(string(ret))
	m.msg.Body = m.buf.Bytes()
	m.msg.BodySize = uint16(len(m.msg.Body))
	return m
}

func (m *MessageBodyWriter) Msg() *Message {
	return m.msg
}
