package main

import (
	"fmt"
	"go.uber.org/zap"
)

type LbsLobby struct {
	app *Lbs

	Platform   uint8
	ID         uint16
	Comment    string
	McsRegion  string
	Rule       *Rule
	Users      map[string]*DBUser
	RenpoRooms map[uint16]*LbsRoom
	ZeonRooms  map[uint16]*LbsRoom
	EntryUsers []string
}

var regionMapping = map[uint16]string{
	4:  "asia-east1",
	5:  "asia-east2",
	6:  "asia-northeast1",
	9:  "asia-northeast2",
	10: "asia-northeast3",
	11: "asia-south1",
	12: "asia-southeast1",
	13: "australia-southeast1",
	14: "europe-north1",
	15: "europe-west2",
	16: "europe-west6",
	17: "northamerica-northeast1",
	19: "southamerica-east1",
	20: "us-central1",
	21: "us-east1",
	22: "us-west3",
}

func NewLobby(app *Lbs, platform uint8, lobbyID uint16) *LbsLobby {
	lobby := &LbsLobby{
		app: app,

		Platform:   platform,
		ID:         lobbyID,
		Comment:    fmt.Sprintf("<B>Lobby %d<BR><B>Default Server<END>", lobbyID),
		McsRegion:  "",
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

	// Apply special lobby settings
	// PS2 LobbyID: 1-23
	// DC2 LobbyID: 2, 4-6, 9-17, 19-22
	if region, ok := regionMapping[lobby.ID]; ok {
		lobby.McsRegion = region
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	}

	return lobby
}

func (l *LbsLobby) canStartBattle() bool {
	a, b := l.GetLobbyMatchEntryUserCount()
	if l.Platform != PlatformPS2 {
		switch l.ID {
		case 4:
			return 1 <= a+b
		case 5:
			return 2 <= a+b
		}
	}
	return 2 <= a && 2 <= b
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

func (l *LbsLobby) Entry(u *LbsPeer) {
	l.EntryUsers = append(l.EntryUsers, u.UserID)
}

func (l *LbsLobby) EntryCancel(userID string) {
	for i, id := range l.EntryUsers {
		if id == userID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
			break
		}
	}
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
	a := uint16(0)
	b := uint16(0)
	var peers []*LbsPeer
	for _, userID := range l.EntryUsers {
		if p := l.app.FindPeer(userID); p != nil {
			switch p.Team {
			case TeamRenpo:
				if a < 2 {
					peers = append(peers, p)
				}
				a++
			case TeamZeon:
				if b < 2 {
					peers = append(peers, p)
				}
				b++
			}
		}
	}
	for _, p := range peers {
		l.EntryCancel(p.UserID)
	}
	return peers
}

func (l *LbsLobby) CheckLobbyBattleStart() {
	if !l.canStartBattle() {
		return
	}

	var mcsPeer *LbsPeer
	var mcsAddr = conf.BattlePublicAddr

	if McsFuncEnabled() && l.McsRegion != "" {
		stat := l.app.FindMcs(l.McsRegion)
		if stat == nil {
			logger.Info("mcs status not found")
			GoMcsFuncAlloc(l.McsRegion)
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			logger.Info("mcs peer not found")
			GoMcsFuncAlloc(l.McsRegion)
			return
		}

		mcsPeer = peer
		mcsAddr = stat.PublicAddr
	}

	b := NewBattle(l.app, l.ID, l.Rule, l.McsRegion, mcsAddr)

	participants := l.pickLobbyBattleParticipants()
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

	for _, q := range participants {
		AddUserWhoIsGoingToBattle(
			b.BattleCode, b.McsRegion, q.UserID, q.Name, q.Team, q.SessionID)
		NotifyReadyBattle(q)
	}

	if mcsPeer != nil {
		NotifyLatestLbsStatus(mcsPeer)
	}
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
			GoMcsFuncAlloc(l.McsRegion)
			return
		}

		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer == nil {
			logger.Info("mcs peer not found")
			GoMcsFuncAlloc(l.McsRegion)
			return
		}

		mcsPeer = peer
		mcsAddr = stat.PublicAddr
	}

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

	for _, q := range participants {
		AddUserWhoIsGoingToBattle(
			b.BattleCode, b.McsRegion, q.UserID, q.Name, q.Team, q.SessionID)
		NotifyReadyBattle(q)
	}

	if mcsPeer != nil {
		NotifyLatestLbsStatus(mcsPeer)
	}
}
