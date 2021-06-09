package main

import (
	"encoding/hex"
	"go.uber.org/zap"
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

func (p PipeConn) SetDeadline(_ time.Time) error {
	return nil
}

func (p PipeConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (p PipeConn) SetWriteDeadline(_ time.Time) error {
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

type TestLbsClient struct {
	DBUser
	t    *testing.T
	conn *PipeConn
}

func (c *TestLbsClient) MustWriteMessage(message *LbsMessage) {
	err := WriteLbsMessage(c.conn.Writer, message)
	must(c.t, err)
}

func (c *TestLbsClient) MustReadMessage() *LbsMessage {
	msg := new(LbsMessage)
	err := ReadLbsMessage(c.conn.Reader, msg)
	must(c.t, err)
	return msg
}

func AssertMsg(t *testing.T, expected *LbsMessage, actual *LbsMessage) {
	if 0 < expected.Direction {
		if expected.Direction != actual.Direction {
			t.Fatal("direction", expected, actual)
		}
	}
	if 0 < expected.Category {
		if expected.Category != actual.Category {
			t.Fatal("category", expected, actual)
		}
	}
	if 0 < expected.Command {
		if expected.Command != actual.Command {
			t.Fatal("command", expected, actual)
		}
	}
	if 0 < expected.BodySize {
		if expected.BodySize != actual.BodySize {
			t.Fatal("command", expected, actual)
		}
	}
	if 0 < expected.Seq {
		if expected.Seq != actual.Seq {
			t.Fatal("seq", expected, actual)
		}
	}
	if 0 < expected.Status {
		if expected.Status != actual.Status {
			t.Fatal("status", expected, actual)
		}
	}
	if 0 < len(expected.Body) {
		a := hex.EncodeToString(expected.Body)
		b := hex.EncodeToString(actual.Body)
		if a != b {
			t.Fatal("body", expected, actual)
		}
	}
}
func hexbytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func prepareLoggedInUser(t *testing.T, lbs *Lbs, user DBUser) (*TestLbsClient, func()) {
	nw := NewPipeNetwork()
	p := lbs.NewPeer(nw.Server)
	go p.serve()

	cli := &TestLbsClient{t: t, DBUser: user, conn: nw.Client}
	// ignore first message
	msg := cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskConnectionID}, msg)

	p.app.Locked(func(_ *Lbs) {
		p.GameDisk = GameDiskDC2
		p.DBUser = user
		p.app.userPeers[p.UserID] = p
		p.logger = p.logger.With(
			zap.String("user_id", p.UserID),
			zap.String("handle_name", p.Name),
		)
	})

	return cli, func() {
		_ = nw.Close()
	}
}

func forceEnterLobby(t *testing.T, lbs *Lbs, cli *TestLbsClient, lobbyID uint16, team uint16) {
	lbs.Locked(func(*Lbs) {
		p := lbs.FindPeer(cli.UserID)
		if p == nil {
			t.Fatal("user not found", cli.DBUser)
		}

		lobby := lbs.GetLobby(p.Platform, p.GameDisk, lobbyID)
		if lobby == nil {
			t.Fatal("lobby not found")
		}

		p.Team = team
		p.Lobby = lobby
		lobby.Users[p.UserID] = &p.DBUser
	})
}

func TestLobbyChatSameLobby(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	user1, cancel1 := prepareLoggedInUser(t, lbs, DBUser{UserID: "TEST01", Name: "NAME01"})
	defer cancel1()
	forceEnterLobby(t, lbs, user1, 1, TeamRenpo)

	user2, cancel2 := prepareLoggedInUser(t, lbs, DBUser{UserID: "TEST02", Name: "NAME02"})
	defer cancel2()
	forceEnterLobby(t, lbs, user2, 1, TeamRenpo)

	text := "HELLO WORLD"
	user1.MustWriteMessage(NewClientNotice(lbsPostChatMessage).Writer().WriteString(text).Msg())

	msg := user1.MustReadMessage()
	AssertMsg(t, &LbsMessage{
		Category:  CategoryNotice,
		Direction: ServerToClient,
		Command:   lbsChatMessage,
		Body:      hexbytes("000654455354303100064e414d453031000b48454c4c4f20574f524c4400000000"),
	}, msg)

	msg = user2.MustReadMessage()
	AssertMsg(t, &LbsMessage{
		Category:  CategoryNotice,
		Direction: ServerToClient,
		Command:   lbsChatMessage,
		Body:      hexbytes("000654455354303100064e414d453031000b48454c4c4f20574f524c4400000000"),
	}, msg)
}

func TestLbs_RegisterBattleResult(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	user1, cancel1 := prepareLoggedInUser(t, lbs, DBUser{
		UserID: "TEST01",
		Name:   "NAME01",
	})
	defer cancel1()
	forceEnterLobby(t, lbs, user1, 1, TeamRenpo)

	mustInsertBattleRecord(BattleRecord{
		BattleCode: "TestLbs_RegisterBattleResult",
		UserID:     "TEST01",
		UserName:   "NAME01",
		PilotName:  "NAME01",
		LobbyID:    1,
		Players:    4,
		Aggregate:  1,
		Pos:        1,
		Team:       TeamRenpo,
		Created:    time.Now(),
		Updated:    time.Now(),
		System:     0,
	})

	lbs.Locked(func(*Lbs) {
		p := lbs.FindPeer("TEST01")
		if p == nil {
			t.Fatal("peer not found")
		}

		lbs.RegisterBattleResult(p, &BattleResult{
			BattleCode:  "TestLbs_RegisterBattleResult",
			BattleCount: 10,
			WinCount:    9,
			LoseCount:   1,
			KillCount:   30,
		})

		assertEq(t, 10, p.BattleCount)
		assertEq(t, 9, p.WinCount)
		assertEq(t, 1, p.LoseCount)
		assertEq(t, 30, p.KillCount)

		assertEq(t, 10, p.RenpoBattleCount)
		assertEq(t, 9, p.RenpoWinCount)
		assertEq(t, 1, p.RenpoLoseCount)
		assertEq(t, 30, p.RenpoKillCount)

		assertEq(t, 0, p.ZeonBattleCount)
		assertEq(t, 0, p.ZeonWinCount)
		assertEq(t, 0, p.ZeonLoseCount)
		assertEq(t, 0, p.ZeonKillCount)

		assertEq(t, 10, p.DailyBattleCount)
		assertEq(t, 9, p.DailyWinCount)
		assertEq(t, 1, p.DailyLoseCount)
	})
}

func Test100_LoginFlowNewUser(t *testing.T) {
	nw := NewPipeNetwork()
	defer nw.Close()

	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()
	go lbs.NewPeer(nw.Server).serve()

	cli := &TestLbsClient{t: t, conn: nw.Client}
	var msg *LbsMessage

	// TODO: use readable text
	cli.MustWriteMessage(NewClientCustom(lbsPlatformInfo).Writer().WriteBytes(hexbytes("666c79636173743d76302e372e350a6769745f686173683d32393533393037640a6275696c645f646174653d323032312d30352d33305431373a32333a32375a0a6370753d7838362f36340a6f733d57696e646f77730a6469736b3d320a6d61786c61673d380a70617463685f69643d383135323531370a63707569643d3735366536353437343936353665363936633635373436650a617369612d65617374313d33360a617369612d65617374323d36310a617369612d6e6f72746865617374313d320a617369612d6e6f72746865617374323d31330a617369612d6e6f72746865617374333d33370a617369612d736f75746865617374313d36390a6175737472616c69612d736f75746865617374313d3132320a6575726f70652d6e6f727468313d3237380a6575726f70652d77657374313d3233330a6575726f70652d77657374323d3233320a6575726f70652d77657374333d3234300a6575726f70652d77657374343d3233380a6575726f70652d77657374363d3234360a6e6f727468616d65726963612d6e6f72746865617374313d3136360a736f757468616d65726963612d65617374313d3235370a75732d63656e7472616c313d3133320a75732d65617374313d3135360a75732d65617374343d3136310a75732d77657374313d39340a75732d77657374323d3130300a75732d77657374333d3131320a000877fa1a6571fd1d64")).Msg())

	// Connection ID exchange
	{
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskConnectionID,
		}, msg)

		cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(hexbytes("0000000000000000")).Msg())

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsConnectionID,
		}, msg)

		connectionID := msg.Reader().ReadString()
		if connectionID == "" {
			t.Fatal(msg)
		}

		cli.MustWriteMessage(NewClientAnswer(msg))
	}

	// Regulation text requests
	{
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsWarningMessage,
		}, msg)

		cli.MustWriteMessage(NewClientQuestion(lbsRegulationHeader).Writer().WriteBytes(hexbytes("31303030")).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsRegulationHeader,
		}, msg)
		r := msg.Reader()
		if r.ReadString() == "" {
			t.Fatal(msg)
		}
		if r.ReadString() == "" {
			t.Fatal(msg)
		}

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsRegulationText,
		}, msg)
		r = msg.Reader()
		if r.ReadString() == "" {
			t.Fatal(msg)
		}
		if r.ReadString() == "" {
			t.Fatal(msg)
		}

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsRegulationFooter,
		}, msg)
	}

	// LoginType check
	{
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsLoginType,
		}, msg)

		cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write8(2).Msg())
	}

	// UserInfo requests
	{
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsUserInfo1,
		}, msg)

		cli.MustWriteMessage(NewClientNotice(lbsEncodeStart))

		// encoded loginkey
		cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(hexbytes("72fe")).Msg())

		// UserInfo 2~8 are currently skipped

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsUserInfo9,
		}, msg)

		cli.MustWriteMessage(NewClientAnswer(msg))
	}

	// User registration
	{
		// Server sends empty user list
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsUserHandle,
		}, msg)
		if msg.Reader().Read8() != 0 {
			t.Fatal("user list should be empty")
		}

		// あいうえお
		cli.MustWriteMessage(NewClientQuestion(lbsUserRegist).Writer().
			WriteString("******").WriteBytes(hexbytes("82a082a282a482a682a8")).Msg())

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsUserRegist,
		}, msg)

		userID := msg.Reader().ReadString()
		if len(userID) != 6 {
			t.Fatal("invalid user id length")
		}

		cli.MustWriteMessage(NewClientQuestion(lbsUserDecide).Writer().WriteString(userID).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsUserDecide,
		}, msg)
		if msg.Reader().ReadString() != userID {
			t.Fatal("unexpected user id")
		}
	}

	// Game code / Battle Result
	{
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskGameCode,
		}, msg)

		cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write8(3).Write8(1).Msg())

		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskBattleResult,
		}, msg)
		cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write(hexbytes("000e000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")).Msg())
	}

	// login ok
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{
		Command: lbsLoginOk,
	}, msg)

	{
		cli.MustWriteMessage(NewClientQuestion(lbsAskNewsTag))
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskNewsTag,
		}, msg)
		// news_tag

		cli.MustWriteMessage(NewClientQuestion(lbsNewsText).Writer().Write8(0).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsNewsText,
		}, msg)
		// news_text

		cli.MustWriteMessage(NewClientQuestion(lbsAskKDDICharges))
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskKDDICharges,
		}, msg)
		if msg.Reader().Read32() != 0 {
			t.Fatal("charge should be 0")
		}

		gameParam := hexbytes("028000000100030002000700040000000000825300000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
		cli.MustWriteMessage(NewClientQuestion(lbsPostGameParameter).Writer().Write(gameParam).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsPostGameParameter,
		}, msg)

		cli.MustWriteMessage(NewClientQuestion(lbsInvitationTag))
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsInvitationTag,
		}, msg)

		cli.MustWriteMessage(NewClientQuestion(lbsAskPatchData).Writer().Write(hexbytes("0003000431303030")).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsAskPatchData,
		}, msg)

		cli.MustWriteMessage(NewClientQuestion(lbsRankRanking).Writer().Write8(0).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsRankRanking,
		}, msg)
		r := msg.Reader()
		_ = r.Read8()
		_ = r.Read32()
		_ = r.Read32()

		cli.MustWriteMessage(NewClientQuestion(lbsWinLose).Writer().Write8(0).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsWinLose,
		}, msg)
		r = msg.Reader()
		_ = r.Read16()
		_ = r.Read16()
		_ = r.Read16()
		_ = r.Read16()
		_ = r.Read16()
		_ = r.Read32()
		_ = r.Read32()

		cli.MustWriteMessage(NewClientQuestion(lbsDeviceData).Writer().Write(hexbytes("00000000000000000001000000000000")).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsDeviceData,
		}, msg)

		cli.MustWriteMessage(NewClientQuestion(lbsTopRankingTag))
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsTopRankingTag,
		}, msg)
		r = msg.Reader()
		_ = r.Read8()
		_ = r.ReadString()

		cli.MustWriteMessage(NewClientQuestion(lbsServerMoney))
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{
			Command: lbsServerMoney,
			Body:    []byte{0, 0, 0, 0, 0, 0, 0, 0},
		}, msg)
	}
}
