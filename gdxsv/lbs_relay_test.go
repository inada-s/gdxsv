package main

import (
	"net"
	"sort"
	"strconv"
	"testing"
	"time"
)

func latencies(kv ...interface{}) map[string]string {
	m := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = strconv.Itoa(kv[i+1].(int))
	}
	return m
}

// GCP latencies close to what real players report.
var (
	relayTestJP = latencies("asia-east2", 67, "asia-east1", 53, "asia-northeast1", 26, "asia-northeast2", 18, "asia-southeast1", 99, "europe-west2", 230)
	relayTestHK = latencies("asia-east2", 9, "asia-east1", 35, "asia-northeast1", 67, "asia-northeast2", 56, "asia-southeast1", 42, "europe-west2", 190)
	relayTestTW = latencies("asia-east2", 25, "asia-east1", 8, "asia-northeast1", 45, "asia-northeast2", 40, "asia-southeast1", 50, "europe-west2", 220)
	relayTestSG = latencies("asia-east2", 35, "asia-east1", 50, "asia-northeast1", 70, "asia-northeast2", 75, "asia-southeast1", 5, "europe-west2", 160)
	relayTestEU = latencies("asia-east2", 190, "asia-east1", 230, "asia-northeast1", 230, "asia-northeast2", 240, "asia-southeast1", 160, "europe-west2", 10)
)

func TestLbsRelay_NeededRegions(t *testing.T) {
	hkOnly := []string{"asia-east2"}
	hkOsaka := []string{"asia-east2", "asia-northeast2"}
	tests := []struct {
		name    string
		regions []string
		peers   []map[string]string
		want    []string
	}{
		{"jp and hk", hkOnly, []map[string]string{relayTestJP, relayTestHK}, []string{"asia-east2"}},
		{"jp and hk among others", hkOnly, []map[string]string{relayTestJP, relayTestJP, relayTestHK, {}}, []string{"asia-east2"}},
		{"jp and hk: the closer allowed region", hkOsaka, []map[string]string{relayTestJP, relayTestHK}, []string{"asia-northeast2"}},
		{"jp only", hkOnly, []map[string]string{relayTestJP, relayTestJP}, nil},
		{"hk only", hkOnly, []map[string]string{relayTestHK, relayTestHK}, nil},
		{"jp and tw: hong kong is a detour", hkOnly, []map[string]string{relayTestJP, relayTestTW}, nil},
		{"jp and eu: too far", hkOsaka, []map[string]string{relayTestJP, relayTestEU}, nil},
		{"no latency info", hkOsaka, []map[string]string{{}, {}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbs := &Lbs{userPeers: map[string]*LbsPeer{}}
			for i, info := range tt.peers {
				lbs.userPeers[strconv.Itoa(i)] = &LbsPeer{PlatformInfo: info}
			}
			var got []string
			for r := range lbs.relayNeededRegions(tt.regions) {
				got = append(got, r)
			}
			sort.Strings(got)
			assertEq(t, tt.want, got)
		})
	}
}

func TestLbsRelay_SelectRelay(t *testing.T) {
	relay := func(region string) *LbsPeer {
		return &LbsPeer{relayStatus: &RelayStatus{Region: region, PublicAddr: region + ":9879"}}
	}
	hk, sg := relay("asia-east2"), relay("asia-southeast1")
	lbs := &Lbs{relayPeers: map[string]*LbsPeer{"hk": hk, "sg": sg}}
	jp1, jp2 := &LbsPeer{PlatformInfo: relayTestJP}, &LbsPeer{PlatformInfo: relayTestJP}
	hk1, sg1 := &LbsPeer{PlatformInfo: relayTestHK}, &LbsPeer{PlatformInfo: relayTestSG}
	if lbs.selectRelay([]*LbsPeer{jp1, jp2, hk1, hk1}) != hk {
		t.Fatal("jp vs hk should use the hong kong relay")
	}
	if lbs.selectRelay([]*LbsPeer{sg1, sg1, hk1, sg1}) != sg {
		t.Fatal("sg vs hk should use the singapore relay")
	}
	if (&Lbs{relayPeers: map[string]*LbsPeer{}}).selectRelay([]*LbsPeer{jp1, hk1}) != nil {
		t.Fatal("no relay to select")
	}
}

func TestLbsRelay_AllSupportRelay(t *testing.T) {
	yes := &LbsPeer{PlatformInfo: map[string]string{"relay_server": "1"}}
	no := &LbsPeer{PlatformInfo: map[string]string{}}
	assertEq(t, true, allSupportRelay([]*LbsPeer{yes, yes}))
	assertEq(t, false, allSupportRelay([]*LbsPeer{yes, no}))
}

// A relay registers with a real lobby over TCP, receives a session the lobby opens,
// and is shut down once the lobby no longer needs it and it is idle.
func TestLbsRelay_Integration(t *testing.T) {
	oldRegions := conf.RelayRegions
	conf.RelayRegions = "asia-east2"
	defer func() { conf.RelayRegions = oldRegions }()

	lbsAddr := freeTCPAddr(t)
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.ListenAndServe(lbsAddr)
	dialWithRetry(t, lbsAddr, 5*time.Second).Close()

	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	must(t, err)
	defer udp.Close()
	relay := NewRelay()
	go relay.Serve(udp)

	done := make(chan error, 1)
	go func() {
		var lastConnected time.Time
		done <- relay.DialAndSyncWithLbs(lbsAddr, udp.LocalAddr().String(), "asia-east2", &lastConnected)
	}()

	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	relaySessions := func() int {
		n := 0
		lbs.Locked(func(lbs *Lbs) {
			if p := lbs.findRelay("asia-east2"); p != nil {
				n = p.relayStatus.Sessions
			}
		})
		return n
	}

	waitFor("relay registration", func() bool {
		found := false
		lbs.Locked(func(lbs *Lbs) { found = lbs.findRelay("asia-east2") != nil })
		return found
	})

	var server interface {
		GetIp() string
		GetPort() int32
		GetToken() uint64
	}
	lbs.Locked(func(lbs *Lbs) {
		s, err := openRelaySession(lbs.findRelay("asia-east2"), 777)
		must(t, err)
		server = s
	})
	assertEq(t, "127.0.0.1", server.GetIp())
	assertEq(t, int32(udp.LocalAddr().(*net.UDPAddr).Port), server.GetPort())
	waitFor("session on the relay", func() bool {
		relay.mtx.Lock()
		defer relay.mtx.Unlock()
		s, ok := relay.sessions[777]
		return ok && s.token == server.GetToken()
	})

	// Unneeded for long, but a match is still using it: keep it.
	waitFor("status with a session", func() bool { return relaySessions() == 1 })
	lbs.Locked(func(lbs *Lbs) {
		lbs.relayLastNeeded["asia-east2"] = time.Now().Add(-relayIdleShutdown - time.Minute)
		lbs.updateRelay(time.Now())
	})
	select {
	case err := <-done:
		t.Fatalf("relay stopped while in use: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	// The match ended.
	relay.mtx.Lock()
	relay.now = func() time.Time { return time.Now().Add(relayMaxLifetime + time.Minute) }
	relay.mtx.Unlock()
	relay.RemoveStaleSessions()
	waitFor("idle status", func() bool { return relaySessions() == 0 })
	lbs.Locked(func(lbs *Lbs) { lbs.updateRelay(time.Now()) })
	select {
	case err := <-done:
		assertEq(t, ErrRelayShutdown, err)
	case <-time.After(5 * time.Second):
		t.Fatal("relay was not shut down")
	}
}
