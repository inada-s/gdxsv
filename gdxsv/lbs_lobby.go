package main

import (
	"fmt"
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

func NewLobby(app *Lbs, platform uint8, lobbyID uint16) *LbsLobby {
	lobby := &LbsLobby{
		app: app,

		Platform:   platform,
		ID:         lobbyID,
		Comment:    fmt.Sprintf("<B>Lobby %d<END>", lobbyID),
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
	switch lobbyID {
	case 2:
		lobby.Comment = "<B>戦績なし<B><BR><B>NO WIN/LOSE<END>"
		lobby.Rule.NoRanking = 1
	case 4:
		if platform != PlatformPS2 {
			lobby.Comment = "<B>1人対戦<B><BR><B>SINGLE PLAYER BATTLE<END>"
		}
	case 5:
		if platform != PlatformPS2 {
			lobby.Comment = "<B>2人対戦<B><BR><B>TWO PLAYERS BATTLE<END>"
		}
	case 6:
		lobby.Comment = "<B>弾無限<B><BR><B>UNLIMITED AMMO<END>"
		lobby.Rule.NoRanking = 1
		lobby.Rule.ReloadFlag = 1
	case 10:
		lobby.McsRegion = "asia-east1"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 11:
		lobby.McsRegion = "asia-east2"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 12:
		lobby.McsRegion = "asia-northeast1"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 13:
		lobby.McsRegion = "asia-northeast2"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 14:
		lobby.McsRegion = "australia-southeast1"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 15:
		lobby.McsRegion = "europe-west3"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 16:
		lobby.McsRegion = "us-central1"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 17:
		lobby.McsRegion = "us-east1"
		lobby.Comment = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", lobby.McsRegion, gcpLocationName[lobby.McsRegion])
	case 19:
		lobby.Comment = "<B>ダメージレベル4<B><BR><B>DAMAGELEVEL4<END>"
		lobby.Rule.DamageLevel = 3
	case 20:
		if platform != PlatformPS2 {
			lobby.Comment = "<B>弾無限(戦績なし) １人対戦<B><BR><B>UNLIMITED AMMO / SINGLE PLAYER<END>"
			lobby.Rule.NoRanking = 1
			lobby.Rule.ReloadFlag = 1
		}
	case 21:
		if platform != PlatformPS2 {
			lobby.Comment = "<B>弾無限(戦績なし) ２人対戦<B><BR><B>UNLIMITED AMMO / TWO PLAYER<END>"
			lobby.Rule.NoRanking = 1
			lobby.Rule.ReloadFlag = 1
		}
	}

	return lobby
}

func (l *LbsLobby) canStartBattle() bool {
	a, b := l.GetLobbyMatchEntryUserCount()
	if l.Platform != PlatformPS2 {
		switch l.ID {
		case 4, 20:
			return 1 <= a+b
		case 5, 21:
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
	peers := []*LbsPeer{}
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
			GoMcsFuncAlloc(l.McsRegion)
			return
		}
		peer := l.app.FindMcsPeer(stat.PublicAddr)
		if peer != nil {
			mcsPeer = peer
			mcsAddr = stat.PublicAddr
		}
	}

	b := NewBattle(l.app, l.ID, l.Rule, l.McsRegion, mcsAddr)

	participants := l.pickLobbyBattleParticipants()
	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  1,
		})
		AddUserWhoIsGoingTobattle(
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

	mcsAddr := conf.BattlePublicAddr
	if McsFuncEnabled() && l.McsRegion != "" {
		stat := l.app.FindMcs(l.McsRegion)
		if stat == nil {
			GoMcsFuncAlloc(l.McsRegion)
			return
		}
		mcsAddr = stat.PublicAddr
	}

	b := NewBattle(l.app, l.ID, l.Rule, l.McsRegion, mcsAddr)

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  1,
		})
		AddUserWhoIsGoingTobattle(
			b.BattleCode, b.McsRegion, q.UserID, q.Name, q.Team, q.SessionID)
		NotifyReadyBattle(q)
	}
}
