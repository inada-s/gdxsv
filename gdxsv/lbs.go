package main

import (
	"context"
	"database/sql"
	"fmt"
	"gdxsv/gdxsv/proto"
	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	maxLobbyCount = 22
	maxRoomCount  = 5

	PlatformConsole  = "console"    // Real PS2 / Dreamcast
	PlatformEmuX8664 = "emu-x86/64" // PCSX2 / Flycast on x64 platform

	GameDiskDC1 = "dc1" // Dreamcast
	GameDiskDC2 = "dc2" // Dreamcast DX
	GameDiskPS2 = "ps2" // PS2 DX

	McsAddrP2PGame = "255.255.255.255:255"
)

func lobbyKey(platform string, disk string) string {
	return fmt.Sprintf("%s|%s", platform, disk)
}

const (
	TeamNone  = 0
	TeamRenpo = 1
	TeamZeon  = 2
)

type Lbs struct {
	handlers  map[CmdID]LbsHandler
	userPeers map[string]*LbsPeer
	mcsPeers  map[string]*LbsPeer
	lobbies   map[string]map[uint16]*LbsLobby
	chEvent   chan interface{}
	chQuit    chan interface{}

	noBan      bool
	noTempBan  bool
	reload     bool
	banChecked map[string]bool
	bannedIPs  map[string]time.Time
}

func NewLbs() *Lbs {
	app := &Lbs{
		handlers:  defaultLbsHandlers,
		userPeers: make(map[string]*LbsPeer),
		mcsPeers:  make(map[string]*LbsPeer),
		lobbies:   make(map[string]map[uint16]*LbsLobby),
		chEvent:   make(chan interface{}, 64),
		chQuit:    make(chan interface{}),

		banChecked: make(map[string]bool),
		bannedIPs:  make(map[string]time.Time),
	}

	for _, pf := range []string{PlatformConsole, PlatformEmuX8664} {
		for _, disk := range []string{GameDiskDC1, GameDiskDC2, GameDiskPS2} {
			key := lobbyKey(pf, disk)
			app.lobbies[key] = make(map[uint16]*LbsLobby)

			for i := 1; i <= maxLobbyCount; i++ {
				app.lobbies[key][uint16(i)] = NewLobby(app, pf, disk, uint16(i))
			}
		}
	}

	return app
}

func (lbs *Lbs) NoBan() {
	lbs.noBan = true
}

func (lbs *Lbs) NoTempBan() {
	lbs.noTempBan = true
}

func (lbs *Lbs) IsBannedEndpoint(p *LbsPeer) bool {
	banned, err := getDB().IsBannedEndpoint(p.IP(), p.PlatformInfo["machine_id"])
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		logger.Warn("GetBan returned err", zap.Error(err))
		return false
	}
	if banned && lbs.noBan {
		logger.Warn("passed banned user", zap.String("ip", p.IP()), zap.String("machine_id", p.PlatformInfo["machine_id"]))
		return false
	}
	return banned
}

func (lbs *Lbs) IsBannedAccount(loginKey string) bool {
	banned, err := getDB().IsBannedAccount(loginKey)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		logger.Warn("IsBannedAccount returned err", zap.Error(err))
		return false
	}
	if banned && lbs.noBan {
		logger.Warn("passed banned user", zap.String("login_key", loginKey))
		return false
	}
	return banned
}

func (lbs *Lbs) IsTempBan(p *LbsPeer) bool {
	if t, ok := p.app.bannedIPs[p.IP()]; ok && time.Since(t).Minutes() <= 10 {
		if lbs.noTempBan {
			logger.Warn("passed temp banned user", zap.String("user_id", p.UserID), zap.String("name", p.Name))
			return false
		}
		return true
	}
	return false
}

func (lbs *Lbs) TempBan(userID string) {
	user, err := getDB().GetUser(userID)
	if err != nil {
		logger.Warn("failed to get banned user", zap.String("user_id", userID), zap.Error(err))
		return
	}

	account, err := getDB().GetAccountByLoginKey(user.LoginKey)
	if err != nil {
		logger.Warn("failed to get banned user account", zap.String("user_id", userID), zap.Error(err))
		return
	}

	if account.LastLoginIP == "" {
		logger.Warn("last login ip is empty", zap.String("user_id", userID))
		return
	}

	logger.Info("temporary ip banned",
		zap.String("ip_addr", account.LastLoginIP),
		zap.String("user_id", userID),
		zap.String("name", user.Name))

	lbs.bannedIPs[account.LastLoginIP] = time.Now()

	for _, p := range lbs.userPeers {
		if p.IP() == account.LastLoginIP {
			p.SendMessage(NewServerNotice(lbsShutDown).Writer().
				WriteString("<LF=5><BODY><CENTER>TEMPORARY BANNED<END>").Msg())
		}
	}
}

func (lbs *Lbs) GetLobby(platform, disk string, lobbyID uint16) *LbsLobby {
	lobbies, ok := lbs.lobbies[lobbyKey(platform, disk)]
	if !ok {
		return nil
	}

	lobby, ok := lobbies[lobbyID]
	if !ok {
		return nil
	}

	return lobby
}

func (lbs *Lbs) ListenAndServe(addr string) {
	logger.Info("lbs.ListenAndServe", zap.String("addr", addr))

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		logger.Fatal("net.ResolveTCPAddr", zap.Error(err))
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		logger.Fatal("net.ListenTCP", zap.Error(err))
	}

	go lbs.serveUDP(addr)
	go lbs.eventLoop()

	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			logger.Error("failed to accept", zap.Error(err))
			continue
		}
		logger.Info("a new connection open", zap.String("addr", tcpConn.RemoteAddr().String()))
		peer := lbs.NewPeer(tcpConn)
		go peer.serve()
	}
}

func (lbs *Lbs) serveUDP(addr string) {
	logger.Info("lbs.ServeUDP", zap.String("addr", addr))

	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		logger.Fatal("net.ResolveUDPAddr", zap.Error(err))
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		logger.Fatal("net.ListenUDP", zap.Error(err))
	}

	buf := make([]byte, 128)
	pkt := new(proto.Packet)
	for {
		n, remoteAddr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			logger.Error("udpConn.ReadFromUDP", zap.Error(err))
			if err == net.ErrClosed {
				go lbs.serveUDP(addr)
				return
			}
		}

		pkt.Reset()
		err = pb.Unmarshal(buf[:n], pkt)
		if err != nil {
			logger.Error("pb.Unmarshal", zap.Error(err))
			continue
		}

		if pkt.HelloLbsData != nil {
			lbs.Locked(func(lbs *Lbs) {
				peer := lbs.FindPeer(pkt.HelloLbsData.UserId)
				if peer != nil {
					peer.udpAddr = *remoteAddr
					logger.Info("set peer.udpAddr",
						zap.String("user_id", peer.UserID), zap.String("addr", peer.udpAddr.String()))
				}
			})
		}
	}
}

func (lbs *Lbs) NewPeer(conn net.Conn) *LbsPeer {
	return &LbsPeer{
		app:  lbs,
		conn: conn,

		// Since it is not possible to distinguish between
		// the emulator and the real console at this point,
		// it is treated as an real console.
		Platform:     PlatformConsole,
		PlatformInfo: map[string]string{},
		chWrite:      make(chan bool, 1),
		chDispatch:   make(chan bool, 1),
		outbuf:       make([]byte, 0, 1024),
		inbuf:        make([]byte, 0, 1024),
		logger:       logger.With(zap.String("addr", conn.RemoteAddr().String())),
	}
}

func (lbs *Lbs) FindMcs(region string) *McsStatus {
	for _, p := range lbs.mcsPeers {
		if p.mcsStatus != nil {
			if strings.HasPrefix(p.mcsStatus.Region, region) &&
				p.mcsStatus.PublicAddr != "" {
				return p.mcsStatus
			}
		}
	}
	return nil
}

func (lbs *Lbs) FindPeer(userID string) *LbsPeer {
	p, ok := lbs.userPeers[userID]
	if !ok {
		return nil
	}
	return p
}

func (lbs *Lbs) FindMcsPeer(mcsAddr string) *LbsPeer {
	p, ok := lbs.mcsPeers[mcsAddr]
	if !ok {
		return nil
	}
	return p
}

func (lbs *Lbs) Locked(f func(*Lbs)) {
	c := make(chan interface{})
	lbs.chEvent <- eventFunc{f: f, c: c}
	<-c
}

func (lbs *Lbs) Quit() {
	lbs.Locked(func(app *Lbs) {
		for _, p := range app.userPeers {
			SendServerShutDown(p)
		}
	})
	time.Sleep(10 * time.Millisecond)
	close(lbs.chQuit)
}

func stripHost(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		logger.DPanic("failed to split host port", zap.Error(err))
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

func (lbs *Lbs) cleanPeer(p *LbsPeer) {
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
		if p.logout && p.Battle != nil {
			mcsAddr := net.JoinHostPort(p.Battle.ServerIP.String(), fmt.Sprint(p.Battle.ServerPort))
			if mcsAddr == McsAddrP2PGame {
				if !isOldFlycastVersion(p.PlatformInfo["flycast"], "v1.2.0") {
					sharedData.UpdateMcsGameState(p.Battle.BattleCode, McsGameStateClosed)
					sharedData.UpdateMcsUserState(p.Battle.BattleCode, McsUserStateLeft)
				}
			}
		}
		if p.Battle != nil {
			p.Battle = nil
		}
		delete(lbs.userPeers, p.UserID)
	}

	if p.mcsStatus != nil {
		if len(p.mcsStatus.Games) != 0 || len(p.mcsStatus.Users) != 0 {
			logger.Warn("mcs closed during game",
				zap.Any("games", p.mcsStatus.Games), zap.Any("users", p.mcsStatus.Users))
			for _, g := range p.mcsStatus.Games {
				sharedData.UpdateMcsGameState(g.BattleCode, McsGameStateClosed)
			}
		}
		delete(p.app.mcsPeers, p.mcsStatus.PublicAddr)
		p.mcsStatus = nil
	}

	p.conn.Close()
	p.cleaned = true
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
				args.peer.logger.Info("eventPeerCome")
				args.peer.lastRecvTime = time.Now()
				peers[args.peer.Address()] = args.peer
				StartLoginFlow(args.peer)
				lbs.BroadcastBattleUserCount()
			case eventPeerMessage:
				if ce := args.peer.logger.Check(zap.DebugLevel, ""); ce != nil {
					fmt.Println(args.msg)
				}
				if args.peer.cleaned {
					args.peer.logger.Warn("got message after cleaned", zap.Any("msg", args.msg))
					continue
				}

				args.peer.lastRecvTime = time.Now()
				if f, ok := lbs.handlers[args.msg.Command]; ok {
					f(args.peer, args.msg)
				} else {
					logger.Warn("handler not found",
						zap.String("cmd", args.msg.Command.String()),
						zap.String("cmd_id", fmt.Sprintf("0x%04x", uint16(args.msg.Command))),
						zap.String("msg", fmt.Sprint(args.msg)),
						zap.Binary("body", args.msg.Body),
					)
					if args.msg.Category == CategoryQuestion {
						args.peer.SendMessage(NewServerAnswer(args.msg))
					}
				}
			case eventPeerLeave:
				args.peer.logger.Info("eventPeerLeave")
				lbs.cleanPeer(args.peer)
				delete(peers, args.peer.Address())
				lbs.BroadcastBattleUserCount()
			case eventFunc:
				func() {
					defer func() {
						args.c <- nil
					}()
					args.f(lbs)
				}()
			}
		case <-tick:
			for _, p := range peers {
				lastRecvSince := time.Since(p.lastRecvTime)
				if 1 <= lastRecvSince.Minutes() {
					logger.Info("kick peer", zap.String("addr", p.Address()))
					lbs.cleanPeer(p)
					delete(peers, p.Address())
				} else if 10 <= lastRecvSince.Seconds() {
					RequestLineCheck(p)
				}
			}

			// temp ban check
			for _, g := range sharedData.GetMcsGames() {
				if g.State != McsGameStateClosed {
					continue
				}

				if lbs.banChecked[g.BattleCode] {
					continue
				}

				lbs.banChecked[g.BattleCode] = true

				if g.McsAddr == McsAddrP2PGame {
					// temp ban is currently disabled on p2p game
					logger.Info("p2p game ignored")
					continue
				}

				var mcsUsers []*McsUser
				stateCount := map[int]int{}
				for _, u := range sharedData.GetMcsUsers() {
					if g.BattleCode == u.BattleCode {
						mcsUsers = append(mcsUsers, u)
						stateCount[u.State]++
					}
				}

				if len(mcsUsers) < 4 {
					continue
				}

				// All players joined the game except for one player.
				if stateCount[McsUserStateCreated] == 1 {
					for _, u := range mcsUsers {
						if u.State == McsUserStateCreated {
							lbs.TempBan(u.UserID)
						}
					}
				}

				// All players joined the game, player disconnected during the game.
				if stateCount[McsUserStateCreated] == 0 {
					for _, u := range mcsUsers {
						switch u.CloseReason {
						case "cl_hard_reset", "cl_soft_reset", "cl_hard_quit":
							lbs.TempBan(u.UserID)
						}
					}
				}
			}

			sharedData.RemoveStaleData()

			reload := lbs.reload
			lbs.reload = false
			for _, pfLobbies := range lbs.lobbies {
				for _, lobby := range pfLobbies {
					if reload {
						err := lobby.LoadLobbySetting()
						if err != nil {
							logger.Error("LoadLobbySetting failed", zap.Error(err))
						}
					}
					lobby.Update()
				}
			}
		}
	}
}

func (lbs *Lbs) BroadcastLobbyUserCount(lobby *LbsLobby) {
	if lobby == nil {
		return
	}

	// To lobby select scene.
	if lobby.GameDisk == GameDiskPS2 {
		ps2msg := NewServerNotice(lbsPlazaJoin).Writer().
			Write16(lobby.ID).Write16(uint16(len(lobby.Users))).Msg()
		for _, u := range lbs.userPeers {
			if u.Platform == lobby.Platform && u.IsPS2() {
				u.SendMessage(ps2msg)
			}
		}
	} else if lobby.GameDisk == GameDiskDC1 || lobby.GameDisk == GameDiskDC2 {
		lobby1 := lbs.GetLobby(lobby.Platform, GameDiskDC1, lobby.ID)
		lobby2 := lbs.GetLobby(lobby.Platform, GameDiskDC2, lobby.ID)
		if lobby1 == nil || lobby2 == nil {
			return
		}
		dcmsg := NewServerNotice(lbsPlazaJoin).Writer().
			Write16(lobby.ID).
			Write16(uint16(len(lobby1.Users))).
			Write16(uint16(len(lobby2.Users))).Msg()
		for _, u := range lbs.userPeers {
			if u.Platform == lobby.Platform && u.IsDC() {
				u.SendMessage(dcmsg)
			}
		}
	}

	// To lobby scene.
	if lobby.GameDisk == GameDiskPS2 {
		renpo, zeon := lobby.GetUserCountByTeam()
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
	} else if lobby.GameDisk == GameDiskDC1 || lobby.GameDisk == GameDiskDC2 {
		lobby1 := lbs.GetLobby(lobby.Platform, GameDiskDC1, lobby.ID)
		lobby2 := lbs.GetLobby(lobby.Platform, GameDiskDC2, lobby.ID)
		if lobby1 == nil || lobby2 == nil {
			return
		}

		renpo1, zeon1 := lobby1.GetUserCountByTeam()
		renpo2, zeon2 := lobby2.GetUserCountByTeam()
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

func (lbs *Lbs) BroadcastBattleUserCount() {
	for userID := range lbs.userPeers {
		var mcsUsersCount uint32 = uint32(len(sharedData.mcsUsers))
		if p := lbs.FindPeer(userID); p != nil {
			p.SendMessage(NewServerNotice(lbsBattleUserCount).Writer().Write32(mcsUsersCount).Msg())
		}
	}
}

func (lbs *Lbs) RegisterBattleResult(p *LbsPeer, result *BattleResult) {
	record, err := getDB().GetBattleRecordUser(result.BattleCode, p.UserID)
	if err != nil {
		logger.Warn("failed to load battle record",
			zap.Error(err),
			zap.String("battle_code", result.BattleCode),
			zap.Any("battle_result", result))
		return
	}

	if record.System != 0 {
		// already updated
		return
	}

	record.Round = int(result.BattleCount)
	record.Win = int(result.WinCount)
	record.Lose = int(result.LoseCount)
	record.Kill = int(result.KillCount)
	record.Death = 0 // missing in gdxsv
	record.Frame = 0 // missing in gdxsv
	record.System = 1

	err = getDB().UpdateBattleRecord(record)
	if err != nil {
		logger.Error("failed to save battle record",
			zap.Error(err),
			zap.String("battle_code", result.BattleCode),
			zap.Any("battle_result", result))
		return
	}

	logger.Info("update battle count",
		zap.String("user_id", p.UserID),
		zap.Any("before", p.DBUser))

	if record.Players == 4 {
		if record.Aggregate != 0 {
			p.DBUser.BattleCount += record.Round
			p.DBUser.WinCount += record.Win
			p.DBUser.LoseCount += record.Lose
			p.DBUser.KillCount += record.Kill
			p.DBUser.DeathCount += record.Death

			if record.Team == TeamRenpo {
				p.DBUser.RenpoBattleCount += record.Round
				p.DBUser.RenpoWinCount += record.Win
				p.DBUser.RenpoLoseCount += record.Lose
				p.DBUser.RenpoKillCount += record.Kill
				p.DBUser.RenpoDeathCount += record.Death
			}

			if record.Team == TeamZeon {
				p.DBUser.ZeonBattleCount += record.Round
				p.DBUser.ZeonWinCount += record.Win
				p.DBUser.ZeonLoseCount += record.Lose
				p.DBUser.ZeonKillCount += record.Kill
				p.DBUser.ZeonDeathCount += record.Death
			}
		}

		p.DBUser.DailyBattleCount += record.Round
		p.DBUser.DailyWinCount += record.Win
		p.DBUser.DailyLoseCount += record.Lose
	}

	err = getDB().UpdateUser(&p.DBUser)
	if err != nil {
		logger.Error("failed to update user", zap.Error(err))
		return
	}

	logger.Info("update battle count",
		zap.String("user_id", p.UserID),
		zap.Any("after", p.DBUser))
}

type LbsPeer struct {
	DBUser
	logger *zap.Logger

	conn    net.Conn
	udpAddr net.UDPAddr
	app     *Lbs
	Room    *LbsRoom
	Lobby   *LbsLobby
	Battle  *LbsBattle

	Platform     string
	GameDisk     string
	PlatformInfo map[string]string
	Team         uint16
	GameParam    []byte
	PilotName    string
	Rank         int

	bestRegion    string
	lastSessionID string
	lastRecvTime  time.Time
	logout        bool
	cleaned       bool

	chWrite    chan bool
	chDispatch chan bool

	mOutbuf sync.Mutex
	outbuf  []byte

	mInbuf sync.Mutex
	inbuf  []byte

	// used only mcs peer
	mcsStatus *McsStatus
}

func (p *LbsPeer) InLobbyChat() bool {
	return p.Lobby != nil && p.Room == nil && p.Team != TeamNone
}

func (p *LbsPeer) IsPS2() bool {
	return p.GameDisk == GameDiskPS2
}

func (p *LbsPeer) IsDC() bool {
	return p.GameDisk == GameDiskDC1 || p.GameDisk == GameDiskDC2
}

func (p *LbsPeer) IsDC1() bool {
	return p.GameDisk == GameDiskDC1
}

func (p *LbsPeer) IsDC2() bool {
	return p.GameDisk == GameDiskDC2
}

func (p *LbsPeer) serve() {
	defer p.conn.Close()
	defer func() {
		p.app.chEvent <- eventPeerLeave{p}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	go p.dispatchLoop(ctx, cancel)
	go p.writeLoop(ctx, cancel)
	go p.readLoop(ctx, cancel)

	p.app.chEvent <- eventPeerCome{p}
	<-ctx.Done()
}

func (p *LbsPeer) SendMessage(msg *LbsMessage) {
	if ce := p.logger.Check(zap.DebugLevel, ""); ce != nil {
		fmt.Println(msg)
	}

	p.mOutbuf.Lock()
	p.outbuf = append(p.outbuf, msg.Serialize()...)
	p.mOutbuf.Unlock()
	select {
	case p.chWrite <- true:
	default:
	}
}

func (p *LbsPeer) Address() string {
	return p.conn.RemoteAddr().String()
}

func (p *LbsPeer) IP() string {
	host, _, err := net.SplitHostPort(p.conn.RemoteAddr().String())
	if err != nil {
		return ""
	}
	return host
}

func (p *LbsPeer) readLoop(ctx context.Context, cancel func()) {
	defer cancel()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := p.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
		if err != nil {
			logger.Warn("SetReadDeadline failed", zap.Error(err))
		}

		n, err := p.conn.Read(buf)
		if err != nil {
			logger.Info("tcp read error", zap.Error(err))
			return
		}

		if n == 0 {
			if _, ok := p.conn.(*net.TCPConn); ok {
				logger.Info("tcp read zero")
				return
			}
			// FIXME: Pipe sock may read zero byte
		}

		p.mInbuf.Lock()
		p.inbuf = append(p.inbuf, buf[:n]...)
		p.mInbuf.Unlock()

		select {
		case p.chDispatch <- true:
		default:
		}
	}
}

func (p *LbsPeer) writeLoop(ctx context.Context, cancel func()) {
	defer cancel()

	buf := make([]byte, 0, 128)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.chWrite:
			p.mOutbuf.Lock()
			if len(p.outbuf) == 0 {
				p.mOutbuf.Unlock()
				continue
			}
			buf = append(buf, p.outbuf...)
			p.outbuf = p.outbuf[:0]
			p.mOutbuf.Unlock()

			sum := 0
			size := len(buf)
			for sum < size {
				err := p.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
				if err != nil {
					logger.Warn("SetReadDeadline failed", zap.Error(err))
				}

				n, err := p.conn.Write(buf[sum:])
				if err != nil {
					p.logger.Info("tcp write error", zap.Error(err))
					break
				}

				sum += n
			}
			buf = buf[:0]
		}
	}
}

func (p *LbsPeer) dispatchLoop(ctx context.Context, cancel func()) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.chDispatch:
			p.mInbuf.Lock()
			for len(p.inbuf) >= HeaderSize {
				n, msg := Deserialize(p.inbuf)
				if n == 0 {
					// not enough data coming
					break
				}

				p.inbuf = p.inbuf[n:]
				if msg != nil {
					p.app.chEvent <- eventPeerMessage{peer: p, msg: msg}
				}
			}
			p.mInbuf.Unlock()
		}
	}
}
