package main

import (
	"fmt"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

func TestSpectatorRegistry_ChallengeDefersBothMapsAndIgnoresForgedAck(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "battle"}, "dc2", nil)
	s, _ := r.GetAny("battle")
	s.PushInputs(0, make([]uint64, 10))
	addr := testAddr(40000)
	req := &proto.SpectatorSubscribeRequest{BattleCode: "battle", Cookie: make([]byte, spectatorCookieBytes)}

	// A challenge must not wait on the input-capture mutex.
	s.mtx.Lock()
	challenge := r.HandleSubscribe(addr, req)
	s.mtx.Unlock()
	if challenge == nil {
		t.Fatal("missing stateless challenge")
	}
	assertEq(t, 0, len(s.downlinks))
	assertEq(t, 0, len(r.subscriberIndex))
	assertEq(t, 0, len(r.wake))
	r.HandleAck(addr, &proto.SpectatorInputAck{BattleCode: "battle", AckFrame: 10})
	assertEq(t, 0, len(s.downlinks))
	assertEq(t, 0, len(r.subscriberIndex))

	// Lost challenges are harmless; the request can be repeated statelessly.
	again := r.HandleSubscribe(addr, req)
	if again == nil {
		t.Fatal("missing replacement challenge")
	}
	req.Cookie = again.Cookie
	assertEq(t, true, r.HandleSubscribe(addr, req) == nil)
	assertEq(t, 1, len(s.downlinks))
	assertEq(t, s, r.subscriberIndex[addr.String()])
	assertEq(t, 1, len(r.wake))
	push, ok := s.buildPush(s.downlinks[addr.String()])
	assertEq(t, true, ok)
	assertEq(t, true, push.Header != nil)
}

func TestSpectatorRegistry_ChallengeDoesNotAmplify(t *testing.T) {
	for _, code := range []string{"b", "1787426305907", "battle\x00code"} {
		t.Run(code, func(t *testing.T) {
			r := newTestSpectatorRegistry()
			r.Open(&proto.P2PMatching{BattleCode: code}, "dc2", nil)
			req := &proto.SpectatorSubscribeRequest{BattleCode: code, Cookie: make([]byte, spectatorCookieBytes)}
			challenge := r.HandleSubscribe(testAddr(40000), req)
			if challenge == nil {
				t.Fatal("missing challenge")
			}
			request := &proto.Packet{Type: proto.MessageType_SpectatorSubscribeType, SpectatorSubscribeData: req}
			reply := &proto.Packet{Type: proto.MessageType_SpectatorSubscribeChallengeType, SpectatorSubscribeChallengeData: challenge}
			if pb.Size(reply) > pb.Size(request) {
				t.Fatalf("challenge %d bytes exceeds request %d", pb.Size(reply), pb.Size(request))
			}
			for _, size := range []int{0, 1, 15, 17, 1024} {
				req.Cookie = make([]byte, size)
				assertEq(t, true, r.HandleSubscribe(testAddr(40000), req) == nil)
			}
			assertEq(t, 0, len(r.subscriberIndex))
		})
	}
}

func TestSpectatorRegistry_SpoofedSubscribeFloodAllocatesNoSubscribers(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "battle"}, "dc2", nil)
	req := &proto.SpectatorSubscribeRequest{BattleCode: "battle", Cookie: make([]byte, spectatorCookieBytes)}
	for port := 10000; port < 20000; port++ {
		if r.HandleSubscribe(testAddr(port), req) == nil {
			t.Fatal("expected stateless challenge")
		}
		r.HandleAck(testAddr(port), &proto.SpectatorInputAck{BattleCode: "battle"})
	}
	s, _ := r.GetAny("battle")
	assertEq(t, 0, len(s.downlinks))
	assertEq(t, 0, len(r.subscriberIndex))
	assertEq(t, 0, len(r.wake))
}

func TestSpectatorRegistry_CookieCannotBeReusedAtAnotherEndpointOrBattle(t *testing.T) {
	r := newTestSpectatorRegistry()
	for _, code := range []string{"a", "b"} {
		r.Open(&proto.P2PMatching{BattleCode: code}, "dc2", nil)
	}
	addr := testAddr(40000)
	req := testSubscribeRequest(r, "a", addr, 0)
	assertEq(t, true, r.HandleSubscribe(testAddr(40001), req) != nil)
	req.BattleCode = "b"
	assertEq(t, true, r.HandleSubscribe(addr, req) != nil)
	assertEq(t, 0, len(r.subscriberIndex))
}

func TestSpectatorRegistry_ExpiredKeepaliveRenewsWithoutLosingProgress(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "battle"}, "dc2", nil)
	s, _ := r.GetAny("battle")
	s.PushInputs(0, make([]uint64, 10))
	addr := testAddr(40000)
	req := testSubscribeRequest(r, "battle", addr, 5)
	r.HandleSubscribe(addr, req)
	sub := s.downlinks[addr.String()]
	sub.probeDue = false
	sub.skipFanouts = 64
	lastSeen := sub.lastSeen
	epoch := time.Now().Unix() / int64(spectatorCookieWindow/time.Second)
	req.Cookie = r.subscribeCookie("battle", addr, epoch-2)
	challenge := r.HandleSubscribe(addr, req)
	if challenge == nil {
		t.Fatal("expected renewal challenge")
	}
	assertEq(t, lastSeen, sub.lastSeen)
	assertEq(t, false, sub.probeDue)
	assertEq(t, int32(5), sub.ackedFrame)
	req.Cookie = challenge.Cookie
	r.HandleSubscribe(addr, req)
	assertEq(t, 1, len(r.subscriberIndex))
	assertEq(t, sub, s.downlinks[addr.String()])
	assertEq(t, true, sub.probeDue)
	assertEq(t, false, sub.shouldSkipPush())
	assertEq(t, int32(64), sub.skipFanouts)
	assertEq(t, int32(5), sub.ackedFrame)
}

func TestSpectatorRegistry_RejectsInvalidResumeFramesWithoutChangingState(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.Open(&proto.P2PMatching{BattleCode: "battle"}, "dc2", nil)
	s, _ := r.GetAny("battle")
	s.PushInputs(0, make([]uint64, 10))
	for _, frame := range []int32{-1, math.MinInt32, 11, math.MaxInt32} {
		addr := testAddr(40000)
		r.HandleSubscribe(addr, testSubscribeRequest(r, "battle", addr, frame))
		assertEq(t, 0, len(s.downlinks))
		assertEq(t, 0, len(r.subscriberIndex))
	}
	addr := testAddr(40000)
	r.HandleSubscribe(addr, testSubscribeRequest(r, "battle", addr, 10))
	sub := s.downlinks[addr.String()]
	sub.probeDue = false
	lastSeen := sub.lastSeen
	for _, frame := range []int32{-1, 11, math.MaxInt32} {
		r.HandleSubscribe(addr, testSubscribeRequest(r, "battle", addr, frame))
		assertEq(t, int32(10), sub.ackedFrame)
		assertEq(t, lastSeen, sub.lastSeen)
		assertEq(t, false, sub.probeDue)
	}
}

func TestSpectatorRegistry_SubscriptionCeilingsAndBattleSwitch(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.maxSubscribers = 2
	r.maxSubscribersPerBattle = 1
	for _, code := range []string{"a", "b", "c"} {
		r.Open(&proto.P2PMatching{BattleCode: code}, "dc2", nil)
	}
	first, second, third := testAddr(40000), testAddr(40001), testAddr(40002)
	join := func(addr *net.UDPAddr, code string) {
		r.HandleSubscribe(addr, testSubscribeRequest(r, code, addr, 0))
	}
	join(first, "a")
	join(second, "a") // per-battle ceiling, though global room remains
	assertEq(t, 1, len(r.subscriberIndex))
	join(second, "b")
	join(third, "c") // server-wide ceiling
	assertEq(t, 2, len(r.subscriberIndex))
	assertEq(t, 0, len(r.sessions["c"].downlinks))
	join(first, "b") // a rejected move must retain the previous battle
	assertEq(t, r.sessions["a"], r.subscriberIndex[first.String()])
	join(first, "c") // moving an existing endpoint consumes no new global slot
	assertEq(t, 0, len(r.sessions["a"].downlinks))
	assertEq(t, 1, len(r.sessions["c"].downlinks))
	assertEq(t, 2, len(r.subscriberIndex))
	join(first, "c") // a keepalive is allowed at both ceilings
	assertEq(t, 2, len(r.subscriberIndex))
	r.HandleAck(first, &proto.SpectatorInputAck{BattleCode: "a"})
	assertEq(t, r.sessions["c"], r.subscriberIndex[first.String()])
}

func TestSpectatorRegistry_ConcurrentAdmissionHonorsBothCeilings(t *testing.T) {
	r := newTestSpectatorRegistry()
	r.maxSubscribers = 12
	r.maxSubscribersPerBattle = 8
	for i := 0; i < 3; i++ {
		r.Open(&proto.P2PMatching{BattleCode: fmt.Sprint(i)}, "dc2", nil)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			addr := testAddr(10000 + i)
			code := fmt.Sprint(i % 3)
			r.HandleSubscribe(addr, testSubscribeRequest(r, code, addr, 0))
		}(i)
	}
	wg.Wait()
	assertEq(t, 12, len(r.subscriberIndex))
	count := 0
	for _, s := range r.sessions {
		if len(s.downlinks) > r.maxSubscribersPerBattle {
			t.Fatal("per-battle limit exceeded")
		}
		count += len(s.downlinks)
	}
	assertEq(t, len(r.subscriberIndex), count)
}

func TestSpectatorRegistry_ExpiryReleasesBothMapsAndSlots(t *testing.T) {
	for _, expiry := range []string{"subscriber", "closed", "idle"} {
		t.Run(expiry, func(t *testing.T) {
			r := newTestSpectatorRegistry()
			r.maxSubscribers = 1
			r.Open(&proto.P2PMatching{BattleCode: "old"}, "dc2", nil)
			s, _ := r.GetAny("old")
			addr := testAddr(40000)
			r.HandleSubscribe(addr, testSubscribeRequest(r, "old", addr, 0))
			s.downlinks[addr.String()].lastSeen = time.Now().Add(-2 * spectatorSubscriberTimeout)
			switch expiry {
			case "subscriber":
				r.fanoutOnce(nil) // no subscribers remain to send to
			case "closed":
				s.Close("finished", -1)
				s.closedAt = time.Now().Add(-2 * spectatorSessionRetention)
				r.mtx.Lock()
				r.sweepLocked()
				r.mtx.Unlock()
			case "idle":
				s.lastPushAt = time.Now().Add(-2 * spectatorSessionIdleTimeout)
				r.mtx.Lock()
				r.sweepLocked()
				r.mtx.Unlock()
			}
			assertEq(t, 0, len(s.downlinks))
			assertEq(t, 0, len(r.subscriberIndex))
			r.Open(&proto.P2PMatching{BattleCode: "new"}, "dc2", nil)
			addr = testAddr(40001)
			r.HandleSubscribe(addr, testSubscribeRequest(r, "new", addr, 0))
			assertEq(t, 1, len(r.subscriberIndex))
		})
	}
}

func TestSpectatorRegistry_SweepRetainsConnectedSessions(t *testing.T) {
	for _, expiry := range []string{"closed", "idle"} {
		for _, activity := range []string{"keepalive", "ack"} {
			t.Run(expiry+"/"+activity, func(t *testing.T) {
				r := newTestSpectatorRegistry()
				r.Open(&proto.P2PMatching{BattleCode: "watched"}, "dc2", nil)
				s, _ := r.GetAny("watched")
				s.PushInputs(0, []uint64{1, 2, 3})
				active, stale := testAddr(40000), testAddr(40001)
				for _, addr := range []*net.UDPAddr{active, stale} {
					r.HandleSubscribe(addr, testSubscribeRequest(r, "watched", addr, 3))
					s.downlinks[addr.String()].lastSeen = time.Now().Add(-2 * spectatorSubscriberTimeout)
				}
				if expiry == "closed" {
					s.Close("finished", -1)
					s.closedAt = time.Now().Add(-2 * spectatorSessionRetention)
				} else {
					s.lastPushAt = time.Now().Add(-2 * spectatorSessionIdleTimeout)
				}
				beforePushAt, beforeClosedAt := s.lastPushAt, s.closedAt

				// Even a fully caught-up viewer keeps the recording available.
				if activity == "keepalive" {
					r.HandleSubscribe(active, testSubscribeRequest(r, "watched", active, 3))
				} else {
					r.HandleAck(active, &proto.SpectatorInputAck{BattleCode: "watched", AckFrame: 3})
				}
				r.mtx.Lock()
				r.sweepLocked()
				r.mtx.Unlock()

				assertEq(t, s, r.sessions["watched"])
				assertEq(t, 1, len(s.downlinks))
				assertEq(t, 1, len(r.subscriberIndex))
				assertEq(t, s, r.subscriberIndex[active.String()])
				assertEq(t, beforePushAt, s.lastPushAt)
				assertEq(t, beforeClosedAt, s.closedAt)
				assertEq(t, []uint64{1, 2, 3}, s.log.Inputs)

				// Opening another battle also sweeps, without evicting this one.
				r.Open(&proto.P2PMatching{BattleCode: "new"}, "dc2", nil)
				assertEq(t, s, r.sessions["watched"])

				// The original deadline already passed: no extra wait after exit.
				s.downlinks[active.String()].lastSeen = time.Now().Add(-2 * spectatorSubscriberTimeout)
				r.mtx.Lock()
				r.sweepLocked()
				r.mtx.Unlock()
				_, ok := r.GetAny("watched")
				assertEq(t, false, ok)
				assertEq(t, 0, len(s.downlinks))
				assertEq(t, 0, len(r.subscriberIndex))
				_, ok = r.GetAny("new")
				assertEq(t, true, ok)
			})
		}
	}
}

func TestSpectatorRegistry_SweepExpiresSubscribersWithinRetention(t *testing.T) {
	for _, state := range []string{"open", "closed"} {
		t.Run(state, func(t *testing.T) {
			r := newTestSpectatorRegistry()
			r.Open(&proto.P2PMatching{BattleCode: "recent"}, "dc2", nil)
			s, _ := r.GetAny("recent")
			addr := testAddr(40000)
			r.HandleSubscribe(addr, testSubscribeRequest(r, "recent", addr, 0))
			s.downlinks[addr.String()].lastSeen = time.Now().Add(-2 * spectatorSubscriberTimeout)
			if state == "closed" {
				s.Close("finished", -1)
			}

			r.mtx.Lock()
			r.sweepLocked()
			r.mtx.Unlock()

			assertEq(t, s, r.sessions["recent"])
			assertEq(t, 0, len(s.downlinks))
			assertEq(t, 0, len(r.subscriberIndex))
		})
	}
}

func TestSpectatorRegistry_InvalidAckDoesNotAdvanceOrRefresh(t *testing.T) {
	r := newTestSpectatorRegistry()
	s := newTestSpectatorSessionWithPatches(2, 1)
	r.sessions[s.battleCode] = s
	s.PushInputs(0, make([]uint64, 10))
	addr := testAddr(40000)
	r.HandleSubscribe(addr, testSubscribeRequest(r, s.battleCode, addr, 5))
	sub := s.downlinks[addr.String()]
	lastSeen := sub.lastSeen
	sub.skipFanouts = 64
	for _, ack := range []*proto.SpectatorInputAck{
		{BattleCode: "another-battle", AckFrame: 7},
		{BattleCode: s.battleCode, AckFrame: -1},
		{BattleCode: s.battleCode, AckFrame: 11},
		{BattleCode: s.battleCode, AckFrame: math.MaxInt32},
		{BattleCode: s.battleCode, AckFrame: 7, PatchAck: -1},
		{BattleCode: s.battleCode, AckFrame: 7, PatchAck: 3},
		{BattleCode: s.battleCode, AckFrame: 7, PatchAck: math.MaxInt32},
	} {
		r.HandleAck(addr, ack)
		assertEq(t, int32(5), sub.ackedFrame)
		assertEq(t, int32(0), sub.ackedPatches)
		assertEq(t, int32(64), sub.skipFanouts)
		assertEq(t, lastSeen, sub.lastSeen)
	}
	r.HandleAck(addr, &proto.SpectatorInputAck{BattleCode: s.battleCode, AckFrame: 10, PatchAck: 2})
	assertEq(t, int32(10), sub.ackedFrame)
	assertEq(t, int32(2), sub.ackedPatches)
	assertEq(t, int32(0), sub.skipFanouts)
}

func TestSpectatorSubscribeUDP_RetriesChallengeAndBootstraps(t *testing.T) {
	listen := func() *net.UDPConn {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	server, client := listen(), listen()
	r := newTestSpectatorRegistry()
	saved := spectatorRegistry
	spectatorRegistry = r
	t.Cleanup(func() { spectatorRegistry = saved })
	r.Open(&proto.P2PMatching{BattleCode: "1787426305907"}, "dc2", nil)
	s, _ := r.GetAny("1787426305907")
	s.PushInputs(0, make([]uint64, 10))
	req := &proto.SpectatorSubscribeRequest{BattleCode: s.battleCode, Cookie: make([]byte, spectatorCookieBytes)}

	readPacket := func(conn *net.UDPConn) (*proto.Packet, *net.UDPAddr) {
		buf := make([]byte, 8192)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			t.Fatal(err)
		}
		pkt := &proto.Packet{}
		if err := pb.Unmarshal(buf[:n], pkt); err != nil {
			t.Fatal(err)
		}
		return pkt, addr
	}
	sendSubscribe := func() {
		pkt := &proto.Packet{Type: proto.MessageType_SpectatorSubscribeType, SpectatorSubscribeData: req}
		bin, err := pb.Marshal(pkt)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.WriteToUDP(bin, server.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}
		inbound, addr := readPacket(server)
		handleSpectatorSubscribe(server, addr, inbound.SpectatorSubscribeData)
	}

	// Losing the first challenge allocates nothing; the next request retries.
	sendSubscribe()
	first, _ := readPacket(client)
	assertEq(t, proto.MessageType_SpectatorSubscribeChallengeType, first.Type)
	assertEq(t, 0, len(r.subscriberIndex))
	sendSubscribe()
	second, _ := readPacket(client)
	assertEq(t, proto.MessageType_SpectatorSubscribeChallengeType, second.Type)
	assertEq(t, s.battleCode, second.GetSpectatorSubscribeChallengeData().GetBattleCode())
	req.Cookie = second.GetSpectatorSubscribeChallengeData().GetCookie()
	sendSubscribe()
	assertEq(t, 1, len(r.subscriberIndex))
	r.fanoutOnce(server)
	header, _ := readPacket(client)
	assertEq(t, proto.MessageType_SpectatorInputPushType, header.Type)
	assertEq(t, true, header.GetSpectatorInputPushData().GetHeader() != nil)

	// A lost header is still retried by a from-frame-zero keepalive.
	sendSubscribe()
	r.fanoutOnce(server)
	header, _ = readPacket(client)
	assertEq(t, true, header.GetSpectatorInputPushData().GetHeader() != nil)
	r.fanoutOnce(server)
	inputs, _ := readPacket(client)
	assertEq(t, 10, len(inputs.GetSpectatorInputPushData().GetInputs()))
}
