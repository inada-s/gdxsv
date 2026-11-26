package main

import (
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"gdxsv/gdxsv/proto"

	pb "google.golang.org/protobuf/proto"
)

func latencies(kv ...interface{}) map[string]string {
	m := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = strconv.Itoa(kv[i+1].(int))
	}
	return m
}

// GCP latencies close to what real players report, for ordering relays.
var (
	relayTestJP = latencies("asia-east2", 67, "asia-east1", 53, "asia-northeast1", 26, "asia-northeast2", 18, "asia-southeast1", 99, "europe-west2", 230)
	relayTestHK = latencies("asia-east2", 9, "asia-east1", 35, "asia-northeast1", 67, "asia-northeast2", 56, "asia-southeast1", 42, "europe-west2", 190)
	relayTestSG = latencies("asia-east2", 35, "asia-east1", 50, "asia-northeast1", 70, "asia-northeast2", 75, "asia-southeast1", 5, "europe-west2", 160)
)

func TestLbsRelay_Needed(t *testing.T) {
	user := func(bestRegion string) *LbsPeer {
		return &LbsPeer{PlatformInfo: map[string]string{"relay_server": "1"}, bestRegion: bestRegion}
	}
	old := &LbsPeer{PlatformInfo: map[string]string{}, bestRegion: "asia-east2"}
	tests := []struct {
		name  string
		peers []*LbsPeer
		want  bool
	}{
		{"japan and hong kong", []*LbsPeer{user("asia-northeast1"), user("asia-northeast2"), user("asia-east2"), user("asia-east2")}, true},
		{"japan only", []*LbsPeer{user("asia-northeast1"), user("asia-northeast2"), user("asia-northeast1"), user("asia-northeast2")}, false},
		{"japan and korea: one group", []*LbsPeer{user("asia-northeast1"), user("asia-northeast1"), user("asia-northeast3"), user("asia-northeast3")}, false},
		{"hong kong and taiwan: one group", []*LbsPeer{user("asia-east2"), user("asia-east2"), user("asia-east1"), user("asia-east1")}, false},
		{"too few players", []*LbsPeer{user("asia-northeast1"), user("asia-east2"), user("asia-east2")}, false},
		{"older clients don't count", []*LbsPeer{user("asia-northeast1"), user("asia-northeast1"), user("asia-northeast2"), old}, false},
		{"unknown regions don't count", []*LbsPeer{user("asia-northeast1"), user(""), user(""), user("")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbs := &Lbs{userPeers: map[string]*LbsPeer{}}
			for i, p := range tt.peers {
				lbs.userPeers[strconv.Itoa(i)] = p
			}
			assertEq(t, tt.want, lbs.relayNeeded())
		})
	}
}

func TestLbsRelay_UpdateStartsEveryRegion(t *testing.T) {
	oldRegions := conf.RelayRegions
	conf.RelayRegions = "asia-east2, asia-northeast2"
	defer func() { conf.RelayRegions = oldRegions }()

	lbs := &Lbs{userPeers: map[string]*LbsPeer{}, relayPeers: map[string]*LbsPeer{}, relayLastNeeded: map[string]time.Time{}}
	for i, r := range []string{"asia-northeast1", "asia-northeast2", "asia-east2", "asia-east2"} {
		lbs.userPeers[strconv.Itoa(i)] = &LbsPeer{PlatformInfo: map[string]string{"relay_server": "1"}, bestRegion: r}
	}
	now := time.Now()
	lbs.updateRelay(now)
	assertEq(t, map[string]time.Time{"asia-east2": now, "asia-northeast2": now}, lbs.relayLastNeeded)
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

func TestLbsRelay_StatusNeedsSecret(t *testing.T) {
	oldSecret := conf.RelaySecret
	defer func() { conf.RelaySecret = oldSecret }()

	status := func(secret string) *LbsMessage {
		body, err := json.Marshal(RelayStatus{Region: "asia-east2", PublicAddr: "192.0.2.1:9879", Secret: secret})
		must(t, err)
		return NewServerNotice(lbsExtRelayStatus).Writer().WriteBytes(body).Msg()
	}
	tests := []struct {
		name       string
		lbsSecret  string
		sentSecret string
		accepted   bool
	}{
		{"right secret", "s3cret", "s3cret", true},
		{"wrong secret", "s3cret", "guess", false},
		{"no secret sent", "s3cret", "", false},
		{"lobby has no secret", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf.RelaySecret = Secret(tt.lbsSecret)
			lbs := &Lbs{relayPeers: map[string]*LbsPeer{}, relayLastNeeded: map[string]time.Time{}}
			p := &LbsPeer{app: lbs, logger: logger}
			defaultLbsHandlers[lbsExtRelayStatus](p, status(tt.sentSecret))
			assertEq(t, tt.accepted, lbs.relayPeers["192.0.2.1:9879"] != nil)
			if tt.accepted {
				assertEq(t, "", p.relayStatus.Secret)
			}
		})
	}
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

func TestLbsRelay_MinRelayLevel(t *testing.T) {
	peer := func(v string) *LbsPeer {
		info := map[string]string{}
		if v != "" {
			info["relay_server"] = v
		}
		return &LbsPeer{PlatformInfo: info}
	}
	tests := []struct {
		name   string
		levels []string
		want   int
	}{
		{"no participants", nil, 0},
		{"all 1", []string{"1", "1"}, 1},
		{"all 2", []string{"2", "2", "2", "2"}, 2},
		{"2 with 1", []string{"2", "1", "2", "2"}, 1},
		{"2 with none", []string{"2", "", "2", "2"}, 0},
		{"future level", []string{"3", "2"}, 2},
		{"garbage", []string{"2", "x"}, 0},
		{"negative", []string{"2", "-1"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ps []*LbsPeer
			for _, v := range tt.levels {
				ps = append(ps, peer(v))
			}
			assertEq(t, tt.want, minRelayLevel(ps))
		})
	}
}

// A relay registers with a real lobby over TCP, receives a session the lobby opens,
// and is shut down once the lobby no longer needs it and it is idle.
func TestLbsRelay_Integration(t *testing.T) {
	oldRegions, oldSecret := conf.RelayRegions, conf.RelaySecret
	conf.RelayRegions, conf.RelaySecret = "asia-east2", "s3cret"
	defer func() { conf.RelayRegions, conf.RelaySecret = oldRegions, oldSecret }()

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
			strconv.Itoa(udp.LocalAddr().(*net.UDPAddr).Port), Secret: "s3cret"}
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

// The match tells the players each one's relay level, along with the relays.
func TestLbsRelay_MatchingRelayLevels(t *testing.T) {
	tests := []struct {
		name       string
		levels     []string
		wantRelays int
		wantLevels []int32
	}{
		{"all 2", []string{"2", "2", "2", "2"}, 1, []int32{2, 2, 2, 2}},
		{"2 with 1", []string{"2", "1", "2", "2"}, 1, []int32{2, 1, 2, 2}},
		{"all 1", []string{"1", "1", "1", "1"}, 1, []int32{1, 1, 1, 1}},
		{"one without relays", []string{"2", "2", "", "2"}, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relay := NewRelay()
			lbs := &Lbs{relayPeers: map[string]*LbsPeer{},
				localRelay: &relayEndpoint{region: localRelayRegion, publicAddr: "203.0.113.5:9879", register: relay.RegisterSession}}
			peers := playerInfoTestMatch()
			for i, p := range peers {
				p.conn = &PipeConn{}
				p.PlatformInfo = map[string]string{}
				if tt.levels[i] != "" {
					p.PlatformInfo["relay_server"] = tt.levels[i]
				}
			}
			lobby := &LbsLobby{Rule: DefaultRule, app: lbs}
			msgs, err := lobby.makeP2PMatchingMsg(peers[0].Battle, peers, nil)
			must(t, err)
			assertEq(t, 4, len(msgs))
			for _, msg := range msgs {
				matching := new(proto.P2PMatching)
				must(t, pb.Unmarshal(msg.Body, matching))
				assertEq(t, tt.wantRelays, len(matching.Relays))
				assertEq(t, tt.wantLevels, matching.RelayLevels)
			}
		})
	}
}
