package main

import (
	"net"
	"testing"
	"time"
)

// dialWithRetry connects to addr, retrying briefly while the server's listener
// comes up in its goroutine.
func dialWithRetry(t *testing.T, addr string, timeout time.Duration) net.Conn {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			return conn
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("could not connect to %s within %s", addr, timeout)
	return nil
}

// freeTCPAddr binds an ephemeral port and releases it, returning the address so
// Lbs.ListenAndServe (which does not expose its listener) can bind it. The
// small reuse window is acceptable in a test.
func freeTCPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	addr := l.Addr().String()
	must(t, l.Close())
	return addr
}

// loginFlowNewUser drives the LBS login handshake, over whatever net.Conn the
// client wraps, from the first packet through to lbsLoginOk, and returns the
// registered user id. It mirrors Test_LoginFlowNewUser (the in-process pipe
// version) up to the login-ok milestone. loginKey is the raw phone-number bytes
// the client presents; the server derives the account from an FNV hash of them
// and auto-creates it, so passing bytes unique to this test gives it a private
// account with an empty user list and no coupling to other tests.
func loginFlowNewUser(t *testing.T, cli *TestLbsClient, loginKey []byte) string {
	t.Helper()

	cli.MustWriteMessage(NewClientCustom(lbsPlatformInfo).Writer().WriteString(samplePlatformInfo).Msg())

	// Connection ID exchange.
	msg := cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskConnectionID}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(hexbytes("0000000000000000")).Msg())

	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsConnectionID}, msg)
	if msg.Reader().ReadString() == "" {
		t.Fatal("empty connection id")
	}
	cli.MustWriteMessage(NewClientAnswer(msg))

	// Warning + regulation text.
	AssertMsg(t, &LbsMessage{Command: lbsWarningMessage}, cli.MustReadMessage())

	cli.MustWriteMessage(NewClientQuestion(lbsRegulationHeader).Writer().WriteBytes(hexbytes("31303030")).Msg())
	AssertMsg(t, &LbsMessage{Command: lbsRegulationHeader}, cli.MustReadMessage())
	AssertMsg(t, &LbsMessage{Command: lbsRegulationText}, cli.MustReadMessage())
	AssertMsg(t, &LbsMessage{Command: lbsRegulationFooter}, cli.MustReadMessage())

	// Login type.
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsLoginType}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write8(2).Msg())

	// User info: the loginkey bytes become the account key.
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsUserInfo1}, msg)
	cli.MustWriteMessage(NewClientNotice(lbsEncodeStart))
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(loginKey).Msg())

	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsUserInfo9}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg))

	// User list is empty for this fresh account -> register a new user.
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsUserHandle}, msg)
	if msg.Reader().Read8() != 0 {
		t.Fatal("expected an empty user list for a fresh account")
	}
	cli.MustWriteMessage(NewClientQuestion(lbsUserRegist).Writer().
		WriteString("******").WriteBytes(hexbytes("82a082a282a482a682a8")).Msg())

	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsUserRegist}, msg)
	userID := msg.Reader().ReadString()
	if len(userID) != 6 {
		t.Fatalf("invalid user id length: %q", userID)
	}

	cli.MustWriteMessage(NewClientQuestion(lbsUserDecide).Writer().WriteString(userID).Msg())
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsUserDecide}, msg)
	if msg.Reader().ReadString() != userID {
		t.Fatal("unexpected user id in decide answer")
	}

	// Game code / battle result.
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskGameCode}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write8(3).Write8(1).Msg())

	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskBattleResult}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write(hexbytes("000e000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")).Msg())

	// Login OK.
	msg = cli.MustReadMessageSkipNoticeUntil(lbsLoginOk)
	AssertMsg(t, &LbsMessage{Command: lbsLoginOk}, msg)
	return userID
}

// TestLbs_TCPLoginIntegration is a process-level integration test: it starts the
// real Lbs over an actual TCP listener (ListenAndServe, which also runs the
// event loop and UDP responder) and drives a real socket client through the
// full login handshake to lbsLoginOk, then confirms the user was persisted.
// Unlike the pipe-based flow tests, this exercises net.ListenTCP / AcceptTCP and
// the per-connection socket read loop end to end.
func TestLbs_TCPLoginIntegration(t *testing.T) {
	addr := freeTCPAddr(t)

	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.ListenAndServe(addr)

	conn := dialWithRetry(t, addr, 5*time.Second)
	defer conn.Close()
	cli := &TestLbsClient{t: t, conn: conn}

	// Bytes unique to this test so the derived account is private to it.
	userID := loginFlowNewUser(t, cli, hexbytes("a1b2c3d4"))

	// The handshake reached lbsLoginOk; the registered user must be persisted.
	u, err := getDB().GetUser(userID)
	must(t, err)
	if u == nil || u.UserID != userID {
		t.Fatalf("user %q was not persisted", userID)
	}
}
