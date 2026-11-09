package main

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestLbs_AccountUserLimit(t *testing.T) {
	cleanTables(t, "user", "account")
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	nSession := 0
	connect := func(loginKey string) *TestLbsClient {
		t.Helper()
		ac, err := getDB().GetAccountByLoginKey(loginKey)
		must(t, err)
		nSession++
		sid := fmt.Sprintf("%08d", 11110000+nSession)
		must(t, getDB().LoginAccount(ac, sid, "127.0.0.1", ""))
		nw := NewPipeNetwork()
		t.Cleanup(func() { nw.Close() })
		p := lbs.NewPeer(nw.Server)
		p.SessionID = sid
		go p.serve()
		return &TestLbsClient{t: t, conn: nw.Client}
	}
	regist := func(cli *TestLbsClient) (string, error) {
		cli.MustWriteMessage(NewClientQuestion(lbsUserRegist).Writer().
			WriteString("******").WriteBytes(hexbytes("82a0")).Msg())
		for {
			msg := &LbsMessage{}
			if err := readMessageWithTimeout(cli.conn, msg, 5*time.Second); err != nil {
				return "", err
			}
			if msg.Command == lbsUserRegist {
				return msg.Reader().ReadString(), nil
			}
		}
	}
	decide := func(loginKey, userID string) {
		t.Helper()
		cli := connect(loginKey)
		cli.MustWriteMessage(NewClientQuestion(lbsUserDecide).Writer().WriteString(userID).Msg())
		msg := cli.MustReadMessageSkipNoticeUntil(lbsUserDecide)
		if got := msg.Reader().ReadString(); got != userID {
			t.Fatalf("decide %s: got %s", userID, got)
		}
	}
	listed := func(loginKey string) (ids []string) {
		t.Helper()
		cli := connect(loginKey)
		cli.MustWriteMessage(NewClientAnswer(NewServerQuestion(lbsUserInfo9)))
		r := cli.MustReadMessageSkipNoticeUntil(lbsUserHandle).Reader()
		for n := r.Read8(); n > 0; n-- {
			ids = append(ids, r.ReadString())
			r.ReadString()
		}
		sort.Strings(ids)
		return
	}
	countUsers := func(loginKey string) (n int) {
		must(t, getDB().(SQLiteDB).Get(&n, `SELECT COUNT(*) FROM user WHERE login_key = ?`, loginKey))
		return
	}
	refused := func(loginKey string, want int, what string) {
		t.Helper()
		if id, err := regist(connect(loginKey)); err == nil {
			t.Fatalf("%s: registered %s", what, id)
		}
		if n := countUsers(loginKey); n != want {
			t.Fatalf("%s: %d users, want %d", what, n, want)
		}
	}

	ac, err := getDB().RegisterAccount("127.0.0.1")
	must(t, err)
	var made []string
	for i := 0; i < maxUsersPerAccount; i++ {
		id, err := regist(connect(ac.LoginKey))
		must(t, err)
		made = append(made, id)
	}
	refused(ac.LoginKey, maxUsersPerAccount, "user over the limit")
	sort.Strings(made)
	if got := listed(ac.LoginKey); fmt.Sprint(got) != fmt.Sprint(made) {
		t.Fatalf("listed %v, made %v", got, made)
	}
	for _, id := range made {
		decide(ac.LoginKey, id)
	}

	over, err := getDB().RegisterAccount("127.0.0.1")
	must(t, err)
	overUsers := []string{"OVER01", "OVER02", "OVER03", "OVER04"}
	for _, id := range overUsers {
		_, err := getDB().(SQLiteDB).Exec(`INSERT INTO user (user_id, login_key, created) VALUES (?, ?, ?)`, id, over.LoginKey, time.Now())
		must(t, err)
	}
	refused(over.LoginKey, len(overUsers), "account over the limit")
	if got := listed(over.LoginKey); fmt.Sprint(got) != fmt.Sprint(overUsers) {
		t.Fatalf("account over the limit: listed %v", got)
	}
	decide(over.LoginKey, "OVER04")
}
