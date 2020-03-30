package main

import (
	"time"
)

const (
	RoomStateEmpty      = 1
	RoomStatePrepare    = 2
	RoomStateRecruiting = 3
	RoomStateFull       = 4
)

type Room struct {
	ID        uint16
	LobbyID   uint16
	Name      string
	MaxPlayer uint16
	Password  string
	Owner     string
	Deadline  time.Time
	Users     []*AppPeer
	Status    byte
	Rule      *Rule
}

func NewRoom(lobbyID, roomID uint16) *Room {
	return &Room{
		ID:      roomID,
		LobbyID: lobbyID,
		Name:    "",
		Status:  RoomStateEmpty,
		Rule:    NewRule(),
		Users:   make([]*AppPeer, 0),
	}
}

func (r *Room) Enter(u *AppPeer) {
	if len(r.Users) == 0 {
		r.Owner = u.UserID
		r.Deadline = time.Now().Add(30 * time.Minute)
		// r.MaxPlayer = r.Rule.playerCount
	}

	userAlreadyExists := false
	for _, ru := range r.Users {
		if u.UserID == ru.UserID {
			userAlreadyExists = true
		}
	}
	if !userAlreadyExists {
		r.Users = append(r.Users, u)
	}

	if len(r.Users) == int(r.MaxPlayer) {
		r.Status = RoomStateFull
	} else {
		r.Status = RoomStateRecruiting
	}
}

func (r *Room) Exit(userID string) {
	for i, u := range r.Users {
		if u.UserID == userID {
			r.Users, r.Users[len(r.Users)-1] = append(r.Users[:i], r.Users[i+1:]...), nil
			break
		}
	}

	if len(r.Users) == int(r.MaxPlayer) {
		r.Status = RoomStateFull
	} else {
		r.Status = RoomStateRecruiting
	}

	if len(r.Users) == 0 {
		r.Remove()
	}
}

func (r *Room) Remove() {
	*r = *NewRoom(r.LobbyID, r.ID)
}

func (r *Room) Entry(u *AppPeer, side uint16) {
	u.Entry = side
}

func (r *Room) GetEntryUserCount() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for _, u := range r.Users {
		switch u.Entry {
		case EntryRenpo:
			a++
		case EntryZeon:
			b++
		}
	}
	return a, b
}

func (r *Room) CanBattleStart() bool {
	a, b := r.GetEntryUserCount()
	return 0 < a && 0 < b && a <= 2 && b <= 2
}

func (r *Room) StartBattleUsers() (active []*AppPeer, inactive []*AppPeer) {
	a := uint16(0)
	b := uint16(0)
	for _, u := range r.Users {
		switch u.Entry {
		case EntryRenpo:
			if a < 2 {
				active = append(active, u)
			} else {
				inactive = append(inactive, u)
			}
			a++
		case EntryZeon:
			if b < 2 {
				active = append(active, u)
			} else {
				inactive = append(inactive, u)
			}
			b++
		default:
			inactive = append(inactive, u)
		}
	}
	return active, inactive
}
