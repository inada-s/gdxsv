package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"

	"github.com/golang/glog"

	"gdxsv/gdxsv/battle"
)

const (
	maxLobbyCount = 3
	maxRoomCount  = 0
)

const (
	EntryNone  = 0
	EntryRenpo = 1
	EntryZeon  = 2
)

type eventPeerCome struct {
	peer *AppPeer
}

type eventPeerLeave struct {
	peer *AppPeer
}

type eventPeerMessage struct {
	peer *AppPeer
	msg  *Message
}

type eventFunc struct {
	f func(*App)
	c chan<- interface{}
}

type App struct {
	handlers     map[CmdID]MessageHandler
	battleServer *rpc.Client
	users        map[string]*AppPeer
	lobbys       map[uint16]*Lobby
	battles      map[string]*Battle
	chEvent      chan interface{}
	chQuit       chan interface{}
}

func NewApp() *App {
	app := &App{
		handlers: defaultHandlers,
		users:    make(map[string]*AppPeer),
		lobbys:   make(map[uint16]*Lobby),
		battles:  make(map[string]*Battle),
		chEvent:  make(chan interface{}, 64),
		chQuit:   make(chan interface{}),
	}
	for i := 1; i <= maxLobbyCount; i++ {
		app.lobbys[uint16(i)] = NewLobby(app, uint16(i))
	}
	return app
}

func (s *App) ListenAndServeBattle(addr string) error {
	hub := battle.NewLogic()
	tcpSv := battle.NewTCPServer(hub)
	return tcpSv.ListenAndServe(addr)
}

func (s *App) ListenAndServe(addr string) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	listner, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	go s.eventLoop()
	for {
		tcpConn, err := listner.AcceptTCP()
		if err != nil {
			glog.Errorln(err)
			continue
		}
		glog.Infoln("A new tcp connection open.", tcpConn.RemoteAddr())
		peer := s.NewPeer(tcpConn)
		go peer.serve()
	}
}

func (a *App) NewPeer(conn *net.TCPConn) *AppPeer {
	return &AppPeer{
		app:        a,
		conn:       conn,
		chWrite:    make(chan bool, 1),
		chDispatch: make(chan bool, 1),
		outbuf:     make([]byte, 0, 1024),
		inbuf:      make([]byte, 0, 1024),
	}
}

func (a *App) FindPeer(userID string) (*AppPeer, bool) {
	p, ok := a.users[userID]
	return p, ok
}

func (a *App) Locked(f func(*App)) {
	c := make(chan interface{})
	a.chEvent <- eventFunc{
		f: f,
		c: c,
	}
	<-c
}

func (a *App) Quit() {
	a.Locked(func(app *App) {
		for _, p := range app.users {
			SendServerShutDown(p)
		}
	})
	time.Sleep(1000 * time.Millisecond)
	close(a.chQuit)
}

func stripHost(addr string) string {
	glog.Info(addr)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		glog.Fatal("err in splitPort", err)
	}
	return ":" + fmt.Sprint(port)
}

func (a *App) eventLoop() {
	aliveCheck := time.Tick(10 * time.Second)
	peers := map[string]*AppPeer{}

	for {
		select {
		case <-a.chQuit:
			return
		case e := <-a.chEvent:
			switch args := e.(type) {
			case eventPeerCome:
				glog.Infoln("eventPeerCome")
				args.peer.lastRecvTime = time.Now()
				peers[args.peer.Address()] = args.peer
				StartLoginFlow(args.peer)
			case eventPeerMessage:
				args.peer.lastRecvTime = time.Now()
				if f, ok := a.handlers[args.msg.Command]; ok {
					f(args.peer, args.msg)
				} else {
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("Handler not found: 0x%04x %v msg:%v", uint16(args.msg.Command), args.msg.Command, args.msg)
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					if args.msg.Category == CategoryQuestion {
						args.peer.SendMessage(NewServerAnswer(args.msg))
					}
				}
			case eventPeerLeave:
				glog.Infoln("eventPeerLeave")
				delete(a.users, args.peer.UserID)
				delete(peers, args.peer.Address())
			case eventFunc:
				args.f(a)
				args.c <- struct{}{}
			}
		case <-aliveCheck:
			for _, p := range peers {
				if time.Since(p.lastRecvTime).Minutes() >= 2.0 {
					glog.Infoln("Recv Timeout", p)
					p.conn.Close()
				} else {
					RequestLineCheck(p)
				}
			}

			for sid, battle := range a.battles {
				if time.Since(battle.StartTime).Hours() >= 1.0 {
					delete(a.battles, sid)
					glog.Infoln("Battle user timeout.", sid, battle)
				}
			}
		}
	}
}

func (a *App) BroadcastLobbyUserCount(lobbyID uint16) {
	lobby, ok := a.lobbys[lobbyID]
	if ok {
		plaza := uint16(len(lobby.Users))
		msg := NewServerNotice(lbsPlazaJoin).Writer().Write16(lobbyID).Write16(plaza).Msg()
		for _, u := range a.users {
			u.SendMessage(msg)
		}

		renpo, zeon := lobby.GetUserCountBySide()
		msgSum1 := NewServerNotice(lbsLobbyJoin).Writer().Write16(EntryRenpo).Write16(renpo + zeon).Msg()
		msgSum2 := NewServerNotice(lbsLobbyJoin).Writer().Write16(EntryZeon).Write16(renpo + zeon).Msg()
		msgRenpo := NewServerNotice(lbsLobbyJoin).Writer().Write16(EntryRenpo).Write16(renpo).Msg()
		msgZeon := NewServerNotice(lbsLobbyJoin).Writer().Write16(EntryZeon).Write16(zeon).Msg()
		for userID := range lobby.Users {
			p, ok := a.FindPeer(userID)
			if ok {
				if p.inLobbyChat {
					p.SendMessage(msgSum1)
					p.SendMessage(msgSum2)
				} else {
					p.SendMessage(msgRenpo)
					p.SendMessage(msgZeon)
				}
			}
		}
	}
}

func (a *App) BroadcastLobbyMatchEntryUserCount(lobbyID uint16) {
	lobby, ok := a.lobbys[lobbyID]
	if ok {
		renpo, zeon := lobby.GetLobbyMatchEntryUserCount()
		msg1 := NewServerNotice(lbsLobbyMatchingJoin).Writer().Write16(EntryRenpo).Write16(renpo).Msg()
		msg2 := NewServerNotice(lbsLobbyMatchingJoin).Writer().Write16(EntryZeon).Write16(zeon).Msg()
		for userID := range lobby.Users {
			if p, ok := a.FindPeer(userID); ok {
				p.SendMessage(msg1)
				p.SendMessage(msg2)
			}
		}
	}
}

func (a *App) BroadcastRoomState(room *Room) {
	if room == nil {
		return
	}

	lobby, ok := a.lobbys[room.LobbyID]
	if !ok {
		return
	}

	msg1 := NewServerNotice(lbsRoomStatus).Writer().Write16(room.ID).Write8(room.Status).Msg()
	msg2 := NewServerNotice(lbsRoomTitle).Writer().Write16(room.ID).WriteString(room.Name).Msg()
	for userID := range lobby.Users {
		if p, ok := a.FindPeer(userID); ok {
			p.SendMessage(msg1)
			p.SendMessage(msg2)
		}
	}
}

func (a *App) OnGetBattleResult(p *AppPeer, result *BattleResult) {
	js, err := json.Marshal(result)
	if err != nil {
		glog.Errorln("Failed to marshal battle result", err)
		glog.Infoln(result)
		return
	}

	record, err := getDB().GetBattleRecordUser(result.BattleCode, p.UserID)
	if err != nil {
		glog.Errorln("Failed to load battle record", err)
		glog.Infoln(string(js))
		return
	}

	record.Round = int(result.BattleCount)
	record.Win = int(result.WinCount)
	record.Lose = int(result.LoseCount)
	record.Kill = int(result.KillCount)
	record.Death = 0 // missing in gdxsv
	record.Frame = 0 // missing in gdxsv
	record.Result = string(js)

	err = getDB().UpdateBattleRecord(record)
	if err != nil {
		glog.Errorln("Failed to save battle record", err)
		glog.Infoln(record)
		return
	}

	glog.Infoln("before", p.DBUser)
	rec, err := getDB().CalculateUserTotalBattleCount(p.UserID, 0)
	if err != nil {
		glog.Errorln("Failed to calculate battle count", err)
		return
	}

	p.DBUser.BattleCount = rec.Battle
	p.DBUser.WinCount = rec.Win
	p.DBUser.LoseCount = rec.Lose
	p.DBUser.KillCount = rec.Kill
	p.DBUser.DeathCount = rec.Death

	rec, err = getDB().CalculateUserTotalBattleCount(p.UserID, 1)
	if err != nil {
		glog.Errorln("Failed to calculate battle count", err)
		return
	}

	p.DBUser.RenpoBattleCount = rec.Battle
	p.DBUser.RenpoWinCount = rec.Win
	p.DBUser.RenpoLoseCount = rec.Lose
	p.DBUser.RenpoKillCount = rec.Kill
	p.DBUser.RenpoDeathCount = rec.Death

	rec, err = getDB().CalculateUserTotalBattleCount(p.UserID, 2)
	if err != nil {
		glog.Errorln("Failed to calculate battle count", err)
		return
	}

	p.DBUser.ZeonBattleCount = rec.Battle
	p.DBUser.ZeonWinCount = rec.Win
	p.DBUser.ZeonLoseCount = rec.Lose
	p.DBUser.ZeonKillCount = rec.Kill
	p.DBUser.ZeonDeathCount = rec.Death

	rec, err = getDB().CalculateUserDailyBattleCount(p.UserID)
	if err != nil {
		glog.Errorln("Failed to calculate battle count", err)
		return
	}

	p.DBUser.DailyBattleCount = rec.Battle
	p.DBUser.DailyWinCount = rec.Win
	p.DBUser.DailyLoseCount = rec.Lose

	err = getDB().UpdateUser(&p.DBUser)
	if err != nil {
		glog.Errorln(err)
		return
	}
	glog.Infoln("after", p.DBUser)
}

type RankingEntry struct {
	Rank        uint32
	EntireCount uint32
	Class       byte
	Battle      uint32
	Win         uint32
	Lose        uint32
	Invalid     uint32
	Kill        uint32
}

type AppPeer struct {
	DBUser

	conn   *net.TCPConn
	app    *App
	Room   *Room
	Lobby  *Lobby
	Battle *Battle

	Entry     uint16
	GameParam []byte
	PilotName string

	inLobbyChat       bool
	inBattleAfterRoom bool

	lastRecvTime time.Time

	chWrite    chan bool
	chDispatch chan bool
	chQuit     chan interface{}

	mOutbuf sync.Mutex
	outbuf  []byte

	mInbuf sync.Mutex
	inbuf  []byte
}

func (c *AppPeer) serve() {
	defer c.conn.Close()
	defer func() {
		c.app.chEvent <- eventPeerLeave{c}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	go c.dispatchLoop(ctx, cancel)
	go c.writeLoop(ctx, cancel)
	go c.readLoop(ctx, cancel)

	c.app.chEvent <- eventPeerCome{c}
	<-ctx.Done()
}

func (c *AppPeer) SendMessage(msg *Message) {
	glog.V(2).Infof("\t->%v %v \n", c.Address(), msg)
	c.mOutbuf.Lock()
	c.outbuf = append(c.outbuf, msg.Serialize()...)
	c.mOutbuf.Unlock()
	select {
	case c.chWrite <- true:
	default:
	}
}

func (c *AppPeer) Address() string {
	return c.conn.RemoteAddr().String()
}

func (c *AppPeer) readLoop(ctx context.Context, cancel func()) {
	defer cancel()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(time.Minute * 30))
		n, err := c.conn.Read(buf)
		if err != nil {
			glog.Infoln("TCP conn error:", err)
			return
		}
		if n == 0 {
			glog.Infoln("TCP read zero")
			return
		}
		c.mInbuf.Lock()
		c.inbuf = append(c.inbuf, buf[:n]...)
		c.mInbuf.Unlock()

		select {
		case c.chDispatch <- true:
		default:
		}
	}
}

func (c *AppPeer) writeLoop(ctx context.Context, cancel func()) {
	defer cancel()

	buf := make([]byte, 0, 128)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.chWrite:
			c.mOutbuf.Lock()
			if len(c.outbuf) == 0 {
				c.mOutbuf.Unlock()
				continue
			}
			buf = append(buf, c.outbuf...)
			c.outbuf = c.outbuf[:0]
			c.mOutbuf.Unlock()

			sum := 0
			size := len(buf)
			for sum < size {
				c.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
				n, err := c.conn.Write(buf[sum:])
				if err != nil {
					glog.Errorf("%v write error: %v\n", c.Address(), err)
					break
				}
				sum += n
			}
			buf = buf[:0]
		}
	}
}

func (c *AppPeer) dispatchLoop(ctx context.Context, cancel func()) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.chDispatch:
			c.mInbuf.Lock()
			for len(c.inbuf) >= HeaderSize {
				n, msg := Deserialize(c.inbuf)
				if n == 0 {
					// not enough data comming
					break
				}

				c.inbuf = c.inbuf[n:]
				if msg != nil {
					glog.V(2).Infof("%v %v\n", c.Address(), msg)
					c.app.chEvent <- eventPeerMessage{peer: c, msg: msg}
				}
			}
			c.mInbuf.Unlock()
		}
	}
}
