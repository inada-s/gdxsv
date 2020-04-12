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

//go:generate stringer -type=CmdID
type CmdID uint16

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

type LbsMessage struct {
	Direction byte
	Category  byte
	Command   CmdID
	BodySize  uint16
	Seq       uint16
	Status    uint32
	Body      []byte
}

func (m *LbsMessage) String() string {
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

	fmt.Fprintf(b, " %v (ID:0x%X)", m.Command, uint16(m.Command))
	fmt.Fprintf(b, " Seq:%v", m.Seq)
	fmt.Fprintf(b, " Body(%d bytes): %v", len(m.Body), hex.EncodeToString(m.Body))
	return b.String()
}

func (m *LbsMessage) SetErr() *LbsMessage {
	m.Status = StatusError
	text := fmt.Sprintf("<LF=6><BODY><CENTER>ERROR: %s<END>", string(m.Command))
	return m.Writer().WriteString(text).Msg()
}

func (m *LbsMessage) Serialize() []byte {
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

func Deserialize(data []byte) (int, *LbsMessage) {
	if len(data) < HeaderSize {
		return 0, nil
	}

	m := LbsMessage{}
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

func NewServerQuestion(command CmdID) *LbsMessage {
	return &LbsMessage{
		Direction: ServerToClient,
		Category:  CategoryQuestion,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewServerAnswer(request *LbsMessage) *LbsMessage {
	return &LbsMessage{
		Direction: ServerToClient,
		Category:  CategoryAnswer,
		Command:   request.Command,
		Status:    StatusSuccess,
		Seq:       request.Seq,
	}
}

func NewServerNotice(command CmdID) *LbsMessage {
	return &LbsMessage{
		Direction: ServerToClient,
		Category:  CategoryNotice,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewClientQuestion(command CmdID) *LbsMessage {
	return &LbsMessage{
		Direction: ClientToServer,
		Category:  CategoryQuestion,
		Command:   command,
		Status:    StatusSuccess,
		Seq:       nextSeq(),
	}
}

func NewClientAnswer(request *LbsMessage) *LbsMessage {
	return &LbsMessage{
		Direction: ClientToServer,
		Category:  CategoryAnswer,
		Command:   request.Command,
		Status:    StatusSuccess,
		Seq:       request.Seq,
	}
}

func NewClientNotice(command CmdID) *LbsMessage {
	return &LbsMessage{
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

func (msg *LbsMessage) Reader() *MessageBodyReader {
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

// Read length-prefixed byte data
func (m *MessageBodyReader) ReadBytes() []byte {
	if m.r.Len() == 0 {
		return nil
	}
	size := m.Read16()
	buf := make([]byte, size, size)
	m.r.Read(buf)
	return buf
}

// Read length-prefixed string
func (m *MessageBodyReader) ReadString() string {
	return string(bytes.Trim(m.ReadBytes(), "\x00"))
}

func (m *MessageBodyReader) ReadShiftJISString() string {
	size := m.Read16()
	buf := make([]byte, size, size)
	m.r.Read(buf)
	ret, err := ioutil.ReadAll(transform.NewReader(bytes.NewReader(buf), japanese.ShiftJIS.NewDecoder()))
	if err != nil {
		glog.Errorln(err)
	}
	return string(bytes.Trim(ret, "\x00"))
}

type MessageBodyWriter struct {
	msg *LbsMessage
	buf *bytes.Buffer
}

func (msg *LbsMessage) Writer() *MessageBodyWriter {
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

func (m *MessageBodyWriter) WriteBytes(bin []byte) *MessageBodyWriter {
	binary.Write(m.buf, binary.BigEndian, uint16(len(bin)))
	m.buf.Write(bin)
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

func (m *MessageBodyWriter) Msg() *LbsMessage {
	return m.msg
}
