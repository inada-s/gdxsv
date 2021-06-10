package main

import (
	"testing"
)

func Test_teamShuffle(t *testing.T) {
	type args struct {
		seed  int64
		peers []*LbsPeer
	}
	tests := []struct {
		name string
		args args
		want []uint16
	}{
		{
			name: "random shuffle seed 1",
			args: args{
				seed:  1,
				peers: []*LbsPeer{&LbsPeer{}, &LbsPeer{}, &LbsPeer{}, &LbsPeer{}},
			},
			want: []uint16{1, 1, 2, 2},
		},
		{
			name: "random shuffle seed 2",
			args: args{
				seed:  2,
				peers: []*LbsPeer{&LbsPeer{}, &LbsPeer{}, &LbsPeer{}, &LbsPeer{}},
			},
			want: []uint16{1, 2, 2, 1},
		},
		{
			name: "region friendly shuffle seed 1",
			args: args{
				seed: 1,
				peers: []*LbsPeer{
					&LbsPeer{bestRegion: "us-east1"}, &LbsPeer{bestRegion: "us-east1"},
					&LbsPeer{bestRegion: "us-west1"}, &LbsPeer{bestRegion: "us-west1"}},
			},
			want: []uint16{1, 1, 2, 2},
		},
		{
			name: "region friendly shuffle seed 2",
			args: args{
				seed: 2,
				peers: []*LbsPeer{
					&LbsPeer{bestRegion: "us-east1"}, &LbsPeer{bestRegion: "us-east1"},
					&LbsPeer{bestRegion: "us-west1"}, &LbsPeer{bestRegion: "us-west1"}},
			},
			want: []uint16{2, 2, 1, 1},
		},
		{
			name: "not region friendly shuffle",
			args: args{
				seed: 2,
				peers: []*LbsPeer{
					&LbsPeer{}, &LbsPeer{bestRegion: "us-east1"},
					&LbsPeer{bestRegion: "us-west1"}, &LbsPeer{bestRegion: "us-west1"}},
			},
			want: []uint16{1, 2, 2, 1},
		},
		{
			name: "two players",
			args: args{
				seed:  9,
				peers: []*LbsPeer{&LbsPeer{}, &LbsPeer{}},
			},
			want: []uint16{2, 1},
		},
		{
			name: "three players",
			args: args{
				seed:  2,
				peers: []*LbsPeer{&LbsPeer{}, &LbsPeer{}, &LbsPeer{}},
			},
			want: []uint16{1, 2, 2},
		},
		{
			name: "more than five players is not supported",
			args: args{
				seed:  1,
				peers: []*LbsPeer{&LbsPeer{}, &LbsPeer{}, &LbsPeer{}, &LbsPeer{}, &LbsPeer{}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			teams := teamShuffle(tt.args.seed, tt.args.peers)
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
