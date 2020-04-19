package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
)

const (
	maxLobbyCount = 22
	maxRoomCount  = 5
)

const (
	PlatformUnk = 0 // Unknown
	PlatformDC1 = 1 // Dreamcast
	PlatformDC2 = 2 // Dreamcast DX
	PlatformPS2 = 3 // PS2 DX
)

const (
	TeamNone  = 0
	TeamRenpo = 1
	TeamZeon  = 2
)

type Lbs struct {
	handlers map[CmdID]LbsHandler
	users    map[string]*LbsPeer
	lobbies  map[byte]map[uint16]*LbsLobby
	mcs      map[string]*McsStatus
	chEvent  chan interface{}
	chQuit   chan interface{}
}

func NewLbs() *Lbs {
	app := &Lbs{
		handlers: defaultLbsHandlers,
		users:    make(map[string]*LbsPeer),
		lobbies:  make(map[byte]map[uint16]*LbsLobby),
		mcs:      make(map[string]*McsStatus),
		chEvent:  make(chan interface{}, 64),
		chQuit:   make(chan interface{}),
	}

	app.lobbies[PlatformPS2] = make(map[uint16]*LbsLobby)
	app.lobbies[PlatformDC1] = make(map[uint16]*LbsLobby)
	app.lobbies[PlatformDC2] = make(map[uint16]*LbsLobby)

	for i := 1; i <= maxLobbyCount; i++ {
		app.lobbies[PlatformPS2][uint16(i)] = NewLobby(app, PlatformPS2, uint16(i))
	}
	for i := 1; i <= maxLobbyCount; i++ {
		app.lobbies[PlatformDC1][uint16(i)] = NewLobby(app, PlatformDC1, uint16(i))
	}
	for i := 1; i <= maxLobbyCount; i++ {
		app.lobbies[PlatformDC2][uint16(i)] = NewLobby(app, PlatformDC2, uint16(i))
	}

	return app
}

func (lbs *Lbs) GetLobby(platform uint8, lobbyID uint16) *LbsLobby {
	lobbies, ok := lbs.lobbies[platform]
	if !ok {
		return nil
	}

	lobby, ok := lobbies[lobbyID]
	if !ok {
		return nil
	}

	return lobby
}

func (s *Lbs) ListenAndServeLobby(addr string) error {
	glog.Info("ListenAndServeLobby", addr)

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

func (lbs *Lbs) NewPeer(conn *net.TCPConn) *LbsPeer {
	return &LbsPeer{
		app:        lbs,
		conn:       conn,
		chWrite:    make(chan bool, 1),
		chDispatch: make(chan bool, 1),
		outbuf:     make([]byte, 0, 1024),
		inbuf:      make([]byte, 0, 1024),
	}
}

func (lbs *Lbs) FindPeer(userID string) *LbsPeer {
	p, ok := lbs.users[userID]
	if !ok {
		return nil
	}
	return p
}

func (lbs *Lbs) Locked(f func(*Lbs)) {
	c := make(chan interface{})
	lbs.chEvent <- eventFunc{
		f: f,
		c: c,
	}
	<-c
}

func (lbs *Lbs) Quit() {
	lbs.Locked(func(app *Lbs) {
		for _, p := range app.users {
			SendServerShutDown(p)
		}
	})
	time.Sleep(1000 * time.Millisecond)
	close(lbs.chQuit)
}

func stripHost(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		glog.Fatal("err in splitPort", err)
	}
	return ":" + fmt.Sprint(port)
}

type eventPeerCome struct {
	peer *LbsPeer
}

type eventPeerLeave struct {
	peer *LbsPeer
}

type eventPeerMessage struct {
	peer *LbsPeer
	msg  *LbsMessage
}

type eventFunc struct {
	f func(*Lbs)
	c chan<- interface{}
}

func (lbs *Lbs) eventLoop() {
	tick := time.Tick(1 * time.Second)
	peers := map[string]*LbsPeer{}

	for {
		select {
		case <-lbs.chQuit:
			return
		case e := <-lbs.chEvent:
			switch args := e.(type) {
			case eventPeerCome:
				glog.Infoln("eventPeerCome")
				args.peer.lastRecvTime = time.Now()
				peers[args.peer.Address()] = args.peer
				StartLoginFlow(args.peer)
			case eventPeerMessage:
				args.peer.lastRecvTime = time.Now()
				if f, ok := lbs.handlers[args.msg.Command]; ok {
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
				delete(lbs.users, args.peer.UserID)
				delete(peers, args.peer.Address())
			case eventFunc:
				args.f(lbs)
				args.c <- struct{}{}
			}
		case <-tick:
			for _, p := range peers {
				if 1 <= time.Since(p.lastRecvTime).Minutes() {
					glog.Infoln("Kick", p.Address())
					if p.UserID != "" {
						if p.Room != nil {
							p.Room.Exit(p.UserID)
							lbs.BroadcastRoomState(p.Room)
							p.Room = nil
						}
						if p.Lobby != nil {
							p.Lobby.Exit(p.UserID)
							lbs.BroadcastLobbyUserCount(p.Lobby)
							lbs.BroadcastLobbyMatchEntryUserCount(p.Lobby)
							p.Lobby = nil
						}
					}
					delete(peers, p.Address())
					p.conn.Close()
				} else {
					RequestLineCheck(p)
				}
			}
			for _, pfLobbies := range lbs.lobbies {
				for _, lobby := range pfLobbies {
					lobby.CheckLobbyBattleStart()
					lobby.CheckRoomBattleStart()
				}
			}
		}
	}
}

func (lbs *Lbs) BroadcastLobbyUserCount(lobby *LbsLobby) {
	if lobby == nil {
		return
	}

	// For lobby select scene.
	msg := NewServerNotice(lbsPlazaJoin).Writer().
		Write16(lobby.ID).Write16(uint16(len(lobby.Users))).Msg()
	for _, u := range lbs.users {
		if u.Platform == lobby.Platform {
			u.SendMessage(msg)
		}
	}

	// For lobby chat scene.
	if lobby.Platform == PlatformPS2 {
		renpo, zeon := lobby.GetUserCountBySide()
		msgSum1 := NewServerNotice(lbsLobbyJoin).Writer().Write16(TeamRenpo).Write16(renpo + zeon).Msg()
		msgSum2 := NewServerNotice(lbsLobbyJoin).Writer().Write16(TeamZeon).Write16(renpo + zeon).Msg()
		msgRenpo := NewServerNotice(lbsLobbyJoin).Writer().Write16(TeamRenpo).Write16(renpo).Msg()
		msgZeon := NewServerNotice(lbsLobbyJoin).Writer().Write16(TeamZeon).Write16(zeon).Msg()
		for userID := range lobby.Users {
			if p := lbs.FindPeer(userID); p != nil {
				if p.InLobbyChat() {
					p.SendMessage(msgSum1)
					p.SendMessage(msgSum2)
				} else {
					p.SendMessage(msgRenpo)
					p.SendMessage(msgZeon)
				}
			}
		}
	} else if lobby.Platform == PlatformDC1 || lobby.Platform == PlatformDC2 {
		lobby1 := lbs.GetLobby(PlatformDC1, lobby.ID)
		lobby2 := lbs.GetLobby(PlatformDC2, lobby.ID)
		if lobby1 == nil || lobby2 == nil {
			return
		}

		renpo1, zeon1 := lobby1.GetUserCountBySide()
		renpo2, zeon2 := lobby2.GetUserCountBySide()
		msgSum1 := NewServerNotice(lbsLobbyJoin).Writer().
			Write16(TeamRenpo).
			Write16(renpo1 + zeon1).
			Write16(renpo2 + zeon2).Msg()
		msgSum2 := NewServerNotice(lbsLobbyJoin).Writer().
			Write16(TeamZeon).
			Write16(renpo1 + zeon1).
			Write16(renpo2 + zeon2).Msg()
		msgRenpo := NewServerNotice(lbsLobbyJoin).Writer().
			Write16(TeamRenpo).
			Write16(renpo1).
			Write16(renpo2).Msg()
		msgZeon := NewServerNotice(lbsLobbyJoin).Writer().
			Write16(TeamZeon).
			Write16(zeon1).
			Write16(zeon2).Msg()

		for userID := range lobby1.Users {
			if p := lbs.FindPeer(userID); p != nil {
				if p.InLobbyChat() {
					p.SendMessage(msgSum1)
					p.SendMessage(msgSum2)
				} else {
					p.SendMessage(msgRenpo)
					p.SendMessage(msgZeon)
				}
			}
		}

		for userID := range lobby2.Users {
			if p := lbs.FindPeer(userID); p != nil {
				if p.InLobbyChat() {
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

func (lbs *Lbs) BroadcastLobbyMatchEntryUserCount(lobby *LbsLobby) {
	renpo, zeon := lobby.GetLobbyMatchEntryUserCount()
	msg1 := NewServerNotice(lbsLobbyMatchingJoin).Writer().Write16(TeamRenpo).Write16(renpo).Msg()
	msg2 := NewServerNotice(lbsLobbyMatchingJoin).Writer().Write16(TeamZeon).Write16(zeon).Msg()
	for userID := range lobby.Users {
		if p := lbs.FindPeer(userID); p != nil {
			p.SendMessage(msg1)
			p.SendMessage(msg2)
		}
	}
}

func (lbs *Lbs) BroadcastRoomState(room *LbsRoom) {
	if room == nil || room.lobby == nil {
		return
	}
	msg1 := NewServerNotice(lbsRoomStatus).Writer().Write16(room.ID).Write8(room.Status).Msg()
	msg2 := NewServerNotice(lbsRoomTitle).Writer().Write16(room.ID).WriteString(room.Name).Msg()
	for userID := range room.lobby.Users {
		if p := lbs.FindPeer(userID); p != nil {
			if p.Team == room.Team {
				p.SendMessage(msg1)
				p.SendMessage(msg2)
			}
		}
	}
}

func (lbs *Lbs) RegisterBattleResult(p *LbsPeer, result *BattleResult) {
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

type LbsPeer struct {
	DBUser

	conn   *net.TCPConn
	app    *Lbs
	Room   *LbsRoom
	Lobby  *LbsLobby
	Battle *LbsBattle

	Platform  byte
	Team      uint16
	GameParam []byte
	PilotName string
	Rank      int

	lastConnectionID string
	lastRecvTime     time.Time

	chWrite    chan bool
	chDispatch chan bool
	chQuit     chan interface{}

	mOutbuf sync.Mutex
	outbuf  []byte

	mInbuf sync.Mutex
	inbuf  []byte
}

func (p *LbsPeer) InLobbyChat() bool {
	return p.Lobby != nil && p.Room == nil && p.Team != TeamNone
}

func (p *LbsPeer) IsPS2() bool {
	return p.Platform == PlatformPS2
}

func (p *LbsPeer) IsDC() bool {
	return p.Platform == PlatformDC1 || p.Platform == PlatformDC2
}

func (p *LbsPeer) IsDC1() bool {
	return p.Platform == PlatformDC1
}

func (p *LbsPeer) IsDC2() bool {
	return p.Platform == PlatformDC2
}

func (c *LbsPeer) serve() {
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

func (c *LbsPeer) SendMessage(msg *LbsMessage) {
	glog.V(2).Infof("\t->%v %v \n", c.Address(), msg)
	c.mOutbuf.Lock()
	c.outbuf = append(c.outbuf, msg.Serialize()...)
	c.mOutbuf.Unlock()
	select {
	case c.chWrite <- true:
	default:
	}
}

func (c *LbsPeer) Address() string {
	return c.conn.RemoteAddr().String()
}

func (c *LbsPeer) readLoop(ctx context.Context, cancel func()) {
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

func (c *LbsPeer) writeLoop(ctx context.Context, cancel func()) {
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

func (c *LbsPeer) dispatchLoop(ctx context.Context, cancel func()) {
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
