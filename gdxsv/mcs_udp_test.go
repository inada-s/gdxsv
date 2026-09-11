package main

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"gdxsv/gdxsv/proto"

	pb "google.golang.org/protobuf/proto"
)

// The tests in this file drive a real McsUDPServer read loop over a loopback
// UDP socket (127.0.0.1:0). Nothing leaves the machine and no external
// service is contacted.

const udpTestRecvWait = 500 * time.Millisecond

// newTestUDPServer starts a McsUDPServer read loop on a loopback socket and
// returns the server, its Mcs, and the address clients should send to.
// Cleanup closes the socket and verifies the read loop and all peers exit.
func newTestUDPServer(t *testing.T) (*McsUDPServer, *Mcs, *net.UDPAddr) {
	t.Helper()

	conf.BattleLogPath = t.TempDir()

	prevTimeout := mcsUDPRecvTimeout
	mcsUDPRecvTimeout = 3 * time.Second

	mcs := NewMcs(0)
	s := NewUDPServer(mcs)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	must(t, err)
	s.conn = conn

	done := make(chan error, 1)
	go func() { done <- s.readLoop() }()

	t.Cleanup(func() {
		// Kick out any peer still being served so its goroutine exits.
		s.mtx.Lock()
		for _, p := range s.peers {
			p.SetCloseReason("test_cleanup")
			_ = p.Close()
		}
		s.mtx.Unlock()
		udpWaitPeers(t, s, 0)
		udpWaitRoomsClosed(t, mcs)

		_ = conn.Close()
		select {
		case err := <-done:
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("readLoop returned %v, want net.ErrClosed", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("readLoop did not exit after the socket was closed")
		}
		mcsUDPRecvTimeout = prevTimeout
	})

	return s, mcs, conn.LocalAddr().(*net.UDPAddr)
}

func newTestUDPClient(t *testing.T, server *net.UDPAddr) *net.UDPConn {
	t.Helper()
	conn, err := net.DialUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}, server)
	must(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func udpSendRaw(t *testing.T, conn *net.UDPConn, data []byte) {
	t.Helper()
	_, err := conn.Write(data)
	must(t, err)
}

func udpSend(t *testing.T, conn *net.UDPConn, pkt *proto.Packet) {
	t.Helper()
	data, err := pb.Marshal(pkt)
	must(t, err)
	udpSendRaw(t, conn, data)
}

// udpRecv returns the next packet of the given type, skipping packets of other
// types (a served peer emits Battle flushes every ~16ms). It returns nil if
// nothing of that type arrives before the timeout.
func udpRecv(t *testing.T, conn *net.UDPConn, typ proto.MessageType, timeout time.Duration) *proto.Packet {
	t.Helper()
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	for {
		must(t, conn.SetReadDeadline(deadline))
		n, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return nil
			}
			t.Fatalf("client read: %v", err)
		}
		pkt := new(proto.Packet)
		must(t, pb.Unmarshal(buf[:n], pkt))
		if pkt.GetType() == typ {
			return pkt
		}
	}
}

func udpPeerCount(s *McsUDPServer) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return len(s.peers)
}

func udpFirstPeer(s *McsUDPServer) *McsUDPPeer {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, p := range s.peers {
		return p
	}
	return nil
}

func udpWaitPeers(t *testing.T, s *McsUDPServer, n int) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool { return udpPeerCount(s) == n })
}

func udpWaitRoomsClosed(t *testing.T, mcs *Mcs) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool {
		mcs.mtx.Lock()
		defer mcs.mtx.Unlock()
		return len(mcs.rooms) == 0
	})
}

func udpRoomLogCount(mcs *Mcs, battleCode string) int {
	mcs.mtx.Lock()
	room := mcs.rooms[battleCode]
	mcs.mtx.Unlock()
	if room == nil {
		return -1
	}
	room.logMtx.RLock()
	defer room.logMtx.RUnlock()
	if room.battleLog == nil {
		return -1
	}
	return len(room.battleLog.BattleData)
}

// udpShareBattle registers a one-player battle in sharedData so that a
// HelloServer with sessionID can join it.
func udpShareBattle(t *testing.T, battleCode, userID, sessionID string) {
	t.Helper()
	sharedData.ShareMcsGame(&McsGame{
		BattleCode: battleCode,
		GameDisk:   GameDiskDC2,
		LobbyID:    1,
	})
	sharedData.ShareMcsUser(&McsUser{
		BattleCode: battleCode,
		UserID:     userID,
		SessionID:  sessionID,
		Name:       userID,
		PilotName:  userID,
		GameDisk:   GameDiskDC2,
		Platform:   PlatformEmuX8664,
	})
}

// udpJoin performs the HelloServer handshake and returns the joined peer.
func udpJoin(t *testing.T, s *McsUDPServer, client *net.UDPConn, sessionID, wantUserID string) *McsUDPPeer {
	t.Helper()
	udpSend(t, client, &proto.Packet{Type: proto.MessageType_HelloServer, SessionId: sessionID})
	reply := udpRecv(t, client, proto.MessageType_HelloServer, udpTestRecvWait)
	if reply == nil {
		t.Fatal("no HelloServer reply")
	}
	assertEq(t, true, reply.GetHelloServerData().GetOk())
	assertEq(t, wantUserID, reply.GetHelloServerData().GetUserId())
	udpWaitPeers(t, s, 1)
	peer := udpFirstPeer(s)
	if peer == nil {
		t.Fatal("peer not registered")
	}
	assertEq(t, wantUserID, peer.UserID())
	return peer
}

func TestMcsUDP_PingWithoutPingDataDoesNotPanic(t *testing.T) {
	_, _, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)

	// A Ping-typed packet with no ping_data field: trivially craftable and,
	// before the fix, a nil dereference in readLoop.
	udpSend(t, client, &proto.Packet{Type: proto.MessageType_Ping})

	if pong := udpRecv(t, client, proto.MessageType_Pong, 200*time.Millisecond); pong != nil {
		assertEq(t, int64(0), pong.GetPongData().GetTimestamp())
	}

	// The loop must still be alive and serving.
	udpSend(t, client, &proto.Packet{
		Type:     proto.MessageType_Ping,
		PingData: &proto.PingMessage{Timestamp: 4242},
	})
	pong := udpRecv(t, client, proto.MessageType_Pong, udpTestRecvWait)
	if pong == nil {
		t.Fatal("server stopped responding after a Ping without ping_data")
	}
	assertEq(t, int64(4242), pong.GetPongData().GetTimestamp())
}

func TestMcsUDP_PingRoundTrip(t *testing.T) {
	_, _, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)

	for _, ts := range []int64{1, 1234567890123, -7} {
		udpSend(t, client, &proto.Packet{
			Type:     proto.MessageType_Ping,
			PingData: &proto.PingMessage{Timestamp: ts, UserId: "PINGER"},
		})
		pong := udpRecv(t, client, proto.MessageType_Pong, udpTestRecvWait)
		if pong == nil {
			t.Fatalf("no Pong for timestamp %d", ts)
		}
		assertEq(t, ts, pong.GetPongData().GetTimestamp())
	}
}

func TestMcsUDP_GarbageAndEmptyDatagramsAreIgnored(t *testing.T) {
	s, _, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)

	// Zero-length datagram.
	udpSendRaw(t, client, nil)
	// Bytes that are not a valid protobuf message.
	udpSendRaw(t, client, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// A truncated but otherwise plausible encoding (field 5, length 200, no payload).
	udpSendRaw(t, client, []byte{0x2a, 0xc8, 0x01, 'a', 'b'})

	if reply := udpRecv(t, client, proto.MessageType_Fin, 200*time.Millisecond); reply != nil {
		t.Fatalf("unexpected reply to garbage: %v", reply)
	}
	assertEq(t, 0, udpPeerCount(s))

	// The loop keeps serving the next valid packet.
	udpSend(t, client, &proto.Packet{
		Type:     proto.MessageType_Ping,
		PingData: &proto.PingMessage{Timestamp: 99},
	})
	pong := udpRecv(t, client, proto.MessageType_Pong, udpTestRecvWait)
	if pong == nil {
		t.Fatal("server stopped responding after garbage input")
	}
	assertEq(t, int64(99), pong.GetPongData().GetTimestamp())
}

func TestMcsUDP_HelloServerWithEmptyOrUnknownSessionCreatesNoPeer(t *testing.T) {
	s, _, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)

	for _, sessionID := range []string{"", "no-such-session"} {
		udpSend(t, client, &proto.Packet{Type: proto.MessageType_HelloServer, SessionId: sessionID})
		reply := udpRecv(t, client, proto.MessageType_HelloServer, udpTestRecvWait)
		if reply == nil {
			t.Fatalf("no HelloServer reply for session %q", sessionID)
		}
		assertEq(t, false, reply.GetHelloServerData().GetOk())
		assertEq(t, "", reply.GetHelloServerData().GetUserId())
		assertEq(t, 0, udpPeerCount(s))
	}

	// A flood of distinct unknown session IDs from one address must not grow
	// the peer map either.
	for i := 0; i < 50; i++ {
		udpSend(t, client, &proto.Packet{
			Type:      proto.MessageType_HelloServer,
			SessionId: "bogus-" + string(rune('a'+i%26)) + string(rune('0'+i%10)),
		})
	}
	for i := 0; i < 50; i++ {
		if udpRecv(t, client, proto.MessageType_HelloServer, udpTestRecvWait) == nil {
			t.Fatalf("missing HelloServer reply #%d", i)
		}
	}
	assertEq(t, 0, udpPeerCount(s))
}

func TestMcsUDP_BattleOrUnexpectedTypeFromUnknownPeerGetsFin(t *testing.T) {
	s, _, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)

	for _, typ := range []proto.MessageType{
		proto.MessageType_None,
		proto.MessageType_Battle,
		proto.MessageType_HelloLbs,
		proto.MessageType_SpectatorInputPushType,
	} {
		udpSend(t, client, &proto.Packet{Type: typ, SessionId: "whatever"})
		fin := udpRecv(t, client, proto.MessageType_Fin, udpTestRecvWait)
		if fin == nil {
			t.Fatalf("no Fin reply for type %v", typ)
		}
		assertEq(t, "close", fin.GetFinData().GetDetail())
		assertEq(t, 0, udpPeerCount(s))
	}

	// Fin from an unknown peer is silently ignored.
	udpSend(t, client, &proto.Packet{Type: proto.MessageType_Fin})
	if reply := udpRecv(t, client, proto.MessageType_Fin, 200*time.Millisecond); reply != nil {
		t.Fatalf("unexpected reply to stray Fin: %v", reply)
	}
}

func TestMcsUDP_OnReceiveFiltersNilAndForeignMessages(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
	u := NewMcsUDPPeer(nil, addr)
	u.SetUserID("ME")

	u.OnReceive(&proto.Packet{
		Type: proto.MessageType_Battle,
		Seq:  1,
		BattleData: []*proto.BattleMessage{
			nil,
			{UserId: "OTHER", Seq: 1, Body: []byte("spoof")},
			{UserId: "", Seq: 1, Body: []byte("anonymous")},
			{UserId: "ME", Seq: 1, Body: []byte("mine-1")},
			nil,
			{UserId: "ME", Seq: 1, Body: []byte("dup")}, // duplicate seq, dropped by the filter
			{UserId: "ME", Seq: 2, Body: []byte("mine-2")},
			{UserId: "OTHER", Seq: 2, Body: []byte("spoof")},
		},
	})

	u.readingMtx.Lock()
	got := append([]*proto.BattleMessage(nil), u.reading...)
	u.readingMtx.Unlock()

	if len(got) != 2 {
		t.Fatalf("reading = %v, want exactly the two own messages", got)
	}
	for i, msg := range got {
		if msg == nil {
			t.Fatalf("reading[%d] is nil", i)
		}
		assertEq(t, "ME", msg.GetUserId())
		assertEq(t, uint32(i+1), msg.GetSeq())
	}
	assertEq(t, "mine-1", string(got[0].GetBody()))
	assertEq(t, "mine-2", string(got[1].GetBody()))

	// Only accepted messages wake the serve loop.
	select {
	case <-u.chRecv:
	default:
		t.Fatal("chRecv was not signalled for accepted messages")
	}

	u.OnReceive(&proto.Packet{
		Type:       proto.MessageType_Battle,
		BattleData: []*proto.BattleMessage{nil, {UserId: "OTHER", Seq: 3}},
	})
	select {
	case <-u.chRecv:
		t.Fatal("chRecv signalled although every message was filtered out")
	default:
	}
	u.readingMtx.Lock()
	n := len(u.reading)
	u.readingMtx.Unlock()
	assertEq(t, 2, n)
}

func TestMcsUDP_JoinRelayAndFin(t *testing.T) {
	const battleCode = "UDPTEST-JOIN"
	const userID = "UDPJOIN1"
	const sessionID = "udp-sess-join"
	udpShareBattle(t, battleCode, userID, sessionID)

	s, mcs, addr := newTestUDPServer(t)
	client := newTestUDPClient(t, addr)
	peer := udpJoin(t, s, client, sessionID, userID)

	// A second HelloServer from the same address+session re-acks without
	// creating a second peer.
	udpSend(t, client, &proto.Packet{Type: proto.MessageType_HelloServer, SessionId: sessionID})
	reply := udpRecv(t, client, proto.MessageType_HelloServer, udpTestRecvWait)
	if reply == nil {
		t.Fatal("no HelloServer re-ack")
	}
	assertEq(t, true, reply.GetHelloServerData().GetOk())
	assertEq(t, userID, reply.GetHelloServerData().GetUserId())
	assertEq(t, 1, udpPeerCount(s))

	// The served peer flushes Battle packets to the client on its own.
	if udpRecv(t, client, proto.MessageType_Battle, udpTestRecvWait) == nil {
		t.Fatal("served peer never flushed a Battle packet")
	}

	// Battle data: nil / spoofed entries are dropped, own messages reach the room.
	// (The peer is keyed by address + session_id, so every packet carries it.)
	udpSend(t, client, &proto.Packet{
		Type:      proto.MessageType_Battle,
		SessionId: sessionID,
		BattleData: []*proto.BattleMessage{
			nil,
			{UserId: "SOMEONE-ELSE", Seq: 1, Body: []byte("spoof")},
			{UserId: userID, Seq: 1, Body: []byte("hello")},
			{UserId: userID, Seq: 2, Body: []byte("world")},
		},
	})
	waitFor(t, 5*time.Second, func() bool { return udpRoomLogCount(mcs, battleCode) == 2 })

	// Fin from the client closes the peer with the client's reason.
	udpSend(t, client, &proto.Packet{
		Type:      proto.MessageType_Fin,
		SessionId: sessionID,
		FinData:   &proto.FinMessage{Detail: "cl_test_bye"},
	})
	udpWaitPeers(t, s, 0)
	assertEq(t, "cl_test_bye", peer.GetCloseReason())
	udpWaitRoomsClosed(t, mcs)

	user, ok := sharedData.GetBattleUserInfo(sessionID)
	assertEq(t, true, ok)
	assertEq(t, McsUserStateLeft, user.State)
	assertEq(t, "cl_test_bye", user.CloseReason)
}

func TestMcsUDP_ServeTimesOutWithSvRecvTimeout(t *testing.T) {
	const battleCode = "UDPTEST-TIMEOUT"
	const userID = "UDPTMO1"
	const sessionID = "udp-sess-timeout"
	udpShareBattle(t, battleCode, userID, sessionID)

	s, mcs, addr := newTestUDPServer(t)
	mcsUDPRecvTimeout = 200 * time.Millisecond
	client := newTestUDPClient(t, addr)

	start := time.Now()
	peer := udpJoin(t, s, client, sessionID, userID)

	// Pings do not count as battle traffic and must not keep the peer alive.
	udpSend(t, client, &proto.Packet{Type: proto.MessageType_Ping, SessionId: sessionID, PingData: &proto.PingMessage{Timestamp: 1}})

	udpWaitPeers(t, s, 0)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("peer took %v to time out, expected roughly %v", elapsed, mcsUDPRecvTimeout)
	}
	assertEq(t, "sv_recv_timeout", peer.GetCloseReason())
	udpWaitRoomsClosed(t, mcs)

	user, ok := sharedData.GetBattleUserInfo(sessionID)
	assertEq(t, true, ok)
	assertEq(t, "sv_recv_timeout", user.CloseReason)
}
