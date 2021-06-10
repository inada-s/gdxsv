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
