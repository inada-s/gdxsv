package main

import (
	"reflect"
	"testing"
	"time"
)

// loginToUserList drives a client from the connection id exchange to the user list
// and returns its new session id. loginType 2 logs in with loginKey, 3 comes back
// from a battle with lastSessionID.
func loginToUserList(t *testing.T, cli *TestLbsClient, lastSessionID string, loginType byte, loginKey []byte) string {
	t.Helper()
	cli.MustWriteMessage(NewClientCustom(lbsPlatformInfo).Writer().WriteString(samplePlatformInfo).Msg())

	msg := cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsAskConnectionID}, msg)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteString(lastSessionID).Msg())
	msg = cli.MustReadMessage()
	AssertMsg(t, &LbsMessage{Command: lbsConnectionID}, msg)
	sessionID := msg.Reader().ReadString()
	cli.MustWriteMessage(NewClientAnswer(msg))

	AssertMsg(t, &LbsMessage{Command: lbsWarningMessage}, cli.MustReadMessage())
	cli.MustWriteMessage(NewClientQuestion(lbsRegulationHeader).Writer().WriteBytes(hexbytes("31303030")).Msg())
	msg = cli.MustReadMessageSkipNoticeUntil(lbsLoginType)
	cli.MustWriteMessage(NewClientAnswer(msg).Writer().Write8(loginType).Msg())

	if loginType == 2 {
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{Command: lbsUserInfo1}, msg)
		cli.MustWriteMessage(NewClientNotice(lbsEncodeStart))
		cli.MustWriteMessage(NewClientAnswer(msg).Writer().WriteBytes(loginKey).Msg())
		msg = cli.MustReadMessage()
		AssertMsg(t, &LbsMessage{Command: lbsUserInfo9}, msg)
		cli.MustWriteMessage(NewClientAnswer(msg))
	}
	AssertMsg(t, &LbsMessage{Command: lbsUserHandle}, cli.MustReadMessage())
	return sessionID
}

func TestLbs_UserOfAnotherAccountRefused(t *testing.T) {
	addr := freeTCPAddr(t)
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.ListenAndServe(addr)

	dial := func() *TestLbsClient {
		conn := dialWithRetry(t, addr, 5*time.Second)
		t.Cleanup(func() { conn.Close() })
		return &TestLbsClient{t: t, conn: conn}
	}

	keyA, keyB := hexbytes("a5a5a5a501"), hexbytes("b5b5b5b502")
	userB := loginFlowNewUser(t, dial(), keyB)
	stored, err := getDB().GetUser(userB)
	must(t, err)
	userA := loginFlowNewUser(t, dial(), keyA)
	peerOf := func(userID string) (p *LbsPeer) {
		lbs.Locked(func(l *Lbs) { p = l.userPeers[userID] })
		return
	}
	peerB := peerOf(userB)
	if peerB == nil {
		t.Fatal("no peer for", userB)
	}

	refused := func(path string, cli *TestLbsClient, m *LbsMessage) {
		t.Helper()
		cli.MustWriteMessage(m)
		for {
			msg := new(LbsMessage)
			err := readMessageWithTimeout(cli.conn, msg, 5*time.Second)
			if err == errTimeout {
				t.Fatalf("%s: connection still open", path)
			}
			if err != nil {
				break
			}
			if msg.Command == m.Command {
				t.Fatalf("%s: user %s of another account accepted", path, userB)
			}
		}
		u, err := getDB().GetUser(userB)
		must(t, err)
		if !reflect.DeepEqual(u, stored) {
			t.Fatalf("%s: stored user changed\n%+v\n%+v", path, stored, u)
		}
		if p := peerOf(userB); p != peerB {
			t.Fatalf("%s: %s held by %v", path, userB, p)
		}
	}

	cli := dial()
	loginToUserList(t, cli, "", 2, keyA)
	refused("regist", cli, NewClientQuestion(lbsUserRegist).Writer().
		WriteString(userB).WriteBytes(hexbytes("82a0")).Msg())

	cli = dial()
	session := loginToUserList(t, cli, "", 2, keyA)
	refused("decide", cli, NewClientQuestion(lbsUserDecide).Writer().WriteString(userB).Msg())

	cli = dial()
	session = loginToUserList(t, cli, session, 3, nil)
	refused("decide after battle", cli, NewClientQuestion(lbsUserDecide).Writer().WriteString(userB).Msg())

	cli = dial()
	loginToUserList(t, cli, session, 3, nil)
	cli.MustWriteMessage(NewClientQuestion(lbsUserDecide).Writer().WriteString(userA).Msg())
	if got := cli.MustReadMessageSkipNoticeUntil(lbsUserDecide).Reader().ReadString(); got != userA {
		t.Fatalf("own user after battle: %q", got)
	}
}
