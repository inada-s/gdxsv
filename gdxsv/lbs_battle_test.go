package main

import (
	"net"
	"testing"
)

func Test_toIPPort(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    net.IP
		want1   uint16
		wantErr bool
	}{
		{"tcp4 addr", args{"192.168.1.10:1234"}, net.IPv4(192, 168, 1, 10), 1234, false},
		{"localhost", args{"localhost:1234"}, net.IPv4(127, 0, 0, 1), 1234, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := toIPPort(tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("toIPPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("toIPPort() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("toIPPort() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
