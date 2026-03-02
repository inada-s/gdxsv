package main

import (
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func Test_teamShuffle(t *testing.T) {
	makePeer := func(userID string) *LbsPeer {
		return &LbsPeer{
			DBUser: DBUser{
				UserID: userID,
			},
		}
	}

	makePeerWithRegion := func(userID string, region string) *LbsPeer {
		p := makePeer(userID)
		p.bestRegion = region
		return p
	}

	type args struct {
		seed           int64
		mode           int
		peers          []*LbsPeer
		lastTeamUserID []string
	}
	tests := []struct {
		name string
		args args
		want []uint16
	}{
		{
			name: "random shuffle seed 1",
			args: args{
				seed: 1,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
					makePeer("3"),
					makePeer("4"),
				},
			},
			want: []uint16{1, 1, 2, 2},
		},
		{
			name: "random shuffle seed 2",
			args: args{
				seed: 2,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
					makePeer("3"),
					makePeer("4"),
				},
			},
			want: []uint16{1, 2, 2, 1},
		},
		{
			name: "random shuffle avoid same team",
			args: args{
				seed: 1,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
					makePeer("3"),
					makePeer("4"),
				},
				lastTeamUserID: []string{"2", "1", "", ""},
			},
			want: []uint16{1, 2, 2, 1},
		},
		{
			name: "region friendly shuffle seed 1",
			args: args{
				seed: 1,
				mode: TeamShuffleRegionFriendly,
				peers: []*LbsPeer{
					makePeerWithRegion("1", "us-east1"),
					makePeerWithRegion("2", "us-east1"),
					makePeerWithRegion("3", "us-west1"),
					makePeerWithRegion("4", "us-west1"),
				},
			},
			want: []uint16{1, 1, 2, 2},
		},
		{
			name: "region friendly shuffle seed 2",
			args: args{
				seed: 2,
				mode: TeamShuffleRegionFriendly,
				peers: []*LbsPeer{
					makePeerWithRegion("1", "us-east1"),
					makePeerWithRegion("2", "us-east1"),
					makePeerWithRegion("3", "us-west1"),
					makePeerWithRegion("4", "us-west1"),
				},
			},
			want: []uint16{2, 2, 1, 1},
		},
		{
			name: "not region friendly shuffle",
			args: args{
				seed: 2,
				mode: TeamShuffleRegionFriendly,
				peers: []*LbsPeer{
					makePeerWithRegion("1", "asia-east1"),
					makePeerWithRegion("2", "us-east1"),
					makePeerWithRegion("3", "us-west1"),
					makePeerWithRegion("4", "us-west1"),
				},
			},
			want: []uint16{1, 2, 2, 1},
		},
		{
			name: "two players",
			args: args{
				seed: 9,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
				},
			},
			want: []uint16{2, 1},
		},
		{
			name: "three players",
			args: args{
				seed: 2,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
					makePeer("3"),
				},
			},
			want: []uint16{1, 2, 2},
		},
		{
			name: "more than five players is not supported",
			args: args{
				seed: 1,
				mode: TeamShuffleDefault,
				peers: []*LbsPeer{
					makePeer("1"),
					makePeer("2"),
					makePeer("3"),
					makePeer("4"),
					makePeer("5"),
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.lastTeamUserID != nil {
				used := map[string]bool{}
				for i, peer := range tt.args.peers {
					if used[peer.UserID] {
						continue
					}

					team := TeamRenpo
					if 2 <= len(used) {
						team = TeamZeon
					}

					used[peer.UserID] = true
					mustInsertBattleRecord(BattleRecord{
						BattleCode: "123456789",
						UserID:     peer.UserID,
						Team:       team,
					})

					teamUserID := tt.args.lastTeamUserID[i]
					if teamUserID != "" {
						used[teamUserID] = true
						mustInsertBattleRecord(BattleRecord{
							BattleCode: "123456789",
							UserID:     teamUserID,
							Team:       team,
						})
					}
				}
			}
			teams := teamShuffle(tt.args.seed, tt.args.peers, 2)
			assertEq(t, tt.want, teams)
		})
	}
}

func TestLbsLobby_buildLobbyReminderMessages(t *testing.T) {
	tests := []struct {
		name    string
		mstring string
		want    []string
		insert  bool
	}{
		{
			name:    "3 lines",
			mstring: "aaa\nbbb\nccc",
			want:    []string{"aaa", "bbb", "ccc"},
			insert:  true,
		},
		{
			name:    "trim line break",
			mstring: "\naaa\nbbb\nccc\n",
			want:    []string{"aaa", "bbb", "ccc"},
			insert:  true,
		},
		{
			name:    "allow padding space",
			mstring: "\n  aaa  \nbbb  \n  ccc\n",
			want:    []string{"  aaa  ", "bbb  ", "  ccc"},
			insert:  true,
		},
		{
			name:    "no reminder text set",
			mstring: "aaa",
			want:    nil,
			insert:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reminder := tt.name
			if tt.insert {
				mustInsertMString(reminder, tt.mstring)
			}
			l := &LbsLobby{
				LobbySetting: LobbySetting{
					Reminder: reminder,
				},
			}
			var chats []string
			for _, msg := range l.buildLobbyReminderMessages() {
				AssertMsg(t, &LbsMessage{Command: lbsChatMessage}, msg)
				r := msg.Reader()
				r.ReadString() // id
				r.ReadString() // name
				chats = append(chats, r.ReadString())
			}
			assertEq(t, tt.want, chats)
		})
	}
}

func TestLbsLobby_findBestGCPRegion(t *testing.T) {
	// Save and restore gcpLocationName
	origLocationName := gcpLocationName
	defer func() { gcpLocationName = origLocationName }()

	// Use a small set of regions for testing
	gcpLocationName = map[string]string{
		"asia-northeast1": "Tokyo",
		"us-west1":        "Oregon",
		"europe-west1":    "Belgium",
	}

	l := &LbsLobby{}

	t.Run("selects region with minimum max RTT", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"asia-northeast1": "50",
				"us-west1":        "150",
				"europe-west1":    "200",
			}},
			{PlatformInfo: map[string]string{
				"asia-northeast1": "60",
				"us-west1":        "120",
				"europe-west1":    "180",
			}},
		}
		region, err := l.findBestGCPRegion(peers)
		must(t, err)
		// asia-northeast1: max(50,60)=60, us-west1: max(150,120)=150, europe-west1: max(200,180)=200
		assertEq(t, "asia-northeast1", region)
	})

	t.Run("all regions RTT=999 returns error", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"asia-northeast1": "999",
				"us-west1":        "999",
				"europe-west1":    "999",
			}},
		}
		_, err := l.findBestGCPRegion(peers)
		if err == nil {
			t.Error("expected error when all regions have RTT=999")
		}
	})

	t.Run("RTT=0 treated as 999", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"asia-northeast1": "0",
				"us-west1":        "100",
				"europe-west1":    "200",
			}},
		}
		region, err := l.findBestGCPRegion(peers)
		must(t, err)
		// asia-northeast1: 0 → 999, us-west1: 100, europe-west1: 200
		assertEq(t, "us-west1", region)
	})

	t.Run("unparseable RTT treated as 999", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"asia-northeast1": "bad",
				"us-west1":        "80",
				"europe-west1":    "200",
			}},
		}
		region, err := l.findBestGCPRegion(peers)
		must(t, err)
		assertEq(t, "us-west1", region)
	})

	t.Run("missing region key treated as 999", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"us-west1": "50",
			}},
		}
		region, err := l.findBestGCPRegion(peers)
		must(t, err)
		assertEq(t, "us-west1", region)
	})

	t.Run("all regions RTT=0 returns error", func(t *testing.T) {
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{
				"asia-northeast1": "0",
				"us-west1":        "0",
				"europe-west1":    "0",
			}},
		}
		_, err := l.findBestGCPRegion(peers)
		if err == nil {
			t.Error("expected error when all regions have RTT=0")
		}
	})

	t.Run("no regions returns error", func(t *testing.T) {
		gcpLocationName = map[string]string{}
		peers := []*LbsPeer{
			{PlatformInfo: map[string]string{}},
		}
		_, err := l.findBestGCPRegion(peers)
		if err == nil {
			t.Error("expected error when no regions available")
		}
	})
}

func TestLbsLobby_sendLobbyChat_NilPeer(t *testing.T) {
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

	// Add a user to lobby.Users but NOT to lbs.userPeers,
	// so FindPeer returns nil.
	lobby.Users["GHOST_USER"] = &DBUser{UserID: "GHOST_USER"}

	// This should not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("sendLobbyChat panicked with nil peer: %v", r)
			}
		}()
		lobby.sendLobbyChat("SENDER", "SenderName", "hello")
	}()
}

func TestLbsLobby_NotifyLobbyEvent_NilPeer(t *testing.T) {
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

	// Add a user to lobby.Users but NOT to lbs.userPeers.
	lobby.Users["GHOST_USER"] = &DBUser{UserID: "GHOST_USER"}

	// This should not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NotifyLobbyEvent panicked with nil peer: %v", r)
			}
		}()
		lobby.NotifyLobbyEvent("TEST", "test message")
	}()
}

func TestLbsLobby_Exit_RemovesEntryUser(t *testing.T) {
	lobby := &LbsLobby{
		Users:      make(map[string]*DBUser),
		EntryUsers: []string{"A", "B", "C"},
	}
	lobby.Users["B"] = &DBUser{UserID: "B"}

	lobby.Exit("B")

	assertEq(t, []string{"A", "C"}, lobby.EntryUsers)
	if _, ok := lobby.Users["B"]; ok {
		t.Error("expected user B to be removed from Users map")
	}
}

func TestLbsLobby_Exit_NonEntryUser(t *testing.T) {
	lobby := &LbsLobby{
		Users:      make(map[string]*DBUser),
		EntryUsers: []string{"A", "C"},
	}
	lobby.Users["B"] = &DBUser{UserID: "B"}

	lobby.Exit("B")

	assertEq(t, []string{"A", "C"}, lobby.EntryUsers)
}

func TestLbsLobby_EntryCancel(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	lobby := &LbsLobby{
		app:        lbs,
		Users:      make(map[string]*DBUser),
		RenpoRooms: make(map[uint16]*LbsRoom),
		ZeonRooms:  make(map[uint16]*LbsRoom),
		EntryUsers: []string{"A", "B", "C"},
	}

	peer := &LbsPeer{
		DBUser:       DBUser{UserID: "B"},
		Team:         TeamRenpo,
		app:          lbs,
		PlatformInfo: map[string]string{},
		logger:       zap.NewNop(),
		chWrite:      make(chan bool, 1),
	}
	lbs.Locked(func(l *Lbs) {
		l.userPeers["B"] = peer
	})
	defer lbs.Locked(func(l *Lbs) {
		delete(l.userPeers, "B")
	})
	lobby.Users["B"] = &peer.DBUser

	lobby.EntryCancel(peer)

	assertEq(t, []string{"A", "C"}, lobby.EntryUsers)
}

func TestLbsLobby_EntryPicked(t *testing.T) {
	lobby := &LbsLobby{
		EntryUsers: []string{"A", "B", "C"},
	}

	peer := &LbsPeer{DBUser: DBUser{UserID: "B"}}
	lobby.EntryPicked(peer)

	assertEq(t, []string{"A", "C"}, lobby.EntryUsers)
}

func TestLbsLobby_canStartBattle(t *testing.T) {
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.eventLoop()

	makePeer := func(userID string, team uint16) *LbsPeer {
		p := &LbsPeer{
			DBUser:       DBUser{UserID: userID},
			Team:         team,
			PlatformInfo: map[string]string{},
		}
		return p
	}

	tests := []struct {
		name         string
		teamShuffle  int
		entryPeers   []*LbsPeer
		wantCanStart bool
	}{
		{
			name:        "no shuffle: 2R+2Z can start",
			teamShuffle: 0,
			entryPeers: []*LbsPeer{
				makePeer("R1", TeamRenpo),
				makePeer("R2", TeamRenpo),
				makePeer("Z1", TeamZeon),
				makePeer("Z2", TeamZeon),
			},
			wantCanStart: true,
		},
		{
			name:        "no shuffle: 1R+2Z cannot start",
			teamShuffle: 0,
			entryPeers: []*LbsPeer{
				makePeer("R1", TeamRenpo),
				makePeer("Z1", TeamZeon),
				makePeer("Z2", TeamZeon),
			},
			wantCanStart: false,
		},
		{
			name:        "no shuffle: 2R+1Z cannot start",
			teamShuffle: 0,
			entryPeers: []*LbsPeer{
				makePeer("R1", TeamRenpo),
				makePeer("R2", TeamRenpo),
				makePeer("Z1", TeamZeon),
			},
			wantCanStart: false,
		},
		{
			name:        "no shuffle: 0 users cannot start",
			teamShuffle: 0,
			entryPeers:  []*LbsPeer{},
			wantCanStart: false,
		},
		{
			name:        "shuffle: 4 users can start",
			teamShuffle: 1,
			entryPeers: []*LbsPeer{
				makePeer("A1", TeamRenpo),
				makePeer("A2", TeamRenpo),
				makePeer("A3", TeamZeon),
				makePeer("A4", TeamZeon),
			},
			wantCanStart: true,
		},
		{
			name:        "shuffle: 3 users cannot start",
			teamShuffle: 1,
			entryPeers: []*LbsPeer{
				makePeer("A1", TeamRenpo),
				makePeer("A2", TeamRenpo),
				makePeer("A3", TeamZeon),
			},
			wantCanStart: false,
		},
		{
			name:        "shuffle: 4 same team can start",
			teamShuffle: 1,
			entryPeers: []*LbsPeer{
				makePeer("B1", TeamRenpo),
				makePeer("B2", TeamRenpo),
				makePeer("B3", TeamRenpo),
				makePeer("B4", TeamRenpo),
			},
			wantCanStart: true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lobbyID := uint16(100 + i)

			lobby := &LbsLobby{
				app:          lbs,
				ID:           lobbyID,
				Users:        make(map[string]*DBUser),
				RenpoRooms:   make(map[uint16]*LbsRoom),
				ZeonRooms:    make(map[uint16]*LbsRoom),
				EntryUsers:   make([]string, 0),
				LobbySetting: LobbySetting{TeamShuffle: tt.teamShuffle},
			}

			for _, p := range tt.entryPeers {
				p.app = lbs
				userID := fmt.Sprintf("CSB_%d_%s", i, p.UserID)
				p.UserID = userID
				lbs.Locked(func(l *Lbs) {
					l.userPeers[userID] = p
				})
				defer lbs.Locked(func(l *Lbs) {
					delete(l.userPeers, userID)
				})
				lobby.Users[userID] = &p.DBUser
				lobby.EntryUsers = append(lobby.EntryUsers, userID)
			}

			got := lobby.canStartBattle()
			assertEq(t, tt.wantCanStart, got)
		})
	}
}
