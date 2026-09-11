package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLbsMessage_String(t *testing.T) {
	type fields struct {
		Direction CmdDirection
		Category  CmdCategory
		Command   CmdID
		BodySize  uint16
		Seq       uint16
		Status    CmdStatus
		Body      []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "simple",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryQuestion,
				Command:   lbsChatMessage,
				BodySize:  3,
				Seq:       99,
				Status:    StatusSuccess,
				Body:      []byte{1, 2, 3},
			},
			want: `LbsMessage{Command: lbsChatMessage, Direction: ServerToClient, Category: CategoryQuestion, Seq: 99, Status: StatusSuccess, BodySize: 3, Body: hexbytes("010203")}`,
		},
		{
			name: "no body",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryNotice,
				Command:   lbsLoginOk,
				Seq:       99,
				Status:    StatusSuccess,
			},
			want: `LbsMessage{Command: lbsLoginOk, Direction: ServerToClient, Category: CategoryNotice, Seq: 99, Status: StatusSuccess}`,
		},
		{
			name: "unknown cmd id",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryQuestion,
				Command:   0x0123,
				BodySize:  3,
				Seq:       99,
				Status:    StatusSuccess,
				Body:      []byte{1, 2, 3},
			},
			want: `LbsMessage{Command: CmdID(0x0123), Direction: ServerToClient, Category: CategoryQuestion, Seq: 99, Status: StatusSuccess, BodySize: 3, Body: hexbytes("010203")}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &LbsMessage{
				Direction: tt.fields.Direction,
				Category:  tt.fields.Category,
				Command:   tt.fields.Command,
				BodySize:  tt.fields.BodySize,
				Seq:       tt.fields.Seq,
				Status:    tt.fields.Status,
				Body:      tt.fields.Body,
			}
			if got := m.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assertBytes(tb testing.TB, expected, actual []byte) {
	tb.Helper()
	if !bytes.Equal(expected, actual) {
		tb.Fatalf("assertBytes failed.\n expected: % x\n actual:   % x", expected, actual)
	}
}

// Shift-JIS encodings used across the tests below.
var (
	sjisGundam     = []byte{0x83, 0x4b, 0x83, 0x93, 0x83, 0x5f, 0x83, 0x80} // ガンダム
	sjisGundamHalf = []byte{0xb6, 0xde, 0xdd, 0xc0, 0xde, 0xd1}             // ｶﾞﾝﾀﾞﾑ
)

func TestMessageBodyWriter_RawBytes(t *testing.T) {
	tests := []struct {
		name  string
		write func(w *MessageBodyWriter)
		want  []byte
	}{
		{"Write8", func(w *MessageBodyWriter) { w.Write8(0x12) }, []byte{0x12}},
		{"Write8LE", func(w *MessageBodyWriter) { w.Write8LE(0x12) }, []byte{0x12}},
		{"Write16", func(w *MessageBodyWriter) { w.Write16(0x1234) }, []byte{0x12, 0x34}},
		{"Write16LE", func(w *MessageBodyWriter) { w.Write16LE(0x1234) }, []byte{0x34, 0x12}},
		{"Write32", func(w *MessageBodyWriter) { w.Write32(0x12345678) }, []byte{0x12, 0x34, 0x56, 0x78}},
		{"Write32LE", func(w *MessageBodyWriter) { w.Write32LE(0x12345678) }, []byte{0x78, 0x56, 0x34, 0x12}},
		{"Write", func(w *MessageBodyWriter) { w.Write([]byte{0xAA, 0xBB, 0xCC}) }, []byte{0xAA, 0xBB, 0xCC}},
		{"WriteBytes", func(w *MessageBodyWriter) { w.WriteBytes([]byte{0xAA, 0xBB, 0xCC}) }, []byte{0x00, 0x03, 0xAA, 0xBB, 0xCC}},
		{"WriteBytes empty", func(w *MessageBodyWriter) { w.WriteBytes(nil) }, []byte{0x00, 0x00}},
		{"WriteString ascii", func(w *MessageBodyWriter) { w.WriteString("abc") }, []byte{0x00, 0x03, 'a', 'b', 'c'}},
		{"WriteString empty", func(w *MessageBodyWriter) { w.WriteString("") }, []byte{0x00, 0x00}},
		{"WriteString sjis", func(w *MessageBodyWriter) { w.WriteString("ガンダム") }, append([]byte{0x00, 0x08}, sjisGundam...)},
		{"chained", func(w *MessageBodyWriter) { w.Write8(1).Write16(2).Write32(3).Write16LE(4) },
			[]byte{0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x03, 0x04, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &LbsMessage{}
			w := m.Writer()
			tt.write(w)
			assertBytes(t, tt.want, m.Body)
			assertEq(t, uint16(len(tt.want)), m.BodySize)
			assertEq(t, len(tt.want), w.BodyLen())
			if w.Msg() != m {
				t.Fatal("Msg() must return the message the writer was created from")
			}
		})
	}
}

func TestMessageBodyWriter_ReadRoundTrip(t *testing.T) {
	values16 := []uint16{0, 1, 0x7F, 0x80, 0xFF, 0x100, 0x1234, 0x8000, 0xABCD, 0xFFFF}
	values32 := []uint32{0, 1, 0xFF, 0x100, 0xFFFF, 0x10000, 0x12345678, 0x80000000, 0xDEADBEEF, 0xFFFFFFFF}
	rnd := rand.New(rand.NewSource(412))
	for i := 0; i < 64; i++ {
		values16 = append(values16, uint16(rnd.Uint32()))
		values32 = append(values32, rnd.Uint32())
	}

	t.Run("Write8/Read8", func(t *testing.T) {
		for v := 0; v <= 0xFF; v++ {
			m := &LbsMessage{}
			m.Writer().Write8(byte(v)).Write8LE(byte(v))
			r := m.Reader()
			assertEq(t, byte(v), r.Read8())
			assertEq(t, byte(v), r.Read8())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("Write16/Read16", func(t *testing.T) {
		for _, v := range values16 {
			m := &LbsMessage{}
			m.Writer().Write16(v)
			r := m.Reader()
			assertEq(t, v, r.Read16())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("Write16LE/Read16 is byte-swapped", func(t *testing.T) {
		for _, v := range values16 {
			m := &LbsMessage{}
			m.Writer().Write16LE(v)
			r := m.Reader()
			assertEq(t, v>>8|v<<8, r.Read16())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("Write32/Read32", func(t *testing.T) {
		for _, v := range values32 {
			m := &LbsMessage{}
			m.Writer().Write32(v)
			r := m.Reader()
			assertEq(t, v, r.Read32())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("Write32LE/Read32 is byte-swapped", func(t *testing.T) {
		for _, v := range values32 {
			m := &LbsMessage{}
			m.Writer().Write32LE(v)
			r := m.Reader()
			swapped := v>>24 | (v>>8)&0xFF00 | (v<<8)&0xFF0000 | v<<24
			assertEq(t, swapped, r.Read32())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("WriteBytes/ReadBytes", func(t *testing.T) {
		for _, n := range []int{0, 1, 2, 255, 256, 1000} {
			bin := make([]byte, n)
			rnd.Read(bin)
			m := &LbsMessage{}
			m.Writer().WriteBytes(bin)
			r := m.Reader()
			got := r.ReadBytes()
			assertBytes(t, bin, got)
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("WriteString/ReadString", func(t *testing.T) {
		for _, s := range []string{"", "a", "hello world", "<LF=6><BODY><CENTER>ERROR<END>", "0123456789ABCDEF"} {
			m := &LbsMessage{}
			m.Writer().WriteString(s)
			r := m.Reader()
			assertEq(t, s, r.ReadString())
			assertEq(t, 0, r.Remaining())
		}
	})

	t.Run("mixed sequence", func(t *testing.T) {
		m := &LbsMessage{}
		m.Writer().
			Write8(0xAB).
			Write16(0x1234).
			Write32(0xDEADBEEF).
			WriteBytes([]byte{1, 2, 3}).
			WriteString("TEST").
			Write16LE(0x1234).
			Write32LE(0xDEADBEEF).
			Write8LE(0xCD)
		r := m.Reader()
		assertEq(t, len(m.Body), r.Remaining())
		assertEq(t, byte(0xAB), r.Read8())
		assertEq(t, uint16(0x1234), r.Read16())
		assertEq(t, uint32(0xDEADBEEF), r.Read32())
		assertBytes(t, []byte{1, 2, 3}, r.ReadBytes())
		assertEq(t, "TEST", r.ReadString())
		assertEq(t, uint16(0x3412), r.Read16())
		assertEq(t, uint32(0xEFBEADDE), r.Read32())
		assertEq(t, byte(0xCD), r.Read8())
		assertEq(t, 0, r.Remaining())
	})
}

func TestLbsMessage_Serialize(t *testing.T) {
	m := &LbsMessage{
		Direction: ServerToClient,
		Category:  CategoryNotice,
		Command:   lbsChatMessage,
		Seq:       0x0102,
		Status:    StatusSuccess,
		Body:      []byte{0xAA, 0xBB, 0xCC},
	}
	got := m.Serialize()
	want := []byte{
		0x18,       // Direction
		0x10,       // Category
		0x67, 0x02, // Command (big endian)
		0x00, 0x03, // BodySize
		0x01, 0x02, // Seq
		0x00, 0xFF, 0xFF, 0xFF, // Status
		0xAA, 0xBB, 0xCC, // Body
	}
	assertBytes(t, want, got)
	assertEq(t, uint16(3), m.BodySize)
	assertEq(t, HeaderSize+len(m.Body), len(got))

	t.Run("BodySize is recomputed from Body", func(t *testing.T) {
		m := &LbsMessage{Direction: ClientToServer, Category: CategoryQuestion, Command: lbsLoginOk, BodySize: 99, Body: []byte{1}}
		got := m.Serialize()
		assertEq(t, uint16(1), m.BodySize)
		assertBytes(t, []byte{0x00, 0x01}, got[4:6])
		assertEq(t, HeaderSize+1, len(got))
	})

	t.Run("no body", func(t *testing.T) {
		m := &LbsMessage{Direction: ClientToServer, Category: CategoryQuestion, Command: lbsLoginOk, Status: StatusError}
		got := m.Serialize()
		assertEq(t, HeaderSize, len(got))
		assertBytes(t, []byte{0x81, 0x01, 0x61, 0x18, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}, got)
	})
}

func TestLbsMessage_SerializeDeserializeRoundTrip(t *testing.T) {
	tests := []*LbsMessage{
		NewServerQuestion(lbsLoginOk),
		NewServerNotice(lbsChatMessage).Writer().WriteString("ガンダム").Write16(0x1234).Msg(),
		NewClientQuestion(lbsUserRegist).Writer().WriteBytes(bytes.Repeat([]byte{0xFF}, 300)).Msg(),
		NewClientCustom(lbsExtSyncSharedData).Writer().Write32LE(0xDEADBEEF).Msg(),
		{Direction: ClientToServer, Category: CategoryAnswer, Command: 0xBEEF, Seq: 0xFFFF, Status: StatusError, Body: make([]byte, 0xFFFF)},
	}
	for i, m := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			data := m.Serialize()
			n, got := Deserialize(data)
			if got == nil {
				t.Fatal("Deserialize returned nil")
			}
			assertEq(t, len(data), n)
			assertEq(t, m.Direction, got.Direction)
			assertEq(t, m.Category, got.Category)
			assertEq(t, m.Command, got.Command)
			assertEq(t, m.Seq, got.Seq)
			assertEq(t, m.Status, got.Status)
			assertEq(t, uint16(len(m.Body)), got.BodySize)
			assertBytes(t, m.Body, got.Body)
		})
	}
}

func TestLbsMessage_Deserialize(t *testing.T) {
	full := (&LbsMessage{
		Direction: ServerToClient, Category: CategoryQuestion, Command: lbsLoginOk,
		Seq: 1, Status: StatusSuccess, Body: []byte{1, 2, 3, 4},
	}).Serialize()

	t.Run("truncated header returns (0, nil)", func(t *testing.T) {
		for n := 0; n < HeaderSize; n++ {
			cnt, m := Deserialize(full[:n])
			assertEq(t, 0, cnt)
			if m != nil {
				t.Fatalf("len=%d: expected nil message, got %v", n, m)
			}
		}
	})

	t.Run("body shorter than BodySize returns (0, nil)", func(t *testing.T) {
		for n := HeaderSize; n < len(full); n++ {
			cnt, m := Deserialize(full[:n])
			assertEq(t, 0, cnt)
			if m != nil {
				t.Fatalf("len=%d: expected nil message, got %v", n, m)
			}
		}
	})

	t.Run("header only with BodySize=0", func(t *testing.T) {
		data := (&LbsMessage{Direction: ClientToServer, Category: CategoryNotice, Command: lbsLogout}).Serialize()
		cnt, m := Deserialize(data)
		assertEq(t, HeaderSize, cnt)
		if m == nil {
			t.Fatal("expected message")
		}
		assertEq(t, lbsLogout, m.Command)
		assertEq(t, uint16(0), m.BodySize)
		assertEq(t, 0, len(m.Body))
	})

	t.Run("BodySize larger than any data returns (0, nil)", func(t *testing.T) {
		data := append([]byte{}, full...)
		data[4], data[5] = 0xFF, 0xFF
		cnt, m := Deserialize(data)
		assertEq(t, 0, cnt)
		if m != nil {
			t.Fatal("expected nil message")
		}
	})

	t.Run("two concatenated messages", func(t *testing.T) {
		first := (&LbsMessage{Direction: ClientToServer, Category: CategoryQuestion, Command: lbsLoginOk, Seq: 1, Status: StatusSuccess}).
			Writer().WriteString("first").Msg().Serialize()
		second := (&LbsMessage{Direction: ClientToServer, Category: CategoryNotice, Command: lbsChatMessage, Seq: 2, Status: StatusSuccess}).
			Writer().Write32(0xCAFEBABE).Msg().Serialize()
		data := append(append([]byte{}, first...), second...)

		n1, m1 := Deserialize(data)
		assertEq(t, len(first), n1)
		if m1 == nil {
			t.Fatal("first message is nil")
		}
		assertEq(t, lbsLoginOk, m1.Command)
		assertEq(t, uint16(1), m1.Seq)
		assertEq(t, "first", m1.Reader().ReadString())

		n2, m2 := Deserialize(data[n1:])
		assertEq(t, len(second), n2)
		if m2 == nil {
			t.Fatal("second message is nil")
		}
		assertEq(t, lbsChatMessage, m2.Command)
		assertEq(t, uint16(2), m2.Seq)
		assertEq(t, uint32(0xCAFEBABE), m2.Reader().Read32())
		assertEq(t, len(data), n1+n2)

		// With only part of the second message buffered, the first still parses
		// and the caller is told nothing more is available yet.
		partial := data[:len(first)+3]
		n1b, m1b := Deserialize(partial)
		assertEq(t, len(first), n1b)
		assertBytes(t, m1.Body, m1b.Body)
		n3, m3 := Deserialize(partial[n1b:])
		assertEq(t, 0, n3)
		if m3 != nil {
			t.Fatal("partial second message must not parse")
		}
	})

	t.Run("BodySize near uint16 max does not overflow", func(t *testing.T) {
		// HeaderSize+BodySize must be computed in int; in uint16 it wraps for
		// BodySize >= 65524 and the slice expression panics.
		for size := 0x10000 - HeaderSize - 2; size <= 0xFFFF; size++ {
			body := bytes.Repeat([]byte{0x5A}, size)
			data := (&LbsMessage{Direction: ClientToServer, Category: CategoryQuestion, Command: lbsLoginOk, Body: body}).Serialize()
			cnt, m := Deserialize(data)
			assertEq(t, HeaderSize+size, cnt)
			if m == nil {
				t.Fatalf("size=%d: expected message", size)
			}
			assertEq(t, uint16(size), m.BodySize)
			assertBytes(t, body, m.Body)

			cnt, m = Deserialize(data[:len(data)-1])
			assertEq(t, 0, cnt)
			if m != nil {
				t.Fatalf("size=%d: truncated frame must not parse", size)
			}
		}
	})

	t.Run("Body aliases the input slice", func(t *testing.T) {
		data := append([]byte{}, full...)
		_, m := Deserialize(data)
		data[HeaderSize] = 0x99
		assertEq(t, byte(0x99), m.Body[0])
	})
}

func TestLbsMessage_SerializeOversizedBody(t *testing.T) {
	// BodySize is a uint16, so a body longer than 65535 bytes cannot be
	// represented in the header. Serialize does not guard against this: it
	// writes the truncated (wrapped) size into the header but still appends
	// the whole body, producing a frame whose header and payload disagree.
	const extra = 5
	m := &LbsMessage{
		Direction: ServerToClient, Category: CategoryNotice, Command: lbsChatMessage,
		Seq: 7, Status: StatusSuccess,
		Body: bytes.Repeat([]byte{0xAB}, 0x10000+extra),
	}
	data := m.Serialize()
	assertEq(t, uint16(extra), m.BodySize)
	assertEq(t, HeaderSize+0x10000+extra, len(data))
	assertBytes(t, []byte{0x00, extra}, data[4:6])

	// A receiver therefore only sees the first `extra` bytes as the message and
	// the remaining 65536 bytes as garbage for the next frame.
	n, got := Deserialize(data)
	assertEq(t, HeaderSize+extra, n)
	assertEq(t, uint16(extra), got.BodySize)
	assertEq(t, extra, len(got.Body))

	// WriteLbsMessage behaves the same way.
	var buf bytes.Buffer
	must(t, WriteLbsMessage(&buf, m))
	assertBytes(t, data, buf.Bytes())

	// Exactly 65535 bytes is the maximum representable body.
	m = &LbsMessage{Body: make([]byte, 0xFFFF)}
	data = m.Serialize()
	assertEq(t, uint16(0xFFFF), m.BodySize)
	n, got = Deserialize(data)
	assertEq(t, len(data), n)
	assertEq(t, 0xFFFF, len(got.Body))
}

func TestLbsMessage_WriteReadLbsMessage(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		msgs := []*LbsMessage{
			NewServerQuestion(lbsLoginOk),
			NewServerNotice(lbsChatMessage).Writer().WriteString("ｶﾞﾝﾀﾞﾑ").Write16(0x1234).Msg(),
			NewClientQuestion(lbsUserRegist).Writer().WriteBytes(bytes.Repeat([]byte{0xFF}, 300)).Msg(),
			{Direction: ClientToServer, Category: CategoryAnswer, Command: 0xBEEF, Seq: 0xFFFF, Status: StatusError, Body: make([]byte, 0xFFFF)},
		}
		var buf bytes.Buffer
		for _, m := range msgs {
			must(t, WriteLbsMessage(&buf, m))
		}
		// WriteLbsMessage and Serialize must agree on the wire format.
		var want []byte
		for _, m := range msgs {
			want = append(want, m.Serialize()...)
		}
		assertBytes(t, want, buf.Bytes())

		for _, m := range msgs {
			var got LbsMessage
			must(t, ReadLbsMessage(&buf, &got))
			assertEq(t, m.Direction, got.Direction)
			assertEq(t, m.Category, got.Category)
			assertEq(t, m.Command, got.Command)
			assertEq(t, m.Seq, got.Seq)
			assertEq(t, m.Status, got.Status)
			assertEq(t, uint16(len(m.Body)), got.BodySize)
			assertBytes(t, m.Body, got.Body)
			if len(m.Body) == 0 && got.Body != nil {
				t.Fatalf("expected nil body, got %v", got.Body)
			}
		}
		assertEq(t, 0, buf.Len())
		var extra LbsMessage
		assertEq(t, io.EOF, ReadLbsMessage(&buf, &extra))
	})

	t.Run("truncated header", func(t *testing.T) {
		full := NewServerQuestion(lbsLoginOk).Serialize()
		for n := 1; n < HeaderSize; n++ {
			var m LbsMessage
			err := ReadLbsMessage(bytes.NewReader(full[:n]), &m)
			if err == nil {
				t.Fatalf("len=%d: expected error", n)
			}
		}
	})

	t.Run("truncated body", func(t *testing.T) {
		full := NewServerQuestion(lbsLoginOk).Writer().WriteString("hello").Msg().Serialize()
		for n := HeaderSize; n < len(full); n++ {
			var m LbsMessage
			err := ReadLbsMessage(bytes.NewReader(full[:n]), &m)
			if err == nil {
				t.Fatalf("len=%d: expected error", n)
			}
		}
	})
}

func TestMessageBodyReader_Exhausted(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		r := (&LbsMessage{}).Reader()
		assertEq(t, 0, r.Remaining())
		assertEq(t, byte(0), r.Read8())
		assertEq(t, uint16(0), r.Read16())
		assertEq(t, uint32(0), r.Read32())
		if got := r.ReadBytes(); got != nil {
			t.Fatalf("ReadBytes on empty body: want nil, got %v", got)
		}
		assertEq(t, "", r.ReadString())
		assertEq(t, "", r.ReadShiftJISString())
		assertEq(t, 0, r.Remaining())
	})

	t.Run("reads past the end return zero", func(t *testing.T) {
		m := &LbsMessage{}
		m.Writer().Write8(0xFF).Write16(0xFFFF).Write32(0xFFFFFFFF)
		r := m.Reader()
		assertEq(t, 7, r.Remaining())
		assertEq(t, byte(0xFF), r.Read8())
		assertEq(t, uint16(0xFFFF), r.Read16())
		assertEq(t, uint32(0xFFFFFFFF), r.Read32())
		assertEq(t, 0, r.Remaining())
		// Over-reading is silent: the zero value comes back and Remaining stays 0.
		assertEq(t, byte(0), r.Read8())
		assertEq(t, uint16(0), r.Read16())
		assertEq(t, uint32(0), r.Read32())
		assertEq(t, 0, r.Remaining())
	})

	t.Run("partial value consumes the tail and returns zero", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0xAA, 0xBB, 0xCC}}).Reader()
		assertEq(t, uint32(0), r.Read32())
		assertEq(t, 0, r.Remaining())

		r = (&LbsMessage{Body: []byte{0xAA}}).Reader()
		assertEq(t, uint16(0), r.Read16())
		assertEq(t, 0, r.Remaining())
	})

	t.Run("Remaining tracks consumption", func(t *testing.T) {
		m := &LbsMessage{}
		m.Writer().Write32(1).WriteBytes([]byte{1, 2}).Write8(2)
		r := m.Reader()
		assertEq(t, 9, r.Remaining())
		r.Read32()
		assertEq(t, 5, r.Remaining())
		r.ReadBytes()
		assertEq(t, 1, r.Remaining())
		r.Read8()
		assertEq(t, 0, r.Remaining())
	})
}

func TestMessageBodyReader_LengthPrefixOverrun(t *testing.T) {
	// The length prefix claims more bytes than the body holds. There is no
	// bounds check: the reader drains whatever is left and zero-pads the rest
	// up to the claimed size. These tests pin that behaviour so a future
	// bounds check (clamping to Remaining()) shows up as a deliberate change.
	t.Run("ReadBytes zero-pads to the claimed size", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0x00, 0x05, 0x41}}).Reader()
		got := r.ReadBytes()
		assertBytes(t, []byte{0x41, 0x00, 0x00, 0x00, 0x00}, got)
		assertEq(t, 0, r.Remaining())
	})

	t.Run("ReadBytes with huge claimed size and nothing left", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0xFF, 0xFF}}).Reader()
		got := r.ReadBytes()
		assertEq(t, 0xFFFF, len(got))
		assertBytes(t, make([]byte, 0xFFFF), got)
		assertEq(t, 0, r.Remaining())
	})

	t.Run("ReadBytes with only one prefix byte", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0x07}}).Reader()
		got := r.ReadBytes()
		assertEq(t, 0, len(got))
		assertEq(t, 0, r.Remaining())
	})

	t.Run("ReadString trims the zero padding", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0x00, 0x05, 'A', 'B'}}).Reader()
		assertEq(t, "AB", r.ReadString())
		assertEq(t, 0, r.Remaining())
	})

	t.Run("ReadShiftJISString trims the zero padding", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0x00, 0x09, 0x83, 0x4b, 0x83, 0x93}}).Reader()
		assertEq(t, "ガン", r.ReadShiftJISString())
		assertEq(t, 0, r.Remaining())
	})

	t.Run("subsequent reads after overrun return zero", func(t *testing.T) {
		r := (&LbsMessage{Body: []byte{0x00, 0x10, 0x01, 0x02}}).Reader()
		_ = r.ReadBytes()
		assertEq(t, 0, r.Remaining())
		assertEq(t, uint16(0), r.Read16())
		assertEq(t, "", r.ReadString())
	})
}

func TestShiftJIS_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		text string
		sjis []byte
	}{
		{"ascii", "GUNDAM", []byte("GUNDAM")},
		{"full-width katakana", "ガンダム", sjisGundam},
		{"half-width katakana", "ｶﾞﾝﾀﾞﾑ", sjisGundamHalf},
		{"hiragana", "がんだむ", []byte{0x82, 0xaa, 0x82, 0xf1, 0x82, 0xbe, 0x82, 0xde}},
		{"kanji", "連邦", []byte{0x98, 0x41, 0x96, 0x4d}},
		{"mixed", "Zeon軍ｼﾞｵﾝ", []byte{'Z', 'e', 'o', 'n', 0x8c, 0x52, 0xbc, 0xde, 0xb5, 0xdd}},
		{"full-width tilde U+FF5E", "～", []byte{0x81, 0x60}},
		{"NEC special circled one", "①", []byte{0x87, 0x40}},
		{"backslash", "\\", []byte{0x5c}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &LbsMessage{}
			m.Writer().WriteString(tt.text)
			want := append([]byte{byte(len(tt.sjis) >> 8), byte(len(tt.sjis))}, tt.sjis...)
			assertBytes(t, want, m.Body)

			r := m.Reader()
			assertEq(t, tt.text, r.ReadShiftJISString())
			assertEq(t, 0, r.Remaining())

			// ReadString does not decode: it returns the raw Shift-JIS bytes.
			r = m.Reader()
			assertEq(t, string(tt.sjis), r.ReadString())
		})
	}

	t.Run("NUL-padded fixed-width name", func(t *testing.T) {
		const width = 16
		name := "ガンダム"
		enc := sjisGundam
		padded := name + strings.Repeat("\x00", width-len(enc))

		m := &LbsMessage{}
		m.Writer().WriteString(padded)
		assertEq(t, 2+width, len(m.Body))
		assertBytes(t, []byte{0x00, width}, m.Body[:2])
		assertBytes(t, enc, m.Body[2:2+len(enc)])
		assertBytes(t, make([]byte, width-len(enc)), m.Body[2+len(enc):])

		// Both readers strip the padding.
		assertEq(t, name, m.Reader().ReadShiftJISString())
		assertEq(t, string(enc), m.Reader().ReadString())
		r := m.Reader()
		_ = r.ReadShiftJISString()
		assertEq(t, 0, r.Remaining())
	})

	t.Run("multiple strings in one body", func(t *testing.T) {
		m := &LbsMessage{}
		m.Writer().WriteString("ガンダム").WriteString("").WriteString("ｼﾞｵﾝ").Write16(0x1234)
		r := m.Reader()
		assertEq(t, "ガンダム", r.ReadShiftJISString())
		assertEq(t, "", r.ReadShiftJISString())
		assertEq(t, "ｼﾞｵﾝ", r.ReadShiftJISString())
		assertEq(t, uint16(0x1234), r.Read16())
		assertEq(t, 0, r.Remaining())
	})
}

func TestShiftJIS_UnrepresentableRune(t *testing.T) {
	// The encoder stops at the first rune Shift-JIS cannot represent.
	// WriteString logs the error and writes whatever was encoded up to that
	// point, so the rest of the string (even representable parts) is dropped.
	tests := []struct {
		name string
		text string
		want []byte
	}{
		{"emoji only", "😀", []byte{0x00, 0x00}},
		{"emoji in the middle", "ab😀cd", []byte{0x00, 0x02, 'a', 'b'}},
		{"emoji after japanese", "ガンダム😀ジオン", append([]byte{0x00, 0x08}, sjisGundam...)},
		{"CJK extension B U+20B9F", "𠮟", []byte{0x00, 0x00}},
		{"wave dash U+301C", "〜", []byte{0x00, 0x00}},
		{"invalid utf-8", "\xff\xfe", []byte{0x00, 0x00}},
		{"invalid utf-8 after ascii", "ok\xffng", []byte{0x00, 0x02, 'o', 'k'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &LbsMessage{Command: lbsChatMessage}
			m.Writer().WriteString(tt.text).Write8(0x7F)
			assertBytes(t, append(append([]byte{}, tt.want...), 0x7F), m.Body)
			assertEq(t, uint16(len(tt.want)+1), m.BodySize)

			// The body stays well-formed: the (truncated) string is followed
			// by the next field.
			r := m.Reader()
			assertEq(t, string(tt.want[2:]), r.ReadString())
			assertEq(t, byte(0x7F), r.Read8())
			assertEq(t, 0, r.Remaining())
		})
	}
}

func TestShiftJIS_InvalidByteSequence(t *testing.T) {
	// Malformed Shift-JIS from the client must never panic; invalid bytes are
	// replaced with U+FFFD.
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{"lone lead byte 0x81", []byte{0x00, 0x01, 0x81}, "�"},
		{"lead byte followed by NUL", []byte{0x00, 0x03, 'A', 0x81, 0x00}, "A�"},
		{"lead byte at the end after ascii", []byte{0x00, 0x02, 'A', 0x81}, "A�"},
		{"invalid single byte 0xA0", []byte{0x00, 0x01, 0xA0}, "�"},
		{"lead byte 0xEB (unassigned row)", []byte{0x00, 0x02, 0xEB, 0x41}, "�"},
		{"valid then lone lead", []byte{0x00, 0x05, 0x83, 0x4b, 0x83, 0x93, 0x83}, "ガン�"},
		{"length prefix only", []byte{0x00, 0x00}, ""},
		{"length prefix cut", []byte{0x00}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := (&LbsMessage{Body: tt.body}).Reader()
			var got string
			func() {
				defer func() {
					if e := recover(); e != nil {
						t.Fatalf("ReadShiftJISString panicked: %v", e)
					}
				}()
				got = r.ReadShiftJISString()
			}()
			assertEq(t, tt.want, got)
			if !utf8.ValidString(got) {
				t.Fatalf("decoded string is not valid UTF-8: %q", got)
			}
			assertEq(t, 0, r.Remaining())
		})
	}

	t.Run("never panics on arbitrary bytes", func(t *testing.T) {
		rnd := rand.New(rand.NewSource(412))
		for i := 0; i < 2000; i++ {
			n := rnd.Intn(40)
			body := make([]byte, 2+n)
			body[0], body[1] = byte(n>>8), byte(n)
			rnd.Read(body[2:])
			r := (&LbsMessage{Body: body}).Reader()
			got := r.ReadShiftJISString()
			if !utf8.ValidString(got) {
				t.Fatalf("body % x decoded to invalid UTF-8 %q", body, got)
			}
			assertEq(t, 0, r.Remaining())
		}
	})
}

func TestLbsMessage_SetErr(t *testing.T) {
	m := NewServerAnswer(&LbsMessage{Command: lbsLoginOk, Seq: 42})
	got := m.SetErr()
	if got != m {
		t.Fatal("SetErr must return the receiver")
	}
	assertEq(t, StatusError, m.Status)
	assertEq(t, uint16(42), m.Seq)
	assertEq(t, "<LF=6><BODY><CENTER>ERROR: lbsLoginOk<END>", m.Reader().ReadString())
	assertEq(t, uint16(len(m.Body)), m.BodySize)

	data := m.Serialize()
	_, back := Deserialize(data)
	assertEq(t, StatusError, back.Status)
	assertEq(t, "<LF=6><BODY><CENTER>ERROR: lbsLoginOk<END>", back.Reader().ReadShiftJISString())
}

func TestLbsMessage_Constructors(t *testing.T) {
	req := &LbsMessage{Direction: ClientToServer, Category: CategoryQuestion, Command: lbsUserRegist, Seq: 0x1234, Status: StatusSuccess}

	t.Run("server messages get a fresh non-zero sequence", func(t *testing.T) {
		q1 := NewServerQuestion(lbsLoginOk)
		q2 := NewServerQuestion(lbsLoginOk)
		n := NewServerNotice(lbsChatMessage)
		assertEq(t, ServerToClient, q1.Direction)
		assertEq(t, CategoryQuestion, q1.Category)
		assertEq(t, lbsLoginOk, q1.Command)
		assertEq(t, StatusSuccess, q1.Status)
		assertEq(t, ServerToClient, n.Direction)
		assertEq(t, CategoryNotice, n.Category)
		assertEq(t, lbsChatMessage, n.Command)
		for _, m := range []*LbsMessage{q1, q2, n} {
			if m.Seq == 0 {
				t.Fatalf("expected non-zero seq: %v", m)
			}
			assertEq(t, 0, len(m.Body))
		}
		if q1.Seq == q2.Seq || q2.Seq == n.Seq {
			t.Fatalf("sequence numbers must differ: %d %d %d", q1.Seq, q2.Seq, n.Seq)
		}
	})

	t.Run("server answer echoes command and seq", func(t *testing.T) {
		a := NewServerAnswer(req)
		assertEq(t, ServerToClient, a.Direction)
		assertEq(t, CategoryAnswer, a.Category)
		assertEq(t, req.Command, a.Command)
		assertEq(t, req.Seq, a.Seq)
		assertEq(t, StatusSuccess, a.Status)
	})

	t.Run("client messages", func(t *testing.T) {
		tests := []struct {
			name     string
			m        *LbsMessage
			category CmdCategory
			command  CmdID
		}{
			{"question", NewClientQuestion(lbsLoginOk), CategoryQuestion, lbsLoginOk},
			{"answer", NewClientAnswer(req), CategoryCustom, req.Command},
			{"notice", NewClientNotice(lbsLogout), CategoryNotice, lbsLogout},
			{"custom", NewClientCustom(lbsExtSyncSharedData), CategoryCustom, lbsExtSyncSharedData},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assertEq(t, ClientToServer, tt.m.Direction)
				assertEq(t, tt.category, tt.m.Category)
				assertEq(t, tt.command, tt.m.Command)
				assertEq(t, uint16(0), tt.m.Seq)
				assertEq(t, StatusSuccess, tt.m.Status)
			})
		}
	})
}

func TestLbsMessage_SequenceGenerator(t *testing.T) {
	next := sequenceGenerator()
	// The counter starts at 1 and is incremented before use, so the first
	// value is 2. Values are unique within a cycle and wrap modulo 0xFFFF.
	assertEq(t, uint16(2), next())
	seen := map[uint16]bool{2: true}
	for i := 0; i < 0xFFFF-3; i++ {
		v := next()
		if seen[v] {
			t.Fatalf("seq %d repeated at i=%d", v, i)
		}
		seen[v] = true
	}
	assertEq(t, uint16(0), next()) // 0xFFFF % 0xFFFF
	assertEq(t, uint16(1), next())
	assertEq(t, uint16(2), next())
}
