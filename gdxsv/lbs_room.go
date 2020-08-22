package main

import (
	"fmt"
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
		r.lobby.NotifyLobbyEvent("GO ROOM", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
		r.NotifyRoomEvent("ENTER ROOM", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
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
			r.NotifyRoomEvent("EXIT ROOM", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
			r.lobby.NotifyLobbyEvent("RETURN", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
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
	for _, u := range r.Users {
		r.NotifyRoomEvent("EXIT ROOM", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
		r.lobby.NotifyLobbyEvent("RETURN", fmt.Sprintf("【%v】%v", u.UserID, u.Name))
	}
	*r = *NewRoom(r.app, r.Platform, r.lobby, r.ID, r.Team)
}

func (r *LbsRoom) Ready(u *LbsPeer, enable uint8) {
	r.ready = enable == 1
}

func (r *LbsRoom) IsReady() bool {
	return r.ready
}

func (r *LbsRoom) NotifyRoomEvent(kind string, text string) {
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
	for _, u := range r.Users {
		peer := r.app.FindPeer(u.UserID)
		if peer.Room == nil {
			continue
		}
		if peer.Room.ID != r.ID {
			continue
		}
		peer.SendMessage(msg)
	}
}
