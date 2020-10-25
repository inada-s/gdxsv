package main

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"go.uber.org/zap"
)

type LbsLobby struct {
	app *Lbs

	LobbySetting
	lobbySettingMessages []*LbsMessage

	forceStart bool
	countDown  int
	extraCost  bool
	ecTimeout  int

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

		forceStart: false,
		countDown:  10,
		extraCost:  false,
		ecTimeout:  0,

		GameDisk:   platform,
		ID:         lobbyID,
		Rule:       RulePresetDefault.Clone(),
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
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
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "McsRegion", l.McsRegion)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "DamageLevel", l.Rule.DamageLevel+1)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "Difficulty", l.Rule.Difficulty+1)))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "TeamShuffle", boolToYesNo(l.TeamShuffle))))
	msgs = append(msgs, toMsg(fmt.Sprintf("%-12s: %v", "/ec /nc Allowed", boolToYesNo(l.EnableExtraCostCmd))))
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

func (l *LbsLobby) ForceStartBattle() {
	l.countDown = 10
	l.forceStart = true
}

func (l *LbsLobby) CancelForceStartBattle() {
	l.forceStart = false
}

func (l *LbsLobby) EnableExtraCost() {
	l.ecTimeout = 120
	l.extraCost = true
}

func (l *LbsLobby) DisableExtraCost() {
	l.ecTimeout = 0
	l.extraCost = false
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

	switch kind {
	case "ENTER RENPO", "ENTER ZEON", "JOIN RENPO", "JOIN ZEON", "CANCEL", "RETURN":
		l.CancelForceStartBattle()
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
				break
			}
		}
	}
}

func (l *LbsLobby) Entry(p *LbsPeer) {
	l.EntryUsers = append(l.EntryUsers, p.UserID)
	if p.Team == TeamRenpo {
		l.NotifyLobbyEvent("JOIN RENPO", p.Name)
	} else if p.Team == TeamZeon {
		l.NotifyLobbyEvent("JOIN ZEON", p.Name)
	}
}

func (l *LbsLobby) EntryCancel(p *LbsPeer) {
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
			break
		}
	}
	l.NotifyLobbyEvent("CANCEL", p.Name)
}

func (l *LbsLobby) EntryPicked(p *LbsPeer) {
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
			break
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

func (l *LbsLobby) pickLobbyBattleParticipants() []*LbsPeer {
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

		var teams = []uint16{1, 1, 2, 2}

		rand.Shuffle(len(teams), func(i, j int) {
			teams[i], teams[j] = teams[j], teams[i]
		})

		for i := 0; i < len(peers); i++ {
			peers[i].Team = teams[i]
		}

		sort.SliceStable(peers, func(i, j int) bool {
			return peers[i].Team < peers[j].Team
		})

		logger.Info("shuffle team", zap.Any("teams", teams))
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

		sort.SliceStable(peers, func(i, j int) bool {
			return peers[i].Team < peers[j].Team
		})
	}

	for _, p := range peers {
		l.EntryPicked(p)
	}

	return peers
}

func (l *LbsLobby) CheckLobbyBattleStart() {
	if l.forceStart && l.countDown > 0 {
		l.NotifyLobbyEvent("", fmt.Sprintf("Force start battle in %d", l.countDown))
		l.countDown--
	}

	if l.ecTimeout > 0 {
		l.ecTimeout--
	} else {
		l.extraCost = false
	}

	if !l.canStartBattle() && !(l.forceStart == true && l.countDown == 0) {
		return
	}

	var mcsPeer *LbsPeer
	var mcsAddr = conf.BattlePublicAddr

	if McsFuncEnabled() && l.McsRegion != "" {
		stat := l.app.FindMcs(l.McsRegion)
		if stat == nil {
			logger.Info("mcs status not found")
			if GoMcsFuncAlloc(l.McsRegion) {
				l.NotifyLobbyEvent("", "Allocating game server...")
			}
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			if GoMcsFuncAlloc(l.McsRegion) {
				l.NotifyLobbyEvent("", "Waiting game server...")
			}
			return
		}

		mcsPeer = peer
		mcsAddr = stat.PublicAddr
	}

	l.NotifyLobbyEvent("", "START LOBBY BATTLE")

	b := NewBattle(l.app, l.ID, l.Rule, l.McsRegion, mcsAddr)

	if l.extraCost {
		ecRule := *l.Rule
		ecRule.RenpoVital = 630
		ecRule.ZeonVital = 630
		b.SetRule(&ecRule)
		l.extraCost = false
	}

	participants := l.pickLobbyBattleParticipants()

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		aggregate := 1
		if l.forceStart {
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

	l.CancelForceStartBattle() //battle started, reset forceStart flag
}

func (l *LbsLobby) CheckRoomBattleStart() {
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

	var mcsPeer *LbsPeer
	var mcsAddr = conf.BattlePublicAddr

	if McsFuncEnabled() && l.McsRegion != "" {
		stat := l.app.FindMcs(l.McsRegion)
		if stat == nil {
			if GoMcsFuncAlloc(l.McsRegion) {
				renpoRoom.NotifyRoomEvent("", "Allocating game server...")
				zeonRoom.NotifyRoomEvent("", "Allocating game server...")
			}
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			logger.Info("mcs peer not found")
			if GoMcsFuncAlloc(l.McsRegion) {
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

	b := NewBattle(l.app, l.ID, l.Rule, l.McsRegion, mcsAddr)

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		err := getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  1,
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
