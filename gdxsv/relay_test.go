package main

import (
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"
)

func relayPing(peer uint8, sessionID uint32, token uint64, timestamp uint64) []byte {
	p := make([]byte, relayPingSize)
	binary.LittleEndian.PutUint32(p[0:], relayPingMagic)
	p[4] = relayTypePing
	p[5] = peer
	p[6] = 1 // relay_idx, echoed back
	binary.LittleEndian.PutUint32(p[8:], sessionID)
	binary.LittleEndian.PutUint64(p[12:], token)
	binary.LittleEndian.PutUint64(p[20:], timestamp)
	return p
}

func ggpoRelayPacket(from, to, orgType uint8) []byte {
	p := make([]byte, ggpoHeaderSize+4)
	binary.LittleEndian.PutUint16(p[0:], ggpoConstMagic)
	binary.LittleEndian.PutUint16(p[2:], 0x1234)
	binary.LittleEndian.PutUint16(p[4:], 7)
	p[6] = from
	p[7] = ggpoTypeRelay
	binary.LittleEndian.PutUint16(p[8:], ggpoRelayMagic)
	p[10] = to
	p[11] = orgType
	copy(p[ggpoHeaderSize:], []byte{1, 2, 3, 4})
	return p
}

func udpAddr(s string) netip.AddrPort {
	return netip.MustParseAddrPort(s)
}

func TestRelay_PingBindsPeer(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 0xabcdef)
	a := udpAddr("1.2.3.4:5000")

	dst, pong := r.handle(relayPing(2, 100, 0xabcdef, 777), a)
	assertEq(t, a.String(), dst.String())
	assertEq(t, byte(relayTypePong), pong[4])
	assertEq(t, byte(2), pong[5])
	assertEq(t, byte(1), pong[6])
	assertEq(t, uint64(777), binary.LittleEndian.Uint64(pong[20:]))
	assertEq(t, a.String(), r.sessions[100].peers[2].String())
}

func TestRelay_PingRejected(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 0xabcdef)
	a := udpAddr("1.2.3.4:5000")

	tests := []struct {
		name string
		p    []byte
	}{
		{"wrong token", relayPing(0, 100, 0xabcdee, 1)},
		{"unknown session", relayPing(0, 101, 0xabcdef, 1)},
		{"peer out of range", relayPing(relayMaxPeers, 100, 0xabcdef, 1)},
		{"pong type", func() []byte { p := relayPing(0, 100, 0xabcdef, 1); p[4] = relayTypePong; return p }()},
		{"short", relayPing(0, 100, 0xabcdef, 1)[:relayPingSize-1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst, _ := r.handle(tt.p, a)
			if dst.IsValid() {
				t.Fatalf("expected no reply")
			}
			assertEq(t, 0, len(r.bindings))
		})
	}
}

func TestRelay_ForwardRestoresHeader(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 1)
	a := udpAddr("1.2.3.4:5000")
	b := udpAddr("5.6.7.8:6000")
	r.handle(relayPing(0, 100, 1, 0), a)
	r.handle(relayPing(3, 100, 1, 0), b)

	dst, out := r.handle(ggpoRelayPacket(0, 3, 3), a)
	assertEq(t, b.String(), dst.String())
	assertEq(t, uint16(ggpoConstMagic), binary.LittleEndian.Uint16(out[0:]))
	assertEq(t, uint16(0x1234), binary.LittleEndian.Uint16(out[2:]))
	assertEq(t, byte(0), out[6])
	assertEq(t, byte(3), out[7])
	assertEq(t, []byte{0, 0, 0, 0, 1, 2, 3, 4}, out[8:])
	assertEq(t, uint64(1), r.sessions[100].forwarded)
}

func TestRelay_ForwardDropped(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 1)
	r.RegisterSession(200, 2)
	a := udpAddr("1.2.3.4:5000")
	b := udpAddr("5.6.7.8:6000")
	c := udpAddr("9.9.9.9:7000")
	r.handle(relayPing(0, 100, 1, 0), a)
	r.handle(relayPing(1, 100, 1, 0), b)
	r.handle(relayPing(2, 200, 2, 0), c)

	tests := []struct {
		name string
		p    []byte
		from netip.AddrPort
	}{
		{"unbound sender", ggpoRelayPacket(0, 1, 3), udpAddr("8.8.8.8:1")},
		{"to itself", ggpoRelayPacket(0, 0, 3), a},
		{"to unbound peer", ggpoRelayPacket(0, 2, 3), a},
		{"other session", ggpoRelayPacket(2, 0, 3), c},
		{"invalid org type", ggpoRelayPacket(0, 1, 9), a},
		{"no org type", ggpoRelayPacket(0, 1, 0), a},
		{"to out of range", ggpoRelayPacket(0, relayMaxPeers, 3), a},
		{"not relay type", func() []byte { p := ggpoRelayPacket(0, 1, 3); p[7] = 3; return p }(), a},
		{"bad relay magic", func() []byte { p := ggpoRelayPacket(0, 1, 3); p[8] = 0; return p }(), a},
		{"short", ggpoRelayPacket(0, 1, 3)[:ggpoHeaderSize-1], a},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst, _ := r.handle(tt.p, tt.from)
			if dst.IsValid() {
				t.Fatalf("expected drop, sent to %v", dst)
			}
		})
	}
}

func TestRelay_Rebind(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 1)
	a := udpAddr("1.2.3.4:5000")
	a2 := udpAddr("1.2.3.4:5001")
	b := udpAddr("5.6.7.8:6000")
	r.handle(relayPing(0, 100, 1, 0), a)
	r.handle(relayPing(1, 100, 1, 0), b)

	// The NAT mapping of peer 0 changed.
	r.handle(relayPing(0, 100, 1, 0), a2)
	if dst, _ := r.handle(ggpoRelayPacket(0, 1, 3), a); dst.IsValid() {
		t.Fatalf("old address still forwards")
	}
	dst, _ := r.handle(ggpoRelayPacket(1, 0, 3), b)
	assertEq(t, a2.String(), dst.String())
}

func TestRelay_RemoveStaleSessions(t *testing.T) {
	r := NewRelay()
	now := time.Now()
	r.now = func() time.Time { return now }
	r.RegisterSession(100, 1)
	r.RegisterSession(200, 2)
	a := udpAddr("1.2.3.4:5000")
	r.handle(relayPing(0, 100, 1, 0), a)

	now = now.Add(relayIdleTimeout - time.Second)
	r.handle(relayPing(0, 100, 1, 0), a)
	now = now.Add(2 * time.Second)
	r.RemoveStaleSessions()
	assertEq(t, 1, r.ActiveSessions())
	if _, ok := r.sessions[100]; !ok {
		t.Fatalf("active session removed")
	}

	now = now.Add(relayMaxLifetime)
	r.RemoveStaleSessions()
	assertEq(t, 0, r.ActiveSessions())
	assertEq(t, 0, len(r.bindings))
}

// The hot path must not allocate, so a busy relay never makes the garbage collector run.
func TestRelay_NoAllocations(t *testing.T) {
	r := NewRelay()
	r.RegisterSession(100, 1)
	a := udpAddr("1.2.3.4:5000")
	b := udpAddr("[2001:db8::2]:6000")
	r.handle(relayPing(0, 100, 1, 0), a)
	r.handle(relayPing(1, 100, 1, 0), b)

	forward, ping := ggpoRelayPacket(0, 1, 3), relayPing(0, 100, 1, 0)
	buf := make([]byte, 2048)
	assertEq(t, 0.0, testing.AllocsPerRun(1000, func() {
		n := copy(buf, forward)
		if dst, _ := r.handle(buf[:n], a); dst != b {
			t.Fatal("not forwarded")
		}
	}))
	assertEq(t, 0.0, testing.AllocsPerRun(1000, func() {
		n := copy(buf, ping)
		if dst, _ := r.handle(buf[:n], a); dst != a {
			t.Fatal("no pong")
		}
	}))
}

// Forwarding through a real socket, IPv4 in and IPv6 out, as a dual-stack relay does.
func TestRelay_ServeDualStack(t *testing.T) {
	conn, err := listenRelay("[::]:0")
	if err != nil {
		t.Skip("no dual-stack socket:", err)
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port
	r := NewRelay()
	r.RegisterSession(100, 1)
	go r.Serve(conn)

	v4, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(t, err)
	defer v4.Close()
	v6, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	if err != nil {
		t.Skip("no IPv6 loopback:", err)
	}
	defer v6.Close()
	relay4 := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
	relay6 := &net.UDPAddr{IP: net.IPv6loopback, Port: port}

	buf := make([]byte, 2048)
	bind := func(c *net.UDPConn, to *net.UDPAddr, peer uint8) {
		t.Helper()
		_, err := c.WriteToUDP(relayPing(peer, 100, 1, 0), to)
		must(t, err)
		must(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
		n, _, err := c.ReadFromUDP(buf)
		must(t, err)
		assertEq(t, byte(relayTypePong), buf[:n][4])
	}
	bind(v4, relay4, 0)
	bind(v6, relay6, 1)

	_, err = v4.WriteToUDP(ggpoRelayPacket(0, 1, 3), relay4)
	must(t, err)
	must(t, v6.SetReadDeadline(time.Now().Add(2*time.Second)))
	n, _, err := v6.ReadFromUDP(buf)
	must(t, err)
	assertEq(t, byte(3), buf[:n][7])
}

func BenchmarkRelay_Forward(b *testing.B) {
	r := NewRelay()
	r.RegisterSession(100, 1)
	a, c := udpAddr("1.2.3.4:5000"), udpAddr("5.6.7.8:6000")
	r.handle(relayPing(0, 100, 1, 0), a)
	r.handle(relayPing(1, 100, 1, 0), c)
	forward := ggpoRelayPacket(0, 1, 3)
	buf := make([]byte, 2048)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		n := copy(buf, forward)
		r.handle(buf[:n], a)
	}
}
