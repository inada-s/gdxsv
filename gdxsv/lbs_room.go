package main

import (
	"time"
)

const (
	RoomStateEmpty      = 1
	RoomStateRecruiting = 3
	RoomStatePrepare    = 4
	RoomStateFull       = 5
)

type LbsRoom struct {
	app   *Lbs
	lobby *LbsLobby
	ready bool

	Platform  uint8
	ID        uint16
	Team      uint16
	Name      string
	MaxPlayer uint16
	Owner     string
	Deadline  time.Time
	Users     []*DBUser
	Status    byte
}

func NewRoom(app *Lbs, platform uint8, lobby *LbsLobby, roomID, team uint16) *LbsRoom {
	return &LbsRoom{
		app:   app,
		lobby: lobby,
		ready: false,

		Platform:  platform,
		ID:        roomID,
		Team:      team,
		Name:      "",
		MaxPlayer: 2,
		Owner:     "",
		Status:    RoomStateEmpty,
		Users:     make([]*DBUser, 0),
	}
}

func (r *LbsRoom) Enter(u *DBUser) {
	if len(r.Users) == 0 {
		r.Owner = u.UserID
		r.Deadline = time.Now().Add(30 * time.Minute)
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

func (r *LbsRoom) Exit(userID string) {
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

func (r *LbsRoom) Remove() {
	*r = *NewRoom(r.app, r.Platform, r.lobby, r.ID, r.Team)
}

func (r *LbsRoom) Ready(u *LbsPeer, enable uint8) {
	r.ready = enable == 1
}

func (r *LbsRoom) IsReady() bool {
	return r.ready
}
