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

// relayTestKR is close enough to Japan that only the lowered minimum distance counts the pair.
var relayTestKR = latencies("asia-east2", 50, "asia-east1", 40, "asia-northeast1", 30, "asia-northeast2", 28, "asia-southeast1", 80, "europe-west2", 240)

func relayTestUser(info map[string]string, disk string) relayUser {
	m := map[string]string{"relay_server": "1"}
	for k, v := range info {
		m[k] = v
	}
	u, _ := newRelayUser(&LbsPeer{PlatformInfo: m, GameDisk: disk})
	return u
}

func relayTestRegions(names ...string) []int {
	var r []int
	for _, n := range names {
		r = append(r, sort.SearchStrings(relayRegionNames, n))
	}
	return r
}

func TestLbsRelay_NeededRegions(t *testing.T) {
	hkOnly := relayTestRegions("asia-east2")
	hkOsaka := relayTestRegions("asia-east2", "asia-northeast2")
	osaka := relayTestRegions("asia-northeast2")
	dc2 := func(infos ...map[string]string) []relayUser {
		var users []relayUser
		for _, info := range infos {
			users = append(users, relayTestUser(info, GameDiskDC2))
		}
		return users
	}
	tests := []struct {
		name    string
		regions []int
		users   []relayUser
		want    []string
	}{
		{"jp and hk", hkOnly, dc2(relayTestJP, relayTestHK), []string{"asia-east2"}},
		{"jp and hk among others", hkOnly, dc2(relayTestJP, relayTestJP, relayTestHK, relayTestEU), []string{"asia-east2"}},
		{"jp and hk: the closer allowed region", hkOsaka, dc2(relayTestJP, relayTestHK), []string{"asia-northeast2"}},
		{"jp only", hkOnly, dc2(relayTestJP, relayTestJP), nil},
		{"hk only", hkOnly, dc2(relayTestHK, relayTestHK), nil},
		{"jp and kr: far enough at 40 ms", osaka, dc2(relayTestJP, relayTestKR), []string{"asia-northeast2"}},
		{"jp and tw: hong kong is a detour", hkOnly, dc2(relayTestJP, relayTestTW), nil},
		{"jp and eu: too far", hkOsaka, dc2(relayTestJP, relayTestEU), nil},
		{"jp and hk on different disks", hkOnly, []relayUser{relayTestUser(relayTestJP, GameDiskDC2), relayTestUser(relayTestHK, GameDiskDC1)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for i := range relayNeededRegions(tt.users, tt.regions) {
				got = append(got, relayRegionNames[i])
			}
			sort.Strings(got)
			assertEq(t, tt.want, got)
		})
	}
}

func TestLbsRelay_NewRelayUser(t *testing.T) {
	if _, ok := newRelayUser(&LbsPeer{PlatformInfo: relayTestJP}); ok {
		t.Fatal("a client without relay support was counted")
	}
	if _, ok := newRelayUser(&LbsPeer{PlatformInfo: map[string]string{"relay_server": "1"}}); ok {
		t.Fatal("a client without latencies was counted")
	}
	u := relayTestUser(relayTestHK, GameDiskDC2)
	assertEq(t, uint16(9), u.lat[relayTestRegions("asia-east2")[0]])
}

// The region decision runs outside the event loop and comes back to mark the region as needed.
func TestLbsRelay_UpdateMarksNeeded(t *testing.T) {
	oldRegions := conf.RelayRegions
	conf.RelayRegions = "asia-east2"
	defer func() { conf.RelayRegions = oldRegions }()

	lbsAddr := freeTCPAddr(t)
	lbs := NewLbs()
	defer lbs.Quit()
	go lbs.ListenAndServe(lbsAddr)
	dialWithRetry(t, lbsAddr, 5*time.Second).Close()

	withRelay := func(info map[string]string) map[string]string {
		m := map[string]string{"relay_server": "1"}
		for k, v := range info {
			m[k] = v
		}
		return m
	}
	lbs.Locked(func(lbs *Lbs) {
		lbs.userPeers["jp"] = &LbsPeer{PlatformInfo: withRelay(relayTestJP), GameDisk: GameDiskDC2}
		lbs.userPeers["hk"] = &LbsPeer{PlatformInfo: withRelay(relayTestHK), GameDisk: GameDiskDC2}
		lbs.updateRelay(time.Now())
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		marked := false
		lbs.Locked(func(lbs *Lbs) { _, marked = lbs.relayLastNeeded["asia-east2"] })
		if marked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("asia-east2 was not marked as needed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	lbs.Locked(func(lbs *Lbs) {
		delete(lbs.userPeers, "jp")
		delete(lbs.userPeers, "hk")
	})
}

func BenchmarkLbsRelay_NeededRegions(b *testing.B) {
	for _, n := range []int{100, 400, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			users := make([]relayUser, n)
			for i := range users {
				// Every pair is checked in full: all of them in Japan, none needs a relay.
				users[i] = relayTestUser(relayTestJP, GameDiskDC2)
			}
			regions := relayTestRegions("asia-east2")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				relayNeededRegions(users, regions)
			}
		})
	}
}

func TestLbsRelay_MatchRelays(t *testing.T) {
	relay := func(region string) *LbsPeer {
		return &LbsPeer{relayStatus: &RelayStatus{Region: region, PublicAddr: region + ":9879"}}
	}
	regionsOf := func(relays []*relayEndpoint) []string {
		var r []string
		for _, e := range relays {
			r = append(r, e.region)
		}
		return r
	}
	local := &relayEndpoint{region: localRelayRegion, publicAddr: "127.0.0.1:9879"}
	lbs := &Lbs{
		relayPeers: map[string]*LbsPeer{"hk": relay("asia-east2"), "sg": relay("asia-southeast1")},
		localRelay: local,
	}
	jp, hk, sg := &LbsPeer{PlatformInfo: relayTestJP}, &LbsPeer{PlatformInfo: relayTestHK}, &LbsPeer{PlatformInfo: relayTestSG}

	// Best estimate first, the lobby's own relay last.
	assertEq(t, []string{"asia-east2", "asia-southeast1", "lbs"}, regionsOf(lbs.matchRelays([]*LbsPeer{jp, jp, hk, hk})))
	assertEq(t, []string{"asia-southeast1", "asia-east2", "lbs"}, regionsOf(lbs.matchRelays([]*LbsPeer{sg, sg, hk, sg})))
	// Only the lobby's own relay while no VM runs.
	assertEq(t, []string{"lbs"}, regionsOf((&Lbs{relayPeers: map[string]*LbsPeer{}, localRelay: local}).matchRelays([]*LbsPeer{jp, hk})))
	assertEq(t, 0, len((&Lbs{relayPeers: map[string]*LbsPeer{}}).matchRelays([]*LbsPeer{jp, hk})))

	// Capped at what clients accept.
	for _, r := range []string{"asia-east1", "asia-northeast1", "asia-northeast2"} {
		lbs.relayPeers[r] = relay(r)
	}
	assertEq(t, maxRelaysPerMatch, len(lbs.matchRelays([]*LbsPeer{jp, hk})))
}

func TestLbsRelay_IPv6(t *testing.T) {
	assertEq(t, "2001:db8::1", relayIPv6("[2001:db8::1]:9879", 9879))
	assertEq(t, "", relayIPv6("", 9879))
	assertEq(t, "", relayIPv6("[2001:db8::1]:9870", 9879)) // must share the IPv4 port
	assertEq(t, "", relayIPv6("192.0.2.1:9879", 9879))
	assertEq(t, "", relayIPv6("relay.example:9879", 9879))
}

func TestLbsRelay_LocalRelaySession(t *testing.T) {
	relay := NewRelay()
	ep := &relayEndpoint{region: localRelayRegion, publicAddr: "203.0.113.5:9879", register: relay.RegisterSession}
	server, err := openRelaySession(ep, 555)
	must(t, err)
	assertEq(t, "203.0.113.5", server.GetIp())
	assertEq(t, int32(9879), server.GetPort())
	assertEq(t, "lbs", server.GetRegion())
	assertEq(t, server.GetToken(), relay.sessions[555].token)
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

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(t, err)
	defer udp.Close()
	relay := NewRelay()
	go relay.Serve(udp)

	done := make(chan error, 1)
	go func() {
		var lastConnected time.Time
		status := RelayStatus{Region: "asia-east2", PublicAddr: udp.LocalAddr().String(), PublicAddr6: "[2001:db8::7]:" +
			strconv.Itoa(udp.LocalAddr().(*net.UDPAddr).Port)}
		done <- relay.DialAndSyncWithLbs(lbsAddr, status, &lastConnected)
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
		GetIp6() string
		GetPort() int32
		GetToken() uint64
	}
	lbs.Locked(func(lbs *Lbs) {
		s, err := openRelaySession(peerRelayEndpoint(lbs.findRelay("asia-east2")), 777)
		must(t, err)
		server = s
	})
	assertEq(t, "127.0.0.1", server.GetIp())
	assertEq(t, "2001:db8::7", server.GetIp6())
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
