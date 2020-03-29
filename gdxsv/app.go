package main

import (
	"fmt"
	"net"
	"net/rpc"
	"time"

	"github.com/golang/glog"
)

const (
	maxLobbyCount = 3
	maxRoomCount  = 3
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

type AppPeer struct {
	DBUser

	conn   *GameConn
	app    *App
	Room   *Room
	Lobby  *Lobby
	Battle *Battle

	Entry uint16

	inLobbyChat       bool
	inBattleAfterRoom bool

	lastRecvTime time.Time
}

func (p *AppPeer) OnOpen() {
	p.app.chEvent <- eventPeerCome{peer: p}
}

func (p *AppPeer) OnMessage(msg *Message) {
	p.app.chEvent <- eventPeerMessage{peer: p, msg: msg}
}

func (p *AppPeer) OnClose() {
	p.app.chEvent <- eventPeerLeave{peer: p}
}

func (p *AppPeer) SendMessage(msg *Message) {
	p.conn.SendMessage(msg)
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
		app.lobbys[uint16(i)] = NewLobby(uint16(i))
	}
	return app
}

func (a *App) NewPeer(conn *GameConn) *AppPeer {
	return &AppPeer{
		conn: conn,
		app:  a,
	}
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

func (a *App) Serve() {
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
				peers[args.peer.conn.Address()] = args.peer
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
				delete(peers, args.peer.conn.Address())
			case eventFunc:
				args.f(a)
				args.c <- struct{}{}
			}
		case <-aliveCheck:
			for _, p := range peers {
				if time.Since(p.lastRecvTime).Minutes() >= 2.0 {
					glog.Infoln("Recv Timeout", p)
					p.conn.conn.Close()
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
		for _, u := range lobby.Users {
			if u.inLobbyChat {
				u.SendMessage(msgSum1)
				u.SendMessage(msgSum2)
			} else {
				u.SendMessage(msgRenpo)
				u.SendMessage(msgZeon)
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
		for _, u := range lobby.Users {
			u.SendMessage(msg1)
			u.SendMessage(msg2)
		}
	}
}
