package main

import (
	"database/sql"
	"fmt"
	"go.uber.org/zap"
	"math/rand"
	"sort"
	"strconv"
	"time"
)

const (
	PingLimitTh = 64
)

type LobbySetting MLobbySetting

type LbsLobby struct {
	app                  *Lbs
	Platform             string
	GameDisk             string
	ID                   uint16
	Users                map[string]*DBUser
	RenpoRooms           map[uint16]*LbsRoom
	ZeonRooms            map[uint16]*LbsRoom
	EntryUsers           []string
	Description          string
	LobbySetting         LobbySetting
	Rule                 Rule
	lobbySettingMessages []*LbsMessage
	forceStartCountDown  int
}

func NewLobby(app *Lbs, platform, disk string, lobbyID uint16) *LbsLobby {
	lobby := &LbsLobby{
		app:                  app,
		Platform:             platform,
		GameDisk:             disk,
		ID:                   lobbyID,
		Users:                make(map[string]*DBUser),
		RenpoRooms:           make(map[uint16]*LbsRoom),
		ZeonRooms:            make(map[uint16]*LbsRoom),
		EntryUsers:           make([]string, 0),
		LobbySetting:         LobbySetting{},
		Rule:                 DefaultRule,
		Description:          "",
		lobbySettingMessages: nil,
		forceStartCountDown:  0,
	}

	for i := 1; i <= maxRoomCount; i++ {
		roomID := uint16(i)
		lobby.RenpoRooms[roomID] = NewRoom(app, platform, disk, lobby, roomID, TeamRenpo)
		lobby.ZeonRooms[roomID] = NewRoom(app, platform, disk, lobby, roomID, TeamZeon)
	}

	err := lobby.LoadLobbySetting()
	if err != nil {
		if err != sql.ErrNoRows {
			logger.Warn("Failed to load lobby setting",
				zap.Error(err), zap.String("platform", platform),
				zap.String("disk", disk), zap.Int("lobby_id", int(lobbyID)))
		}
	}

	return lobby
}

func (l *LbsLobby) LoadLobbySetting() error {
	setting, err := getDB().GetLobbySetting(l.Platform, l.GameDisk, int(l.ID))
	if err == sql.ErrNoRows {
		l.Rule = DefaultRule
		return nil
	}

	if err != nil {
		return err
	}

	var rule *MRule = nil
	if setting.RuleID != "" {
		rule, err = getDB().GetRule(setting.RuleID)
		if err != nil {
			return err
		}
	}

	if setting != nil {
		l.LobbySetting = LobbySetting(*setting)
	}

	if rule != nil {
		l.Rule = Rule(*rule)
	}

	l.lobbySettingMessages = l.buildLobbySettingMessages()

	return err
}

func chatMsg(userID, name, text string) *LbsMessage {
	return NewServerNotice(lbsChatMessage).Writer().
		WriteString(userID).
		WriteString(name).
		WriteString(text).
		Write8(0). // chat_type
		Write8(0). // id color
		Write8(0). // handle color
		Write8(0).Msg() // msg color
}

func (l *LbsLobby) sendLobbyChat(userID, name, text string) {
	msg := chatMsg(userID, name, text)
	for user := range l.Users {
		peer := l.app.FindPeer(user)
		if peer.Room != nil {
			continue
		}
		peer.SendMessage(msg)
	}
}

func (l *LbsLobby) buildLobbySettingMessages() []*LbsMessage {
	boolToYesNo := func(yes bool) string {
		if yes {
			return "Yes"
		}
		return "No"
	}

	var msgs []*LbsMessage
	msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "LobbyID", l.ID)))

	if l.LobbySetting.PingLimit {
		msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "PingLimit", boolToYesNo(l.LobbySetting.PingLimit))))
	}
	if l.LobbySetting.McsRegion != "" {
		msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "McsRegion", l.LobbySetting.McsRegion)))
	}

	msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v/%v", "Dmage/Diff", l.Rule.DamageLevel+1, l.Rule.Difficulty+1)))

	if l.LobbySetting.TeamShuffle {
		msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "TeamShuffle", boolToYesNo(l.LobbySetting.TeamShuffle))))
	}
	if 0 < l.Rule.AutoRebattle {
		msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "Auto Re Battle", l.Rule.AutoRebattle)))
	}
	if l.LobbySetting.EnableForceStart {
		msgs = append(msgs, chatMsg("", "", fmt.Sprintf("%-12s: %v", "/f Allowed", boolToYesNo(l.LobbySetting.EnableForceStart))))
	}

	return msgs
}

func (l *LbsLobby) buildDescription(ping string) string {
	locName, ok := gcpLocationName[l.LobbySetting.McsRegion]
	if !ok {
		locName = "Default Server"
	}
	if l.LobbySetting.McsRegion == "best" {
		locName = "Best Server [Auto Detection]"
	}
	if ping == "" {
		return fmt.Sprintf("<B>%s<B><BR><B>%s<END>", locName, l.LobbySetting.Comment)
	} else {
		return fmt.Sprintf("<B>[%sms]%s<B><BR><B>%s<END>", ping, locName, l.LobbySetting.Comment)
	}
}

func (l *LbsLobby) printSameLobbyUsers(peer *LbsPeer) {
	entryUserIDs := map[string]bool{}
	for _, id := range l.EntryUsers {
		entryUserIDs[id] = true
	}

	if 12 < len(l.Users) {
		peer.SendMessage(chatMsg("", "", "Many users in this lobby"))
	} else {
		for userID := range l.Users {
			if userID == peer.UserID {
				continue
			}
			p := l.app.FindPeer(userID)
			if p == nil || p.Team == TeamNone {
				continue
			}
			if entryUserIDs[userID] {
				continue
			}

			if p.Team == TeamRenpo {
				if p.Room == nil {
					peer.SendMessage(chatMsg(p.UserID, p.Name, ">連邦"))
				} else {
					peer.SendMessage(chatMsg(p.UserID, p.Name, ">連邦>パートナー募集"))
				}
			} else if p.Team == TeamZeon {
				if p.Room == nil {
					peer.SendMessage(chatMsg(p.UserID, p.Name, ">ジオン"))
				} else {
					peer.SendMessage(chatMsg(p.UserID, p.Name, ">ジオン>パートナー募集"))
				}
			}
		}
	}

	for _, userID := range l.EntryUsers {
		if userID == peer.UserID {
			continue
		}
		p := l.app.FindPeer(userID)
		if p == nil || p.Team == TeamNone {
			continue
		}

		if p.Team == TeamRenpo {
			peer.SendMessage(chatMsg(p.UserID, p.Name, ">連邦>自動選抜"))
		} else if p.Team == TeamZeon {
			peer.SendMessage(chatMsg(p.UserID, p.Name, ">ジオン>自動選抜"))
		}
	}
}

func (l *LbsLobby) printLobbyMatchEntryCount(peer *LbsPeer) {
	a, b := l.GetLobbyMatchEntryUserCount()
	peer.SendMessage(chatMsg("", "", fmt.Sprintf("【自動選抜】連邦×%d  ジオン×%d", a, b)))
}

func (l *LbsLobby) SwitchTeam(p *LbsPeer) {
	switch p.Team {
	case TeamNone:
		l.sendLobbyChat(p.UserID, p.Name, "<退")
	case TeamRenpo:
		l.printLobbySetting(p)
		l.printSameLobbyUsers(p)
		l.sendLobbyChat(p.UserID, p.Name, ">連邦")
		l.printLobbyMatchEntryCount(p)
	case TeamZeon:
		l.printLobbySetting(p)
		l.printSameLobbyUsers(p)
		l.sendLobbyChat(p.UserID, p.Name, ">ジオン")
		l.printLobbyMatchEntryCount(p)
	}
}

func (l *LbsLobby) canStartBattle() bool {
	a, b := l.GetLobbyMatchEntryUserCount()

	if l.LobbySetting.TeamShuffle {
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

	msg := chatMsg("", "", msgBody)

	for userID := range l.Users {
		peer := l.app.FindPeer(userID)
		if peer.Room != nil {
			continue
		}
		peer.SendMessage(msg)
	}
}

func (l *LbsLobby) FindRoom(team, roomID uint16) *LbsRoom {
	if team == TeamRenpo {
		r, ok := l.RenpoRooms[roomID]
		if !ok {
			return nil
		}
		return r
	} else if team == TeamZeon {
		r, ok := l.ZeonRooms[roomID]
		if !ok {
			return nil
		}
		return r
	}
	return nil
}

func (l *LbsLobby) printLobbySetting(p *LbsPeer) {
	for _, msg := range l.lobbySettingMessages {
		p.SendMessage(msg)
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
	a, b := l.GetLobbyMatchEntryUserCount()
	if p.Team == TeamRenpo {
		l.sendLobbyChat(p.UserID, p.Name, fmt.Sprintf(">連邦>自動選抜"))
		l.NotifyLobbyEvent("", fmt.Sprintf("【自動選抜】連邦×%d  ジオン×%d", a, b))
	} else if p.Team == TeamZeon {
		l.sendLobbyChat(p.UserID, p.Name, fmt.Sprintf(">ジオン>自動選抜"))
		l.NotifyLobbyEvent("", fmt.Sprintf("【自動選抜】連邦×%d  ジオン×%d", a, b))
	}
}

func (l *LbsLobby) EntryCancel(p *LbsPeer) {
	l.CancelForceStart()
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
		}
	}
	if p.Team == TeamRenpo {
		l.sendLobbyChat(p.UserID, p.Name, fmt.Sprintf(">連邦"))
	} else if p.Team == TeamZeon {
		l.sendLobbyChat(p.UserID, p.Name, fmt.Sprintf(">ジオン"))
	}

	a, b := l.GetLobbyMatchEntryUserCount()
	l.NotifyLobbyEvent("", fmt.Sprintf("【自動選抜】連邦×%d  ジオン×%d", a, b))
}

func (l *LbsLobby) EntryPicked(p *LbsPeer) {
	for i, id := range l.EntryUsers {
		if id == p.UserID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
		}
	}
}

func (l *LbsLobby) GetUserCountByTeam() (uint16, uint16) {
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

	if l.LobbySetting.TeamShuffle {
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

	if l.LobbySetting.TeamShuffle {
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
		l.NotifyLobbyEvent("GO BATTLE", fmt.Sprintf("【%v】%v", p.UserID, p.Name))
	}

	a, b := l.GetLobbyMatchEntryUserCount()
	l.NotifyLobbyEvent("", fmt.Sprintf("      【自動選抜】連邦×%d  ジオン×%d", a, b))

	return peers
}

// Update updates lobby functions, should be called every 1 sec in the event loop.
func (l *LbsLobby) Update() {
	forceStart := false

	if l.LobbySetting.EnableForceStart && 0 < l.forceStartCountDown {
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

	var mcsRegion = l.LobbySetting.McsRegion
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

	b := NewBattle(l.app, l.ID, &l.Rule, mcsRegion, mcsAddr)

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
			LobbyID:    int(l.ID),
			Team:       int(q.Team),
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
		GameDisk:   l.GameDisk,
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
			Platform:    q.Platform,
			GameDisk:    q.GameDisk,
			BattleCount: q.BattleCount,
			WinCount:    q.WinCount,
			LoseCount:   q.LoseCount,
			Team:        q.Team,
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

	var mcsRegion = l.LobbySetting.McsRegion
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

	b := NewBattle(l.app, l.ID, &l.Rule, mcsRegion, mcsAddr)

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
			LobbyID:    int(l.ID),
			Team:       int(q.Team),
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
		GameDisk:   l.GameDisk,
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
			Platform:    q.Platform,
			GameDisk:    q.GameDisk,
			BattleCount: q.BattleCount,
			WinCount:    q.WinCount,
			LoseCount:   q.LoseCount,
			Team:        q.Team,
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
