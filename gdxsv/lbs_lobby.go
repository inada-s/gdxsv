package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type LbsLobby struct {
	app *Lbs

	LobbySetting
	lobbySettingMessages []*LbsMessage
	forceStartCountDown  int

	GameDisk   uint8
	ID         uint16
	Rule       *Rule
	Users      map[string]*DBUser
	RenpoRooms map[uint16]*LbsRoom
	ZeonRooms  map[uint16]*LbsRoom
	EntryUsers []string
}

func NewLobby(app *Lbs, platform uint8, lobbyID uint16) *LbsLobby {
	lobby := &LbsLobby{
		app:                  app,
		LobbySetting:         *lbsLobbySettings[lobbyID],
		lobbySettingMessages: nil,
		forceStartCountDown:  0,

		GameDisk:   platform,
		ID:         lobbyID,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
	}

	if lobby.FreeRule {
		lobby.Rule = RulePresetFree.Clone()
	} else {
		lobby.Rule = RulePresetDefault.Clone()
	}

	if lobby.LobbySetting.No375MS {
		lobby.Rule.RenpoMaskDC = MSMaskAll & ^MSMaskDCGundam & ^MSMaskDCGelgoogS & ^MSMaskDCZeong & ^MSMaskDCElmeth
		lobby.Rule.ZeonMaskDC = MSMaskAll & ^MSMaskDCGundam & ^MSMaskDCGelgoogS & ^MSMaskDCZeong & ^MSMaskDCElmeth
	}

	if lobby.LobbySetting.Cost630 {
		lobby.Rule.RenpoVital = 630
		lobby.Rule.ZeonVital = 630
	}

	if 0 < lobby.LobbySetting.AutoReBattle {
		lobby.Rule.AutoRebattle = byte(lobby.LobbySetting.AutoReBattle)
	}

	for i := 1; i <= maxRoomCount; i++ {
		roomID := uint16(i)
		lobby.RenpoRooms[roomID] = NewRoom(app, platform, lobby, roomID, TeamRenpo)
		lobby.ZeonRooms[roomID] = NewRoom(app, platform, lobby, roomID, TeamZeon)
	}

	lobby.lobbySettingMessages = lobby.buildLobbySettingMessages()

	return lobby
}

func (l *LbsLobby) buildLobbySettingMessages() []*LbsMessage {
	toMsg := func(text string) *LbsMessage {
		return NewServerNotice(lbsChatMessage).Writer().
			WriteString("").
			WriteString("").
			WriteString(text).
			Write8(0). // chat_type
			Write8(0). // id color
			Write8(0). // handle color
			Write8(0).Msg() // msg color
	}

	boolToYesNo := func(yes bool) string {
		if yes {
			return "Yes"
		}
		return "No"
	}

	var msgs []*LbsMessage
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "LobbyID", l.ID)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "PingLimit", boolToYesNo(l.PingLimit))))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "McsRegion", l.McsRegion)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "DamageLevel", l.Rule.DamageLevel+1)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "Difficulty", l.Rule.Difficulty+1)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "TeamShuffle", boolToYesNo(l.TeamShuffle))))
	if 0 < l.AutoReBattle {
		msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "Auto Re Battle", l.AutoReBattle)))
	}
	if l.No375MS {
		msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "No 375 Cost MS", boolToYesNo(l.No375MS))))
	}
	if l.Cost630 {
		msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "Cost630", boolToYesNo(l.Cost630))))
	}
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "/f Allowed", boolToYesNo(l.EnableForceStartCmd))))

	return msgs
}

func (l *LbsLobby) canStartBattle() bool {
	a, b := l.GetLobbyMatchEntryUserCount()

	if l.TeamShuffle {
		return 4 <= a+b
	} else {
		return 2 <= a && 2 <= b
	}
}

func (l *LbsLobby) StartForceStartCountDown() {
	l.forceStartCountDown = 10
}

func (l *LbsLobby) CancelForceStart() {
	l.forceStartCountDown = 0
}

func (l *LbsLobby) NotifyLobbyEvent(kind string, text string) {
	msgBody := text
	if 0 < len(kind) {
		msgBody = fmt.Sprintf("%-12s", kind) + text
	}

	msg := NewServerNotice(lbsChatMessage).Writer().
		WriteString("").
		WriteString("").
		WriteString(msgBody).
		Write8(0). // chat_type
		Write8(0). // id color
		Write8(0). // handle color
		Write8(0).Msg() // msg color
	for userID := range l.Users {
		peer := l.app.FindPeer(userID)
		if peer.Room != nil {
			continue
		}
		peer.SendMessage(msg)
	}
}

func (l *LbsLobby) FindRoom(side, roomID uint16) *LbsRoom {
	if side == TeamRenpo {
		r, ok := l.RenpoRooms[roomID]
		if !ok {
			return nil
		}
		return r
	} else if side == TeamZeon {
		r, ok := l.ZeonRooms[roomID]
		if !ok {
			return nil
		}
		return r
	}
	return nil
}

func (l *LbsLobby) SwitchTeam(p *LbsPeer) {
	switch p.Team {
	case TeamNone:
		l.NotifyLobbyEvent("EXIT", fmt.Sprintf("【%v】%v", p.UserID, p.Name))
	case TeamRenpo:
		for _, msg := range l.lobbySettingMessages {
			p.SendMessage(msg)
		}
		l.NotifyLobbyEvent("ENTER RENPO", fmt.Sprintf("【%v】%v", p.UserID, p.Name))
	case TeamZeon:
		for _, msg := range l.lobbySettingMessages {
			p.SendMessage(msg)
		}
		l.NotifyLobbyEvent("ENTER ZEON", fmt.Sprintf("【%v】%v", p.UserID, p.Name))
	}
}

func (l *LbsLobby) Enter(p *LbsPeer) {
	l.Users[p.UserID] = &p.DBUser
}

func (l *LbsLobby) Exit(userID string) {
	_, ok := l.Users[userID]
	if ok {
		delete(l.Users, userID)
		for i, id := range l.EntryUsers {
			if id == userID {
				l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
				l.CancelForceStart()
			}
		}
	}
}

func (l *LbsLobby) Entry(p *LbsPeer) {
	l.CancelForceStart()
	l.EntryUsers = append(l.EntryUsers, p.UserID)
	if p.Team == TeamRenpo {
		l.NotifyLobbyEvent("JOIN RENPO", p.Name)
	} else if p.Team == TeamZeon {
		l.NotifyLobbyEvent("JOIN ZEON", p.Name)
	}
}

func (l *LbsLobby) EntryCancel(p *LbsPeer) {
	l.CancelForceStart()
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
		}
	}
	l.NotifyLobbyEvent("CANCEL", p.Name)
}

func (l *LbsLobby) EntryPicked(p *LbsPeer) {
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
		}
	}
	l.NotifyLobbyEvent("GO BATTLE", fmt.Sprintf("【%v】%v", p.UserID, p.Name))
}

func (l *LbsLobby) GetUserCountBySide() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for userID := range l.Users {
		if p := l.app.FindPeer(userID); p != nil {
			switch p.Team {
			case TeamRenpo:
				a++
			case TeamZeon:
				b++
			}
		}
	}
	return a, b
}

func (l *LbsLobby) GetLobbyMatchEntryUserCount() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for _, userID := range l.EntryUsers {
		if p := l.app.FindPeer(userID); p != nil {
			switch p.Team {
			case TeamRenpo:
				a++
			case TeamZeon:
				b++
			}
		}
	}
	return a, b
}

func (l *LbsLobby) findBestGCPRegion(peers []*LbsPeer) (string, error) {
	type regionPing struct {
		Region string `json:"region"`
		Ping   int    `json:"ping"`
	}

	var regionPings []regionPing

	for region := range gcpLocationName {
		maxRtt := 0

		for _, p := range peers {
			rtt, err := strconv.Atoi(p.PlatformInfo[region])
			if rtt <= 0 || err != nil {
				rtt = 999
			}
			if maxRtt < rtt {
				maxRtt = rtt
			}
		}

		regionPings = append(regionPings, regionPing{
			Region: region,
			Ping:   maxRtt,
		})
	}

	sort.SliceStable(regionPings, func(i, j int) bool {
		return regionPings[i].Ping < regionPings[j].Ping
	})

	logger.Info("findBestGCPRegion", zap.Any("regionPings", regionPings))

	if len(regionPings) == 0 || regionPings[0].Ping == 0 || regionPings[0].Ping == 999 {
		return "", fmt.Errorf("no available region")
	}

	return regionPings[0].Region, nil
}

func (l *LbsLobby) getNextLobbyBattleParticipants() []*LbsPeer {
	var peers []*LbsPeer

	if l.TeamShuffle {
		for _, userID := range l.EntryUsers {
			if p := l.app.FindPeer(userID); p != nil {
				peers = append(peers, p)
			}
			if len(peers) == 4 {
				break
			}
		}
	} else {
		a := 0
		b := 0
		for _, userID := range l.EntryUsers {
			if p := l.app.FindPeer(userID); p != nil {
				switch p.Team {
				case TeamRenpo:
					if a < 2 {
						peers = append(peers, p)
						a++
					}
				case TeamZeon:
					if b < 2 {
						peers = append(peers, p)
						b++
					}
				}
			}
		}
	}

	return peers
}

func (l *LbsLobby) pickLobbyBattleParticipants() []*LbsPeer {
	peers := l.getNextLobbyBattleParticipants()

	if l.TeamShuffle {
		var teams = []uint16{1, 1, 2, 2}

		rand.Shuffle(len(teams), func(i, j int) {
			teams[i], teams[j] = teams[j], teams[i]
		})

		for i := 0; i < len(peers); i++ {
			peers[i].Team = teams[i]
		}

		logger.Info("shuffle team", zap.Any("teams", teams))
	}

	sort.SliceStable(peers, func(i, j int) bool {
		return peers[i].Team < peers[j].Team
	})

	for _, p := range peers {
		l.EntryPicked(p)
	}

	return peers
}

// Update updates lobby functions, should be called every 1 sec in the event loop.
func (l *LbsLobby) Update() {
	forceStart := false

	if l.EnableForceStartCmd && 0 < l.forceStartCountDown {
		l.NotifyLobbyEvent("", fmt.Sprintf("Force start battle in %d", l.forceStartCountDown))
		l.forceStartCountDown--

		if l.forceStartCountDown == 0 {
			forceStart = true
		}
	}

	l.checkLobbyBattleStart(forceStart)
	l.checkRoomBattleStart()
}

func (l *LbsLobby) checkLobbyBattleStart(force bool) {
	if !(force || l.canStartBattle()) {
		return
	}

	var mcsRegion = l.McsRegion
	var mcsPeer *LbsPeer
	var mcsAddr = conf.BattlePublicAddr

	if mcsRegion == "best" {
		bestRegion, err := l.findBestGCPRegion(l.getNextLobbyBattleParticipants())
		if err != nil {
			logger.Error("findBestGCPRegion failed", zap.Error(err))
			l.NotifyLobbyEvent("", "Failed to find best region.")
			l.NotifyLobbyEvent("", "Use default server.")
			mcsRegion = ""
		} else {
			logger.Info("findBestGCPRegion", zap.String("best_region", bestRegion))
			mcsRegion = bestRegion
		}
	}

	if McsFuncEnabled() && mcsRegion != "" {
		stat := l.app.FindMcs(mcsRegion)
		if stat == nil {
			logger.Info("mcs status not found")
			if GoMcsFuncAlloc(mcsRegion) {
				l.NotifyLobbyEvent("", "Allocating game server...")
			}
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			if GoMcsFuncAlloc(mcsRegion) {
				l.NotifyLobbyEvent("", "Waiting game server...")
			}
			return
		}

		mcsPeer = peer
		mcsAddr = stat.PublicAddr
	}

	l.NotifyLobbyEvent("", "START LOBBY BATTLE")

	b := NewBattle(l.app, l.ID, l.Rule, mcsRegion, mcsAddr)

	participants := l.pickLobbyBattleParticipants()

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		aggregate := 1
		if force || l.Rule.NoRanking == 1 {
			aggregate = 0
		}
		err := getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  aggregate,
		})
		if err != nil {
			logger.Error("AddBattleRecord failed", zap.Error(err))
			return
		}
	}

	sharedData.ShareMcsGame(&McsGame{
		BattleCode: b.BattleCode,
		Rule:       *b.Rule,
		GameDisk:   int(l.GameDisk),
		UpdatedAt:  time.Now(),
		State:      McsGameStateCreated,
		McsAddr:    mcsAddr,
	})

	for _, q := range participants {
		sharedData.ShareMcsUser(&McsUser{
			BattleCode:  b.BattleCode,
			McsRegion:   b.McsRegion,
			UserID:      q.UserID,
			Name:        q.Name,
			PilotName:   q.PilotName,
			GameParam:   q.GameParam,
			BattleCount: q.BattleCount,
			WinCount:    q.WinCount,
			LoseCount:   q.LoseCount,
			Side:        q.Team,
			SessionID:   q.SessionID,
			UpdatedAt:   time.Now(),
			State:       McsUserStateCreated,
		})
		NotifyReadyBattle(q)
	}

	if mcsPeer != nil {
		sharedData.NotifyLatestLbsStatus(mcsPeer)
	}

	l.app.BroadcastLobbyUserCount(l)
	l.app.BroadcastLobbyMatchEntryUserCount(l)
}

func (l *LbsLobby) checkRoomBattleStart() {
	var (
		renpoRoom    *LbsRoom
		zeonRoom     *LbsRoom
		participants []*LbsPeer
	)

	for _, room := range l.RenpoRooms {
		if room.IsReady() {
			var peers []*LbsPeer
			allOk := true
			for _, u := range room.Users {
				p := l.app.FindPeer(u.UserID)
				peers = append(peers, p)
				if p == nil {
					allOk = false
					break
				}
			}
			if allOk {
				renpoRoom = room
				participants = append(participants, peers...)
				break
			}
		}
	}

	for _, room := range l.ZeonRooms {
		if room.IsReady() {
			var peers []*LbsPeer
			allOk := true
			for _, u := range room.Users {
				p := l.app.FindPeer(u.UserID)
				if p == nil {
					allOk = false
					break
				}
				peers = append(peers, p)
			}
			if allOk {
				zeonRoom = room
				participants = append(participants, peers...)
				break
			}
		}
	}

	if renpoRoom == nil || zeonRoom == nil {
		return
	}

	var mcsRegion = l.McsRegion
	var mcsPeer *LbsPeer
	var mcsAddr = conf.BattlePublicAddr

	if mcsRegion == "best" {
		bestRegion, err := l.findBestGCPRegion(l.getNextLobbyBattleParticipants())
		if err != nil {
			logger.Error("findBestGCPRegion failed", zap.Error(err))
			l.NotifyLobbyEvent("", "Failed to find best region.")
			l.NotifyLobbyEvent("", "Use default server.")
			mcsRegion = ""
		} else {
			logger.Info("findBestGCPRegion", zap.String("best_region", bestRegion))
			mcsRegion = bestRegion
		}
	}

	if McsFuncEnabled() && mcsRegion != "" {
		stat := l.app.FindMcs(mcsRegion)
		if stat == nil {
			if GoMcsFuncAlloc(mcsRegion) {
				renpoRoom.NotifyRoomEvent("", "Allocating game server...")
				zeonRoom.NotifyRoomEvent("", "Allocating game server...")
			}
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			logger.Info("mcs peer not found")
			if GoMcsFuncAlloc(mcsRegion) {
				renpoRoom.NotifyRoomEvent("", "Waiting game server...")
				zeonRoom.NotifyRoomEvent("", "Waiting game server...")
			}
			return
		}

		mcsPeer = peer
		mcsAddr = stat.PublicAddr
	}

	renpoRoom.NotifyRoomEvent("", "START ROOM BATTLE")
	zeonRoom.NotifyRoomEvent("", "START ROOM BATTLE")

	b := NewBattle(l.app, l.ID, l.Rule, mcsRegion, mcsAddr)

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		aggregate := 1
		if l.Rule.NoRanking == 1 {
			aggregate = 0
		}
		err := getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  aggregate,
		})
		if err != nil {
			logger.Error("AddBattleRecord failed", zap.Error(err))
			return
		}
	}

	sharedData.ShareMcsGame(&McsGame{
		BattleCode: b.BattleCode,
		Rule:       *b.Rule,
		GameDisk:   int(l.GameDisk),
		UpdatedAt:  time.Now(),
		State:      McsGameStateCreated,
		McsAddr:    mcsAddr,
	})

	for _, q := range participants {
		sharedData.ShareMcsUser(&McsUser{
			BattleCode:  b.BattleCode,
			McsRegion:   b.McsRegion,
			UserID:      q.UserID,
			Name:        q.Name,
			PilotName:   q.PilotName,
			GameParam:   q.GameParam,
			BattleCount: q.BattleCount,
			WinCount:    q.WinCount,
			LoseCount:   q.LoseCount,
			Side:        q.Team,
			SessionID:   q.SessionID,
			UpdatedAt:   time.Now(),
			State:       McsUserStateCreated,
		})
		NotifyReadyBattle(q)
	}

	if mcsPeer != nil {
		sharedData.NotifyLatestLbsStatus(mcsPeer)
	}
}
