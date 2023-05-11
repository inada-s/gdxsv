package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"testing"
	"time"

	"go.uber.org/zap"
)

const samplePlatformInfo = `\
asia-east1=36
asia-east2=61
asia-northeast1=2
asia-northeast2=13
asia-northeast3=37
asia-southeast1=69
australia-southeast1=122
europe-north1=278
europe-west1=233
europe-west2=232
europe-west3=240
europe-west4=238
europe-west6=246
northamerica-northeast1=166
southamerica-east1=257
us-central1=132
us-east1=156
us-east4=161
us-west1=94
us-west2=100
us-west3=112
flycast=v1.0.5
build_date=2021-05-30T17:23:27Z
git_hash=2953907d
cpu=x86/64
cpuid=aaaaaaaaaaaaaaaaaaa
os=Windows
patch_id=8152517
disk=2
maxlag=8`

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

var errTimeout = fmt.Errorf("timeout")

func writeMessageWithTimeout(writer io.Writer, message *LbsMessage, duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var err error
	go func() {
		defer cancel()
		err = WriteLbsMessage(writer, message)
	}()

	<-ctx.Done()

	if err != nil {
		return err
	}

	if ctx.Err() == nil {
		return nil
	}

	if ctx.Err() == context.Canceled {
		return nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return errTimeout
	}

	return ctx.Err()
}

func readMessageWithTimeout(reader io.Reader, message *LbsMessage, duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var err error
	go func() {
		defer cancel()
		err = ReadLbsMessage(reader, message)
	}()

	<-ctx.Done()

	if err != nil {
		return err
	}

	if ctx.Err() == nil {
		return nil
	}

	if ctx.Err() == context.Canceled {
		return nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return errTimeout
	}

	return ctx.Err()
}

func (c *TestLbsClient) MustWriteMessage(message *LbsMessage) {
	must(c.t, writeMessageWithTimeout(c.conn, message, 5*time.Second))
}

func (c *TestLbsClient) MustReadMessage() *LbsMessage {
	msg := new(LbsMessage)
	must(c.t, readMessageWithTimeout(c.conn, msg, 5*time.Second))
	return msg
}

func (c *TestLbsClient) MustReadMessageSkipNotice() *LbsMessage {
	msg := new(LbsMessage)
	deadline := time.Now().Add(5 * time.Second)

	for 0 < time.Until(deadline) {
		must(c.t, readMessageWithTimeout(c.conn, msg, 5*time.Second))
		if msg.Category != CategoryNotice {
			return msg
		}
	}

	c.t.Fatal("MustReadMessageSkipNotice timed out:", string(debug.Stack()))
	return nil
}

func (c *TestLbsClient) MustReadMessageSkipNoticeUntil(cmd CmdID) *LbsMessage {
	msg := new(LbsMessage)
	deadline := time.Now().Add(5 * time.Second)

	for 0 < time.Until(deadline) {
		must(c.t, readMessageWithTimeout(c.conn, msg, 5*time.Second))
		if msg.Command == cmd {
			return msg
		}
	}

	c.t.Fatal("MustReadMessageSkipNoticeUntil timed out:", string(debug.Stack()))
	return nil
}

func AssertMsg(t *testing.T, expected *LbsMessage, actual *LbsMessage) {
	if 0 < expected.Direction {
		if expected.Direction != actual.Direction {
			t.Error("Direction")
			assertEq(t, expected, actual)
		}
	}
	if 0 < expected.Category {
		if expected.Category != actual.Category {
			t.Error("Category")
			assertEq(t, expected, actual)
		}
	}
	if 0 < expected.Command {
		if expected.Command != actual.Command {
			t.Error("Command")
			assertEq(t, expected, actual)
		}
	}
	if 0 < expected.BodySize {
		if expected.BodySize != actual.BodySize {
			t.Error("BodySize")
			assertEq(t, expected, actual)
		}
	}
	if 0 < expected.Seq {
		if expected.Seq != actual.Seq {
			t.Error("Seq")
			assertEq(t, expected, actual)
		}
	}
	if 0 < expected.Status {
		if expected.Status != actual.Status {
			t.Error("Status")
			assertEq(t, expected, actual)
		}
	}
	if 0 < len(expected.Body) {
		a := hex.EncodeToString(expected.Body)
		b := hex.EncodeToString(actual.Body)
		if a != b {
			t.Error("Body")
			assertEq(t, expected, actual)
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

func prepareLoggedInUser(t *testing.T, lbs *Lbs, platform, disk string, user DBUser) (*TestLbsClient, func()) {
	nw := NewPipeNetwork()
	p := lbs.NewPeer(nw.Server)
	go p.serve()

	cli := &TestLbsClient{t: t, DBUser: user, conn: nw.Client}
	// ignore first message
	msg := cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskConnectionID}, msg)

	p.app.Locked(func(_ *Lbs) {
		p.Platform = platform
		p.GameDisk = disk
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
			return
		}

		lobby := lbs.GetLobby(p.Platform, p.GameDisk, lobbyID)
		if lobby == nil {
			t.Fatal("lobby not found")
			return
		}

		p.Team = team
		p.Lobby = lobby
		lobby.Users[p.UserID] = &p.DBUser
	})
}

func forceEnterRoom(t *testing.T, lbs *Lbs, cli *TestLbsClient, roomID uint16) {
	lbs.Locked(func(*Lbs) {
		p := lbs.FindPeer(cli.UserID)
		if p == nil {
			t.Fatal("user not found", cli.DBUser)
			return
		}

		if p.Lobby == nil {
			t.Fatal("not in a lobby")
			return
		}

		if p.Team == TeamNone {
			t.Fatal("no team")
			return
		}

		r := p.Lobby.FindRoom(p.Team, roomID)
		if r == nil {
			t.Fatal("no room", p.Lobby.ID, roomID)
			return
		}

		r.Enter(&cli.DBUser)
		p.Room = r
	})
}

func TestLobbyChatSameLobby(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	user1, cancel1 := prepareLoggedInUser(t, lbs, PlatformConsole, GameDiskDC2, DBUser{UserID: "TEST01", Name: "NAME01"})
	defer cancel1()
	forceEnterLobby(t, lbs, user1, 1, TeamRenpo)

	user2, cancel2 := prepareLoggedInUser(t, lbs, PlatformConsole, GameDiskDC2, DBUser{UserID: "TEST02", Name: "NAME02"})
	defer cancel2()
	forceEnterLobby(t, lbs, user2, 1, TeamRenpo)

	text := "HELLO WORLD"
	user1.MustWriteMessage(NewClientNotice(lbsPostChatMessage).Writer().WriteString(text).Msg())

	msg := user1.MustReadMessageSkipNoticeUntil(lbsChatMessage)
	AssertMsg(t, &LbsMessage{
		Category:  CategoryNotice,
		Direction: ServerToClient,
		Command:   lbsChatMessage,
		Body:      hexbytes("000654455354303100064e414d453031000b48454c4c4f20574f524c4400000000"),
	}, msg)

	msg = user2.MustReadMessageSkipNoticeUntil(lbsChatMessage)
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

	user1, cancel1 := prepareLoggedInUser(t, lbs, PlatformConsole, GameDiskDC2, DBUser{
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

func TestLbs_PlatformInfo(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	user1, cancel1 := prepareLoggedInUser(t, lbs, "", "", DBUser{
		UserID: "TEST01",
		Name:   "NAME01",
	})
	defer cancel1()

	user1.MustWriteMessage(NewClientCustom(lbsPlatformInfo).Writer().WriteString(samplePlatformInfo).Msg())

	time.Sleep(time.Millisecond) // FIXME

	called := false
	lbs.Locked(func(*Lbs) {
		p := lbs.FindPeer(user1.UserID)
		assertEq(t, PlatformEmuX8664, p.Platform)
		assertEq(t, "asia-northeast1", p.bestRegion)
		called = true
	})
	assertEq(t, true, called)
}

func Test_LoginFlowNewUser(t *testing.T) {
	nw := NewPipeNetwork()
	defer nw.Close()

	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()
	go lbs.NewPeer(nw.Server).serve()

	cli := &TestLbsClient{t: t, conn: nw.Client}
	var msg *LbsMessage

	cli.MustWriteMessage(NewClientCustom(lbsPlatformInfo).Writer().WriteString(samplePlatformInfo).Msg())

	// Connection ID exchange
	{

		msg = cli.MustReadMessage()
		AssertMsg(t,
			&LbsMessage{Command: lbsAskConnectionID},
			msg)
		cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(hexbytes("0000000000000000")).Msg())

		msg = cli.MustReadMessage()
		AssertMsg(t,
			&LbsMessage{Command: lbsConnectionID},
			msg)
		connectionID := msg.Reader().ReadString()
		if connectionID == "" {
			t.Fatal(msg)
		}

		cli.MustWriteMessage(NewClientAnswer(msg))
	}

	// Regulation text requests
	{
		AssertMsg(t,
			&LbsMessage{Command: lbsWarningMessage},
			cli.MustReadMessage())

		cli.MustWriteMessage(NewClientQuestion(lbsRegulationHeader).Writer().WriteBytes(hexbytes("31303030")).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{Command: lbsRegulationHeader}, msg)
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
		msg = cli.MustReadMessageSkipNoticeUntil(lbsPostGameParameter)
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

func TestLbs_LobbyListFlow(t *testing.T) {
	lobbyDC1 := []uint16{2, 4, 5, 6, 9, 10, 11, 12, 13, 16, 17, 22}
	lobbyDC2 := []uint16{2, 4, 5, 6, 9, 10, 11, 12, 13, 14, 15, 16, 17, 19, 20, 21, 22}
	lobbyPS2 := []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22}

	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "console dc1",
			platform: PlatformConsole,
			disk:     GameDiskDC1,
		},
		{
			name:     "console dc2",
			platform: PlatformConsole,
			disk:     GameDiskDC2,
		},
		{
			name:     "console ps2",
			platform: PlatformConsole,
			disk:     GameDiskPS2,
		},
		{
			name:     "emu dc1",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC1,
		},
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
		{
			name:     "emu ps2",
			platform: PlatformEmuX8664,
			disk:     GameDiskPS2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			cli, cancel := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel()

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsStartLobby, Direction: ClientToServer, Category: CategoryNotice, Seq: 0, Status: StatusSuccess})

			AssertMsg(t,
				&LbsMessage{Command: lbsStartLobby, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessage())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsPlazaMax, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})

			AssertMsg(t,
				&LbsMessage{Command: lbsPlazaMax, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0016")},
				cli.MustReadMessage())

			var lobbyList []uint16
			if tt.disk == GameDiskDC1 {
				lobbyList = lobbyDC1
			}
			if tt.disk == GameDiskDC2 {
				lobbyList = lobbyDC2
			}
			if tt.disk == GameDiskPS2 {
				lobbyList = lobbyPS2
			}

			for _, lobbyID := range lobbyList {
				cli.MustWriteMessage(
					(&LbsMessage{Command: lbsPlazaJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess}).Writer().Write16(uint16(lobbyID)).Msg())

				msg := cli.MustReadMessage()
				if tt.disk == GameDiskPS2 {
					AssertMsg(t, &LbsMessage{Command: lbsPlazaJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4}, msg)
				}
				if tt.disk == GameDiskDC1 || tt.disk == GameDiskDC2 {
					AssertMsg(t, &LbsMessage{Command: lbsPlazaJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6}, msg)
				}
				assertEq(t, lobbyID, msg.Reader().Read16())

				cli.MustWriteMessage(
					(&LbsMessage{Command: lbsPlazaStatus, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess}).Writer().Write16(uint16(lobbyID)).Msg())

				msg = cli.MustReadMessage()
				AssertMsg(t, &LbsMessage{Command: lbsPlazaStatus, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 3}, msg)
				assertEq(t, lobbyID, msg.Reader().Read16())

				cli.MustWriteMessage(
					(&LbsMessage{Command: lbsPlazaExplain, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess}).Writer().Write16(uint16(lobbyID)).Msg())
				msg = cli.MustReadMessage()
				AssertMsg(t, &LbsMessage{Command: lbsPlazaExplain, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess}, msg)
			}
		})
	}
}

func TestLbs_LobbyEnterFlow(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "console dc1",
			platform: PlatformConsole,
			disk:     GameDiskDC1,
		},
		{
			name:     "console dc2",
			platform: PlatformConsole,
			disk:     GameDiskDC2,
		},
		{
			name:     "console ps2",
			platform: PlatformConsole,
			disk:     GameDiskPS2,
		},
		{
			name:     "emu dc1",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC1,
		},
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
		{
			name:     "emu ps2",
			platform: PlatformEmuX8664,
			disk:     GameDiskPS2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			cli, cancel := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel()

			// Select a lobby
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsPlazaEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0002")})
			AssertMsg(t,
				&LbsMessage{Command: lbsPlazaEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			// Team select scene of the lobby
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
			if tt.disk == GameDiskPS2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00010000")},
					cli.MustReadMessageSkipNotice())
			} else {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000100000000")},
					cli.MustReadMessageSkipNotice())
			}
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0002")})
			if tt.disk == GameDiskPS2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00020000")},
					cli.MustReadMessageSkipNotice())
			} else {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000200000000")},
					cli.MustReadMessageSkipNotice())
			}

			// Decide a team
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsLobbyEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			// In lobby chat
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
			if tt.disk == GameDiskPS2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00010001")},
					cli.MustReadMessageSkipNotice())
			} else if tt.disk == GameDiskDC1 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000100010000")},
					cli.MustReadMessageSkipNotice())
			} else if tt.disk == GameDiskDC2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000100000001")},
					cli.MustReadMessageSkipNotice())
			}

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyMatchingJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsLobbyMatchingJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00010000")},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyMatchingJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0002")})
			AssertMsg(t,
				&LbsMessage{Command: lbsLobbyMatchingJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00020000")},
				cli.MustReadMessageSkipNotice())

			// Exit lobby chat and move to team select scene
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyExit, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
			AssertMsg(t,
				&LbsMessage{Command: lbsLobbyExit, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			// Team select scene
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
			if tt.disk == GameDiskPS2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00010000")},
					cli.MustReadMessageSkipNotice())
			} else {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000100000000")},
					cli.MustReadMessageSkipNotice())
			}
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsLobbyJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0002")})
			if tt.disk == GameDiskPS2 {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00020000")},
					cli.MustReadMessageSkipNotice())
			} else {
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 6, Body: hexbytes("000200000000")},
					cli.MustReadMessageSkipNotice())
			}

			// Exit the lobby
			cli.MustWriteMessage(
				&LbsMessage{Command: lbsPlazaExit, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
			AssertMsg(t,
				&LbsMessage{Command: lbsPlazaExit, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())
		})
	}
}

func TestLbs_RoomEnterFlow(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "console dc1",
			platform: PlatformConsole,
			disk:     GameDiskDC1,
		},
		{
			name:     "console dc2",
			platform: PlatformConsole,
			disk:     GameDiskDC2,
		},
		{
			name:     "console ps2",
			platform: PlatformConsole,
			disk:     GameDiskPS2,
		},
		{
			name:     "emu dc1",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC1,
		},
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
		{
			name:     "emu ps2",
			platform: PlatformEmuX8664,
			disk:     GameDiskPS2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			user1, cancel1 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel1()

			user2, cancel2 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST02", Name: "NAME02"})
			defer cancel2()

			lobbyID := uint16(2)
			roomCount := fmt.Sprintf("%04d", maxRoomCount)

			forceEnterLobby(t, lbs, user1, lobbyID, TeamRenpo)
			forceEnterLobby(t, lbs, user2, lobbyID, TeamRenpo)

			{
				cli := user1

				// Get room list
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsRoomMax, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsRoomMax, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomCount)},
					cli.MustReadMessageSkipNotice())

				for i := 0; i < maxRoomCount; i++ {
					roomID := fmt.Sprintf("%04d", i+1)
					roomStatus := fmt.Sprintf("%02d", RoomStateEmpty)

					cli.MustWriteMessage(
						&LbsMessage{Command: lbsRoomStatus, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomID)})
					AssertMsg(t,
						&LbsMessage{Command: lbsRoomStatus, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 3, Body: hexbytes(roomID + roomStatus)},
						cli.MustReadMessageSkipNotice())

					cli.MustWriteMessage(
						&LbsMessage{Command: lbsRoomTitle, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomID)})
					AssertMsg(t,
						&LbsMessage{Command: lbsRoomTitle, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes(roomID + "0000")},
						cli.MustReadMessageSkipNotice())
				}

				// Create room on roomID = 1
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsRoomCreate, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")})
				AssertMsg(t,
					&LbsMessage{Command: lbsRoomCreate, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())

				// Put room name (= User name)
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsPutRoomName, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess,
						BodySize: 24, Body: hexbytes("001647465a4f484682a082a082a082a082a082a082a082a0")})
				AssertMsg(t,
					&LbsMessage{Command: lbsPutRoomName, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsEndRoomCreate, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsEndRoomCreate, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())

				// Start waiting for other player's join
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsWaitJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsWaitJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")},
					cli.MustReadMessageSkipNotice())
			}

			{
				cli := user2

				// Get room list
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsRoomMax, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsRoomMax, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomCount)},
					cli.MustReadMessageSkipNotice())

				for i := 0; i < maxRoomCount; i++ {
					roomID := fmt.Sprintf("%04d", i+1)
					roomStatus := fmt.Sprintf("%02d", RoomStateEmpty)
					roomName := "0000"
					if i == 0 {
						roomStatus = fmt.Sprintf("%02d", RoomStateRecruiting)
						roomName = "001647465a4f484682a082a082a082a082a082a082a082a0"
					}

					cli.MustWriteMessage(
						&LbsMessage{Command: lbsRoomStatus, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomID)})
					AssertMsg(t,
						&LbsMessage{Command: lbsRoomStatus, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 3, Body: hexbytes(roomID + roomStatus)},
						cli.MustReadMessageSkipNotice())

					cli.MustWriteMessage(
						&LbsMessage{Command: lbsRoomTitle, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes(roomID)})
					AssertMsg(t,
						&LbsMessage{Command: lbsRoomTitle, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, Body: hexbytes(roomID + roomName)},
						cli.MustReadMessageSkipNotice())
				}
			}

			// Enter the room
			user2.MustWriteMessage(
				&LbsMessage{Command: lbsRoomEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 4, Body: hexbytes("00010000")})
			AssertMsg(t,
				&LbsMessage{Command: lbsRoomEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				user2.MustReadMessageSkipNotice())

			// User1 Receive the notification
			AssertMsg(t,
				&LbsMessage{Command: lbsRoomCommer}, // TODO: Check body
				user1.MustReadMessageSkipNoticeUntil(lbsRoomCommer))

			for _, cli := range []*TestLbsClient{user1, user2} {
				// I'm ready to battle
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsMatchingEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("01")})
				AssertMsg(t,
					&LbsMessage{Command: lbsMatchingEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsWaitJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsWaitJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0002")},
					cli.MustReadMessageSkipNotice())
			}

			// user2 leaves the room
			user2.MustWriteMessage(
				&LbsMessage{Command: lbsMatchingEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
			AssertMsg(t,
				&LbsMessage{Command: lbsMatchingEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				user2.MustReadMessageSkipNotice())

			// user1 know user2 have been left the room
			AssertMsg(t,
				&LbsMessage{Command: lbsWaitJoin, Direction: ServerToClient, Category: CategoryNotice, Status: StatusSuccess, BodySize: 2, Body: hexbytes("0001")},
				user1.MustReadMessageSkipNoticeUntil(lbsWaitJoin))
			AssertMsg(t,
				&LbsMessage{Command: lbsRoomLeaver, Direction: ServerToClient, Category: CategoryNotice, Status: StatusSuccess}, // TODO: check body
				user1.MustReadMessageSkipNoticeUntil(lbsRoomLeaver))

			// I'm not ready to battle
			user1.MustWriteMessage(
				&LbsMessage{Command: lbsMatchingEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
			AssertMsg(t,
				&LbsMessage{Command: lbsMatchingEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				user1.MustReadMessageSkipNotice())

			// user1 leaves the room
			user1.MustWriteMessage(
				&LbsMessage{Command: lbsRoomExit, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
			AssertMsg(t,
				&LbsMessage{Command: lbsRoomExit, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				user1.MustReadMessageSkipNotice())
		})
	}
}

func TestLbs_LobbyMatchingFlow(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "console dc1",
			platform: PlatformConsole,
			disk:     GameDiskDC1,
		},
		{
			name:     "console dc2",
			platform: PlatformConsole,
			disk:     GameDiskDC2,
		},
		{
			name:     "console ps2",
			platform: PlatformConsole,
			disk:     GameDiskPS2,
		},
		{
			name:     "emu dc1",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC1,
		},
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
		{
			name:     "emu ps2",
			platform: PlatformEmuX8664,
			disk:     GameDiskPS2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf.BattlePublicAddr = "192.168.1.10:9877"
			hexBattlePublicAddr := "0004c0a8010a00022695"
			lobbyID := uint16(2)

			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			user1, cancel1 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel1()

			user2, cancel2 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST02", Name: "NAME02"})
			defer cancel2()

			user3, cancel3 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST03", Name: "NAME03"})
			defer cancel3()

			user4, cancel4 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST04", Name: "NAME04"})
			defer cancel4()

			forceEnterLobby(t, lbs, user1, lobbyID, TeamRenpo)
			forceEnterLobby(t, lbs, user2, lobbyID, TeamRenpo)
			forceEnterLobby(t, lbs, user3, lobbyID, TeamZeon)
			forceEnterLobby(t, lbs, user4, lobbyID, TeamZeon)

			lbs.Locked(func(*Lbs) {
				lobby := lbs.GetLobby(tt.platform, tt.disk, lobbyID)
				lobby.LobbySetting.TeamShuffle = false
			})

			clients := []*TestLbsClient{user1, user2, user3, user4}

			// Entry
			for _, cli := range clients {
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsLobbyMatchingEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("01")})
				AssertMsg(t,
					&LbsMessage{Command: lbsLobbyMatchingEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())
			}

			// Receive lbsReadyBattle
			for _, cli := range clients {
				AssertMsg(t,
					&LbsMessage{Command: lbsReadyBattle, Direction: ServerToClient, Category: CategoryNotice, Status: StatusSuccess},
					cli.MustReadMessageSkipNoticeUntil(lbsReadyBattle))
			}

			// Ask Match information
			for i, cli := range clients {
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskMatchingJoin, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskMatchingJoin, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("04")},
					cli.MustReadMessageSkipNotice())

				myPos := fmt.Sprintf("%02d", i+1)
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskPlayerSide, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskPlayerSide, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes(myPos)},
					cli.MustReadMessageSkipNotice())

				for j := 0; j < len(clients); j++ {
					askPos := fmt.Sprintf("%02d", i+1)
					cli.MustWriteMessage(
						&LbsMessage{Command: lbsAskPlayerInfo, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes(askPos)})
					AssertMsg(t,
						&LbsMessage{Command: lbsAskPlayerInfo, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
						cli.MustReadMessageSkipNotice())
					// TODO: check body
				}

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskRuleData, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskRuleData, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 39},
					cli.MustReadMessageSkipNotice())
				// TODO: check body

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskBattleCode, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskBattleCode, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())
				// TODO: check body

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskMcsVersion, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskMcsVersion, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("0a")},
					cli.MustReadMessageSkipNotice())

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsAskMcsAddress, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess})
				AssertMsg(t,
					&LbsMessage{Command: lbsAskMcsAddress, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 10, Body: hexbytes(hexBattlePublicAddr)},
					cli.MustReadMessageSkipNotice())

				cli.MustWriteMessage(
					&LbsMessage{Command: lbsLogout, Direction: ClientToServer, Category: CategoryNotice, Seq: 0, Status: StatusSuccess})
			}
		})
	}
}

func TestLbs_RoomMatchingFlow(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "console dc1",
			platform: PlatformConsole,
			disk:     GameDiskDC1,
		},
		{
			name:     "console dc2",
			platform: PlatformConsole,
			disk:     GameDiskDC2,
		},
		{
			name:     "console ps2",
			platform: PlatformConsole,
			disk:     GameDiskPS2,
		},
		{
			name:     "emu dc1",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC1,
		},
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
		{
			name:     "emu ps2",
			platform: PlatformEmuX8664,
			disk:     GameDiskPS2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf.BattlePublicAddr = "192.168.1.10:9877"
			lobbyID := uint16(2)

			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			user1, cancel1 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel1()

			user2, cancel2 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST02", Name: "NAME02"})
			defer cancel2()

			user3, cancel3 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST03", Name: "NAME03"})
			defer cancel3()

			user4, cancel4 := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST04", Name: "NAME04"})
			defer cancel4()

			forceEnterLobby(t, lbs, user1, lobbyID, TeamRenpo)
			forceEnterLobby(t, lbs, user2, lobbyID, TeamRenpo)
			forceEnterLobby(t, lbs, user3, lobbyID, TeamZeon)
			forceEnterLobby(t, lbs, user4, lobbyID, TeamZeon)

			forceEnterRoom(t, lbs, user1, 1)
			forceEnterRoom(t, lbs, user2, 1)
			forceEnterRoom(t, lbs, user3, 1)
			forceEnterRoom(t, lbs, user4, 1)

			clients := []*TestLbsClient{user1, user2, user3, user4}

			for _, cli := range clients {
				cli.MustWriteMessage(
					&LbsMessage{Command: lbsMatchingEntry, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("01")})
				AssertMsg(t,
					&LbsMessage{Command: lbsMatchingEntry, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
					cli.MustReadMessageSkipNotice())
			}

			// Receive lbsReadyBattle
			for _, cli := range clients {
				AssertMsg(t,
					&LbsMessage{Command: lbsReadyBattle, Direction: ServerToClient, Category: CategoryNotice, Status: StatusSuccess},
					cli.MustReadMessageSkipNoticeUntil(lbsReadyBattle))

				// Skip subsequent flow because it is the same as Lobby.
			}
		})
	}
}

func TestLbs_RankingListFlow(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		disk     string
	}{
		{
			name:     "emu dc2",
			platform: PlatformEmuX8664,
			disk:     GameDiskDC2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getDB().(SQLiteDB).Exec(`DELETE FROM user`)
			must(t, err)

			getDB().(SQLiteDB).SQLiteCache.deleteRankingCache()

			mustInsertDBUser(DBUser{UserID: "RANK01", Name: "USER01", WinCount: 1000})
			mustInsertDBUser(DBUser{UserID: "RANK02", Name: "USER02", WinCount: 900})
			mustInsertDBUser(DBUser{UserID: "RANK03", Name: "USER03", WinCount: 800})
			mustInsertDBUser(DBUser{UserID: "RANK04", Name: "USER04", WinCount: 700})
			mustInsertDBUser(DBUser{UserID: "RANK05", Name: "USER05", WinCount: 600})
			mustInsertDBUser(DBUser{UserID: "RANK06", Name: "USER06", WinCount: 500})
			mustInsertDBUser(DBUser{UserID: "RANK07", Name: "USER07", WinCount: 400})
			mustInsertDBUser(DBUser{UserID: "RANK08", Name: "USER08", WinCount: 300})
			mustInsertDBUser(DBUser{UserID: "RANK09", Name: "USER09", WinCount: 200})
			mustInsertDBUser(DBUser{UserID: "RANK10", Name: "USER10", WinCount: 100})

			lbs := NewLbs()
			defer lbs.Quit()
			go lbs.eventLoop()

			cli, cancel := prepareLoggedInUser(t, lbs, tt.platform, tt.disk, DBUser{UserID: "TEST01", Name: "NAME01"})
			defer cancel()

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRankingSuu, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRankingSuu, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 2, Body: hexbytes("000a")},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsWinLose, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 1, Body: hexbytes("00")})
			AssertMsg(t,
				&LbsMessage{Command: lbsWinLose, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess, BodySize: 18},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRanking, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 5, Body: hexbytes("0000010001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRanking, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRanking, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 5, Body: hexbytes("0000020001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRanking, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRanking, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 5, Body: hexbytes("0000030001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRanking, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRanking, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 5, Body: hexbytes("0000040001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRanking, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())

			cli.MustWriteMessage(
				&LbsMessage{Command: lbsTopRanking, Direction: ClientToServer, Category: CategoryQuestion, Seq: 0, Status: StatusSuccess, BodySize: 5, Body: hexbytes("0000050001")})
			AssertMsg(t,
				&LbsMessage{Command: lbsTopRanking, Direction: ServerToClient, Category: CategoryAnswer, Seq: 0, Status: StatusSuccess},
				cli.MustReadMessageSkipNotice())
		})
	}
}
