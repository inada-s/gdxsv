package lobby

import (
	"fmt"
	"net"
	"net/rpc"
	"time"

	"gdxsv/pkg/lobby/message"
	"gdxsv/pkg/lobby/model"

	"github.com/golang/glog"
)

type MessageHandler func(*AppPeer, *message.Message)

type handlerHolder struct {
	handlers     map[uint16]MessageHandler
	handlerNames map[uint16]string
}

var defaultHandlers = &handlerHolder{
	handlers:     make(map[uint16]MessageHandler),
	handlerNames: make(map[uint16]string),
}

func register(id uint16, name string, f MessageHandler) interface{} {
	defaultHandlers.handlers[id] = f
	defaultHandlers.handlerNames[id] = name
	return nil
}

type eventPeerCome struct {
	peer *AppPeer
}

type eventPeerLeave struct {
	peer *AppPeer
}

type eventPeerMessage struct {
	peer *AppPeer
	msg  *message.Message
}

type eventFunc struct {
	f func(*App)
	c chan<- interface{}
}

type AppPeer struct {
	model.User
	conn              *Conn
	app               *App
	inBattleAfterRoom bool

	proxyIP           net.IP
	proxyPort         uint16
	proxyUDPAddrs     []string
	proxyP2PConnected map[string]struct{}

	proxyRegTime time.Time
	proxyUseTime time.Time

	lastRecvTime time.Time
}

func (p *AppPeer) OnOpen() {
	p.app.chEvent <- eventPeerCome{peer: p}
}

func (p *AppPeer) OnMessage(msg *message.Message) {
	p.app.chEvent <- eventPeerMessage{peer: p, msg: msg}
}

func (p *AppPeer) OnClose() {
	p.app.chEvent <- eventPeerLeave{peer: p}
}

func (p *AppPeer) SendMessage(msg *message.Message) {
	p.conn.SendMessage(msg)
}

type App struct {
	*handlerHolder
	battleServer *rpc.Client
	users        map[string]*AppPeer
	lobbys       map[uint16]*model.Lobby
	battles      map[string]*model.Battle
	chEvent      chan interface{}
	chQuit       chan interface{}
}

func NewApp() *App {
	app := &App{
		handlerHolder: defaultHandlers,
		users:         make(map[string]*AppPeer),
		lobbys:        make(map[uint16]*model.Lobby),
		battles:       make(map[string]*model.Battle),
		chEvent:       make(chan interface{}, 64),
		chQuit:        make(chan interface{}),
	}
	for i := 0; i < 26; i++ {
		app.lobbys[uint16(i)] = model.NewLobby(uint16(i))
	}
	return app
}

func (a *App) NewPeer(conn *Conn) Peer {
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
		/*
			for _, p := range app.users {
				SendServerShutDown(p)
			}
		*/
	})
	time.Sleep(10 * time.Millisecond)
	close(a.chQuit)
}

func (a *App) AddHandler(id uint16, name string, f MessageHandler) {
	a.handlers[id] = f
	a.handlerNames[id] = name
}

func (a *App) connectToBattleServer() {
	glog.Infoln("TODO: connectToBattleServer")
	/*
		client, err := rpc.DialHTTP("tcp", config.Conf.Battle.RPCAddr)
		if err != nil {
			glog.Errorln("Faild to connect BattleServer. ", err)
		} else {
			glog.Infoln("Succeeded to connect BattleServer")
			a.battleServer = client
		}
	*/
}

func stripHost(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		glog.Fatal("err in splitPort", err)
	}
	return ":" + fmt.Sprint(port)
}

func (a *App) Serve() {
	aliveCheck := time.Tick(10 * time.Second)
	a.connectToBattleServer()
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
				a.OnOpen(args.peer)
			case eventPeerMessage:
				args.peer.lastRecvTime = time.Now()
				if f, ok := a.handlers[args.msg.Command]; ok {
					f(args.peer, args.msg)
				} else {
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("Handler not found: name = %v msg = %v", symbolMap[args.msg.Command], args.msg)
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					glog.Errorf("======================================")
					if args.msg.Category == message.CategoryQuestion {
						args.peer.SendMessage(message.NewServerAnswer(args.msg))
					}
				}
			case eventPeerLeave:
				glog.Infoln("eventPeerLeave")
				a.OnClose(args.peer)
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
					// RequestLineCheck(p)
				}
			}

			for sid, battle := range a.battles {
				if time.Since(battle.StartTime).Hours() >= 1.0 {
					delete(a.battles, sid)
					glog.Infoln("Battle user timeout.", sid, battle)
				}
			}

			if a.battleServer == nil {
				a.connectToBattleServer()
			} else {
				var reply string
				err := a.battleServer.Call("Logic.PingPong", "Ping", &reply)
				if err != nil {
					glog.Errorln("PingPong failed. ", err)
					a.battleServer.Close()
					a.battleServer = nil
					a.connectToBattleServer()
				}
			}
		}
	}
}
