package main

import (
	"testing"
)

func TestLbsRoom_NotifyRoomEvent_NilPeer(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	lobby := &LbsLobby{
		app:        lbs,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
	}
	room := NewRoom(lbs, PlatformEmuX8664, GameDiskPS2, lobby, 1, TeamRenpo)

	// Add a user to the room but do NOT register them in lbs.userPeers,
	// so FindPeer will return nil.
	room.Users = append(room.Users, &DBUser{UserID: "GHOST_USER"})

	// This should not panic even though FindPeer returns nil.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NotifyRoomEvent panicked with nil peer: %v", r)
			}
		}()
		room.NotifyRoomEvent("TEST", "test message")
	}()
}

func TestLbsRoom_Enter(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	lobby := &LbsLobby{
		app:        lbs,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
	}
	room := NewRoom(lbs, PlatformEmuX8664, GameDiskPS2, lobby, 1, TeamRenpo)

	u1 := &DBUser{UserID: "U1", Name: "User1"}
	u2 := &DBUser{UserID: "U2", Name: "User2"}

	// Enter first user
	room.Enter(u1)
	assertEq(t, 1, len(room.Users))
	assertEq(t, "U1", room.Owner)
	assertEq(t, byte(RoomStateRecruiting), room.Status)

	// Duplicate entry should not add
	room.Enter(u1)
	assertEq(t, 1, len(room.Users))

	// Enter second user fills the room (MaxPlayer=2)
	room.Enter(u2)
	assertEq(t, 2, len(room.Users))
	assertEq(t, byte(RoomStateFull), room.Status)
}

func TestLbsRoom_Exit(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	lobby := &LbsLobby{
		app:        lbs,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
	}
	room := NewRoom(lbs, PlatformEmuX8664, GameDiskPS2, lobby, 1, TeamRenpo)

	u1 := &DBUser{UserID: "U1", Name: "User1"}
	u2 := &DBUser{UserID: "U2", Name: "User2"}
	room.Enter(u1)
	room.Enter(u2)

	// Exit non-owner
	room.Exit("U2")
	assertEq(t, 1, len(room.Users))
	assertEq(t, byte(RoomStateRecruiting), room.Status)

	// Exit owner (last user) should trigger Remove
	room.Exit("U1")
	assertEq(t, 0, len(room.Users))
	assertEq(t, byte(RoomStateEmpty), room.Status)
	assertEq(t, "", room.Owner)
}

func TestLbsRoom_Remove(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	lobby := &LbsLobby{
		app:        lbs,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: make([]string, 0),
	}
	room := NewRoom(lbs, PlatformEmuX8664, GameDiskPS2, lobby, 1, TeamRenpo)
	room.Name = "TestRoom"
	room.Status = RoomStateRecruiting
	room.Owner = "U1"
	room.Users = append(room.Users, &DBUser{UserID: "U1"})

	room.Remove()

	assertEq(t, 0, len(room.Users))
	assertEq(t, byte(RoomStateEmpty), room.Status)
	assertEq(t, "", room.Owner)
	assertEq(t, "", room.Name)
}
