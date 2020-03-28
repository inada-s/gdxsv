package main

const roomCount = 5

type Lobby struct {
	ID         uint16
	Rule       *Rule
	Users      map[string]*AppPeer
	Rooms      map[uint16]*Room
	EntryUsers []string
}

func NewLobby(lobbyID uint16) *Lobby {
	lobby := &Lobby{
		ID:         lobbyID,
		Rule:       NewRule(),
		Users:      make(map[string]*AppPeer),
		Rooms:      make(map[uint16]*Room),
		EntryUsers: make([]string, 0),
	}
	for i := uint16(0); i <= roomCount; i++ {
		lobby.Rooms[i] = NewRoom(lobbyID, i)
	}
	return lobby
}

func (l *Lobby) RoomCount() uint16 {
	return uint16(roomCount)
}

func (l *Lobby) Enter(u *AppPeer) {
	l.Users[u.UserID] = u
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

func (l *Lobby) Entry(u *AppPeer, side byte) {
	u.Entry = side
	if side == EntryNone {
		for i, id := range l.EntryUsers {
			if id == u.UserID {
				l.EntryUsers = append(l.EntryUsers[:i], l.EntryUsers[i+1:]...)
				break
			}
		}
	} else {
		l.EntryUsers = append(l.EntryUsers, u.UserID)
	}
}

func (l *Lobby) GetEntryUserCount() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for _, id := range l.EntryUsers {
		u, ok := l.Users[id]
		if ok {
			switch u.Entry {
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
	a, b := l.GetEntryUserCount()
	if l.ID == uint16(2) {
		return 1 <= a && 1 <= b
	}
	if l.ID == uint16(3) {
		return 1 <= a || 1 <= b
	}
	return 2 <= a && 2 <= b
}

func (l *Lobby) StartBattleUsers() []*AppPeer {
	a := uint16(0)
	b := uint16(0)
	ret := []*AppPeer{}
	for _, id := range l.EntryUsers {
		u, ok := l.Users[id]
		if ok {
			switch u.Entry {
			case EntryRenpo:
				if a < 2 {
					ret = append(ret, u)
				}
				a++
			case EntryZeon:
				if b < 2 {
					ret = append(ret, u)
				}
				b++
			}
		}
	}
	return ret
}
