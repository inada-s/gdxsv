package main

type Lobby struct {
	app *App

	ID         uint16
	Rule       *Rule
	Users      map[string]*DBUser
	Rooms      map[uint16]*Room
	EntryUsers []string
}

func NewLobby(app *App, lobbyID uint16) *Lobby {
	lobby := &Lobby{
		app: app,

		ID:         lobbyID,
		Rule:       NewRule(),
		Users:      make(map[string]*DBUser),
		Rooms:      make(map[uint16]*Room),
		EntryUsers: make([]string, 0),
	}
	for i := 1; i <= maxRoomCount; i++ {
		roomID := uint16(i)
		lobby.Rooms[roomID] = NewRoom(lobbyID, roomID)
	}
	return lobby
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
	return 2 <= a && 2 <= b
}

func (l *Lobby) PickReadyToBattleUsers() []*AppPeer {
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
