package main

import "gdxsv/gdxsv/battle"

type Lobby struct {
	app *App

	Platform   uint8
	ID         uint16
	Rule       *Rule
	Users      map[string]*DBUser
	RenpoRooms map[uint16]*Room
	ZeonRooms  map[uint16]*Room
	EntryUsers []string
}

func NewLobby(app *App, platform uint8, lobbyID uint16) *Lobby {
	lobby := &Lobby{
		app: app,

		Platform:   platform,
		ID:         lobbyID,
		Rule:       NewRule(),
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*Room),
		ZeonRooms:  make(map[uint16]*Room),
		EntryUsers: make([]string, 0),
	}
	for i := 1; i <= maxRoomCount; i++ {
		roomID := uint16(i)
		lobby.RenpoRooms[roomID] = NewRoom(app, platform, lobby, roomID, EntryRenpo)
		lobby.ZeonRooms[roomID] = NewRoom(app, platform, lobby, roomID, EntryZeon)
	}
	return lobby
}

func (l *Lobby) FindRoom(side, roomID uint16) *Room {
	if side == EntryRenpo {
		r, ok := l.RenpoRooms[roomID]
		if !ok {
			return nil
		}
		return r
	} else if side == EntryZeon {
		r, ok := l.ZeonRooms[roomID]
		if !ok {
			return nil
		}
		return r
	}
	return nil
}

func (l *Lobby) CheckLobbyBattleStart() {
	if !l.CanBattleStart() {
		return
	}
	b := NewBattle(l.app, l.ID)
	b.BattleCode = GenBattleCode()
	participants := l.PickBattleUsers()
	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		battle.AddUserWhoIsGoingTobattle(
			b.BattleCode, q.UserID, q.Name, q.Entry, q.SessionID)
		getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  1,
		})
		NotifyReadyBattle(q)
	}
}

func (l *Lobby) CheckRoomBattleStart() {
	var (
		renpoRoom    *Room
		zeonRoom     *Room
		participants []*AppPeer
	)

	for _, room := range l.RenpoRooms {
		if room.IsReady() {
			var peers []*AppPeer
			allOk := true
			for _, u := range room.Users {
				p, ok := l.app.FindPeer(u.UserID)
				if !ok {
					allOk = false
				}
				peers = append(peers, p)
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
			var peers []*AppPeer
			allOk := true
			for _, u := range room.Users {
				p, ok := l.app.FindPeer(u.UserID)
				if !ok {
					allOk = false
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

	b := NewBattle(l.app, l.ID)
	b.BattleCode = GenBattleCode()

	for _, q := range participants {
		b.Add(q)
		q.Battle = b
		battle.AddUserWhoIsGoingTobattle(
			b.BattleCode, q.UserID, q.Name, q.Entry, q.SessionID)
		getDB().AddBattleRecord(&BattleRecord{
			BattleCode: b.BattleCode,
			UserID:     q.UserID,
			UserName:   q.Name,
			PilotName:  q.PilotName,
			Players:    len(participants),
			Aggregate:  1,
		})
		NotifyReadyBattle(q)
	}
}

func (l *Lobby) Enter(p *AppPeer) {
	l.Users[p.UserID] = &p.DBUser
}

func (l *Lobby) Exit(userID string) {
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

func (l *Lobby) Entry(u *AppPeer) {
	l.EntryUsers = append(l.EntryUsers, u.UserID)
}

func (l *Lobby) EntryCancel(userID string) {
	for i, id := range l.EntryUsers {
		if id == userID {
			l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
			break
		}
	}
}

func (l *Lobby) GetUserCountBySide() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for userID := range l.Users {
		if p, ok := l.app.FindPeer(userID); ok {
			switch p.Entry {
			case EntryRenpo:
				a++
			case EntryZeon:
				b++
			}
		}
	}
	return a, b
}

func (l *Lobby) GetLobbyMatchEntryUserCount() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for _, userID := range l.EntryUsers {
		if p, ok := l.app.FindPeer(userID); ok {
			switch p.Entry {
			case EntryRenpo:
				a++
			case EntryZeon:
				b++
			}
		}
	}
	return a, b
}

func (l *Lobby) CanBattleStart() bool {
	a, b := l.GetLobbyMatchEntryUserCount()
	if l.ID == 2 {
		// This game requires four players to play,
		// but can check the connection to the battle server.
		return 1 <= a || 1 <= b
	}
	return 2 <= a && 2 <= b
}

func (l *Lobby) PickBattleUsers() []*AppPeer {
	a := uint16(0)
	b := uint16(0)
	peers := []*AppPeer{}
	for _, userID := range l.EntryUsers {
		if p, ok := l.app.FindPeer(userID); ok {
			switch p.Entry {
			case EntryRenpo:
				if a < 2 {
					peers = append(peers, p)
				}
				a++
			case EntryZeon:
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
