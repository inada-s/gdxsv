package main

import (
	"encoding/hex"
	"io"
	"net"
	"testing"
	"time"
)

type MockAddr struct {
	NetworkString string
	AddrString    string
}

func (a MockAddr) Network() string {
	return a.NetworkString
}

func (a MockAddr) String() string {
	return a.AddrString
}

type PipeConn struct {
	Reader *io.PipeReader
	Writer *io.PipeWriter
}

func (p PipeConn) Close() error {
	if err := p.Writer.Close(); err != nil {
		return err
	}
	if err := p.Reader.Close(); err != nil {
		return err
	}
	return nil
}

func (p PipeConn) Read(data []byte) (n int, err error) {
	return p.Reader.Read(data)
}

func (p PipeConn) Write(data []byte) (n int, err error) {
	return p.Writer.Write(data)
}

func (p PipeConn) LocalAddr() net.Addr {
	return MockAddr{
		NetworkString: "tcp",
		AddrString:    "127.0.0.1",
	}
}

func (p PipeConn) RemoteAddr() net.Addr {
	return MockAddr{
		NetworkString: "tcp",
		AddrString:    "127.0.0.1",
	}
}

func (p PipeConn) SetDeadline(t time.Time) error {
	return nil
}

func (p PipeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (p PipeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type PipeNetwork struct {
	Server *PipeConn
	Client *PipeConn
}

func NewPipeNetwork() *PipeNetwork {
	svRead, clWrite := io.Pipe()
	clRead, svWrite := io.Pipe()

	return &PipeNetwork{
		Server: &PipeConn{
			Reader: svRead,
			Writer: svWrite,
		},
		Client: &PipeConn{
			Reader: clRead,
			Writer: clWrite,
		},
	}
}

func (c *PipeNetwork) Close() error {
	if err := c.Server.Close(); err != nil {
		return err
	}
	if err := c.Client.Close(); err != nil {
		return err
	}
	return nil
}

func hexbytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func Test100_LoginFlowNewUser(t *testing.T) {
	nw := NewPipeNetwork()
	defer nw.Close()

	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()
	go lbs.NewPeer(nw.Server).serve()

	msg := new(LbsMessage)

	// TODO: use readable text
	must(t, WriteLbsMessage(nw.Client.Writer, NewClientCustom(lbsPlatformInfo).Writer().WriteBytes(hexbytes("666c79636173743d76302e372e350a6769745f686173683d32393533393037640a6275696c645f646174653d323032312d30352d33305431373a32333a32375a0a6370753d7838362f36340a6f733d57696e646f77730a6469736b3d320a6d61786c61673d380a70617463685f69643d383135323531370a63707569643d3735366536353437343936353665363936633635373436650a617369612d65617374313d33360a617369612d65617374323d36310a617369612d6e6f72746865617374313d320a617369612d6e6f72746865617374323d31330a617369612d6e6f72746865617374333d33370a617369612d736f75746865617374313d36390a6175737472616c69612d736f75746865617374313d3132320a6575726f70652d6e6f727468313d3237380a6575726f70652d77657374313d3233330a6575726f70652d77657374323d3233320a6575726f70652d77657374333d3234300a6575726f70652d77657374343d3233380a6575726f70652d77657374363d3234360a6e6f727468616d65726963612d6e6f72746865617374313d3136360a736f757468616d65726963612d65617374313d3235370a75732d63656e7472616c313d3133320a75732d65617374313d3135360a75732d65617374343d3136310a75732d77657374313d39340a75732d77657374323d3130300a75732d77657374333d3131320a000877fa1a6571fd1d64")).Msg()))

	// Connection ID exchange
	{
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsAskConnectionID {
			t.Fatal(msg)
		}

		must(t, WriteLbsMessage(nw.Client.Writer, NewClientAnswer(msg).Writer().WriteBytes(hexbytes("0000000000000000")).Msg()))

		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsConnectionID {
			t.Fatal(msg)
		}
		connectionID := msg.Reader().ReadString()
		if connectionID == "" {
			t.Fatal(msg)
		}

		must(t, WriteLbsMessage(nw.Client.Writer, NewClientAnswer(msg)))
	}

	// Regulation text requests
	{
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsWarningMessage {
			t.Fatal(msg)
		}

		must(t, WriteLbsMessage(nw.Client.Writer, NewClientQuestion(lbsRegulationHeader).Writer().WriteBytes(hexbytes("31303030")).Msg()))
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsRegulationHeader {
			t.Fatal(msg)
		}

		r := msg.Reader()
		if r.ReadString() == "" {
			t.Fatal(msg)
		}
		if r.ReadString() == "" {
			t.Fatal(msg)
		}

		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsRegulationText {
			t.Fatal(msg)
		}
		r = msg.Reader()
		if r.ReadString() == "" {
			t.Fatal(msg)
		}
		if r.ReadString() == "" {
			t.Fatal(msg)
		}

		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsRegulationFooter {
			t.Fatal(msg)
		}
	}

	// LoginType check
	{
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsLoginType {
			t.Fatal(msg)
		}
		must(t, WriteLbsMessage(nw.Client.Writer, NewClientAnswer(msg).Writer().Write8(2).Msg()))
	}

	// UserInfo requests
	{
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsUserInfo1 {
			t.Fatal(msg)
		}

		must(t, WriteLbsMessage(nw.Client.Writer, NewClientNotice(lbsEncodeStart)))

		// encoded loginkey
		must(t, WriteLbsMessage(nw.Client.Writer, NewClientAnswer(msg).Writer().WriteBytes(hexbytes("72fe")).Msg()))

		// UserInfo 2~8 are currently skipped

		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsUserInfo9 {
			t.Fatal(msg)
		}
		must(t, WriteLbsMessage(nw.Client.Writer, NewClientAnswer(msg)))
	}

	// User registration
	{
		// Server sends empty user list
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsUserHandle {
			t.Fatal(msg)
		}
		if msg.Reader().Read8() != 0 {
			t.Fatal("user list should be empty")
		}

		// あいうえお
		must(t, WriteLbsMessage(nw.Client.Writer, NewClientQuestion(lbsUserRegist).Writer().
			WriteString("******").WriteBytes(hexbytes("82a082a282a482a682a8")).Msg()))

		t.Log("A")
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsUserRegist {
			t.Fatal(msg)
		}
		t.Log("B")
		userID := msg.Reader().ReadString()
		if len(userID) != 6 {
			t.Fatal("invalid user id length")
		}

		must(t, WriteLbsMessage(nw.Client.Writer, NewClientQuestion(lbsUserDecide).Writer().WriteString(userID).Msg()))
		must(t, ReadLbsMessage(nw.Client.Reader, msg))
		if msg.Command != lbsUserDecide {
			t.Fatal(msg)
		}
		if msg.Reader().ReadString() != userID {
			t.Fatal("unexpected user id")
		}
	}

	// Game code
	{

	}
}
