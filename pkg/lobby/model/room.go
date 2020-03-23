package model

import (
	"time"
)

const (
	RoomStateUnavailable = 0
	RoomStateEmpty       = 1
	RoomStatePrepare     = 2
	RoomStateRecruit     = 3
	RoomStateFull        = 4
)

type Room struct {
	ID        uint16
	LobbyID   uint16
	Name      string
	MaxPlayer uint16
	Password  string
	Owner     string
	Deadline  time.Time
	Users     []*User
	Status    byte
	Rule      *Rule
}

func NewRoom(lobbyID, roomID uint16) *Room {
	return &Room{
		ID:      roomID,
		LobbyID: lobbyID,
		Name:    "(空き)",
		Status:  RoomStateEmpty,
		Rule:    NewRule(),
		Users:   make([]*User, 0),
	}
}

func (r *Room) Enter(u *User) {
	if len(r.Users) == 0 {
		r.Owner = u.UserID
		r.Deadline = time.Now().Add(30 * time.Minute)
		r.MaxPlayer = r.Rule.playerCount
	}

	r.Users = append(r.Users, u)

	if len(r.Users) == int(r.MaxPlayer) {
		r.Status = RoomStateFull
	} else {
		r.Status = RoomStateRecruit
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
		r.Status = RoomStateRecruit
	}

	if len(r.Users) == 0 {
		r.Remove()
	}
}

func (r *Room) Remove() {
	*r = *NewRoom(r.LobbyID, r.ID)
}

func (r *Room) Entry(u *User, side byte) {
	u.Entry = side
}

func (r *Room) GetEntryUserCount() (uint16, uint16) {
	a := uint16(0)
	b := uint16(0)
	for _, u := range r.Users {
		switch u.Entry {
		case EntryAeug:
			a++
		case EntryTitans:
			b++
		}
	}
	return a, b
}

func (r *Room) CanBattleStart() bool {
	a, b := r.GetEntryUserCount()
	return 0 < a && 0 < b && a <= 2 && b <= 2
}

func (r *Room) StartBattleUsers() (active []*User, inactive []*User) {
	a := uint16(0)
	b := uint16(0)
	for _, u := range r.Users {
		switch u.Entry {
		case EntryAeug:
			if a < 2 {
				active = append(active, u)
			} else {
				inactive = append(inactive, u)
			}
			a++
		case EntryTitans:
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
