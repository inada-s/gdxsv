package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// The relay forwards GGPO packets between the players of a rollback match whose direct route is poor.
//
// A client joins a session by sending relay pings (relayPingSize bytes, little endian):
//
//	uint32 magic, uint8 type, uint8 peer_id, uint8 relay_idx, uint8 reserved,
//	uint32 session_id, uint64 token, uint64 timestamp
//
// The relay answers with a pong carrying the same fields and binds the sender address to (session, peer).
// A player may ping from IPv4 and IPv6 alike; packets for it go to the address it last sent a game packet from.
// Game packets use GGPO's own relay header, the same one peers use to relay for each other:
//
//	uint16 const_magic, uint16 magic, uint16 sequence, uint8 remote_endpoint, uint8 type,
//	uint16 relay_magic, uint8 relay_to_endpoint, uint8 org_type
//
// The relay restores the original type and sends the packet to relay_to_endpoint of the sender's session.
const (
	relayPingMagic   = 0x594c4552 // "RELY"
	relayPingSize    = 28
	relayTypePing    = 1
	relayTypePong    = 2
	relayMaxPeers    = 4
	ggpoConstMagic   = 34046
	ggpoRelayMagic   = 26315
	ggpoTypeRelay    = 99
	ggpoHeaderSize   = 12
	ggpoMinType      = 1 // SyncRequest
	ggpoMaxType      = 8 // AppData
	relayIdleTimeout = 2 * time.Minute
	relayMaxLifetime = 3 * time.Hour
)

func mainRelay() {
	relay := NewRelay()

	var testSessionID uint32
	var testToken uint64
	if *relayTestSession != "" {
		id, token, err := parseRelayTestSession(*relayTestSession)
		if err != nil {
			logger.Fatal("invalid relay_test_session", zap.Error(err))
		}
		testSessionID, testToken = id, token
		relay.RegisterSession(testSessionID, testToken)
	}

	// Without a host in the address, this takes IPv4 and IPv6 alike, so it can relay between the two.
	conn, err := listenRelay(conf.RelayAddr)
	if err != nil {
		logger.Fatal("relay listen failed", zap.Error(err))
	}
	logger.Info("relay listening", zap.String("addr", conf.RelayAddr))
	go relay.Serve(conn)

	go func() {
		for range time.Tick(10 * time.Second) {
			relay.RemoveStaleSessions()
			if *relayTestSession != "" && relay.ActiveSessions() == 0 {
				relay.RegisterSession(testSessionID, testToken)
			}
		}
	}()

	if *relayTestSession != "" {
		select {}
	}

	// The lobby registers the sessions and decides when the relay is no longer needed.
	// Without it for a while, stop anyway so an orphaned VM does not keep running.
	lastConnected := time.Now()
	for {
		status := RelayStatus{Region: conf.RelayRegion, PublicAddr: conf.RelayPublicAddr, PublicAddr6: conf.RelayPublicAddr6}
		err := relay.DialAndSyncWithLbs(conf.LobbyPublicAddr, status, &lastConnected)
		if err == ErrRelayShutdown {
			logger.Info("relay shutdown requested by lbs")
			return
		}
		logger.Warn("relay lost lbs", zap.Error(err))
		if relayLbsLostTimeout < time.Since(lastConnected) {
			logger.Info("relay exit: no lbs for a while")
			return
		}
		time.Sleep(10 * time.Second)
	}
}

var ErrRelayShutdown = errors.New("relay shutdown requested")

const relayLbsLostTimeout = 15 * time.Minute

// RelayStatus is sent by a relay to the lobby.
type RelayStatus struct {
	Region      string `json:"region,omitempty"`
	PublicAddr  string `json:"public_addr,omitempty"`
	PublicAddr6 string `json:"public_addr6,omitempty"`
	Sessions    int    `json:"sessions"`
}

// RelayControl is sent by the lobby to a relay.
type RelayControl struct {
	Sessions []RelayControlSession `json:"sessions,omitempty"`
	Shutdown bool                  `json:"shutdown,omitempty"`
}

type RelayControlSession struct {
	SessionID uint32 `json:"session_id"`
	Token     uint64 `json:"token"`
}

func (r *Relay) DialAndSyncWithLbs(lobbyAddr string, status RelayStatus, lastConnected *time.Time) error {
	conn, err := net.Dial("tcp4", lobbyAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	*lastConnected = time.Now()
	logger.Info("relay connected to lbs", zap.String("lobby_addr", lobbyAddr))

	sendStatus := func() error {
		status.Sessions = r.ActiveSessions()
		body, err := json.Marshal(status)
		if err != nil {
			return err
		}
		buf := NewServerNotice(lbsExtRelayStatus).Writer().WriteBytes(body).Msg().Serialize()
		if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		_, err = conn.Write(buf)
		return err
	}
	if err := sendStatus(); err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		var data []byte
		for {
			n, err := conn.Read(buf)
			if err != nil {
				errc <- err
				return
			}
			data = append(data, buf[:n]...)
			for HeaderSize <= len(data) {
				n, msg := Deserialize(data)
				if n == 0 {
					break
				}
				data = data[n:]
				if msg == nil || msg.Command != lbsExtRelayControl {
					continue
				}
				var ctl RelayControl
				if err := json.Unmarshal(msg.Reader().ReadBytes(), &ctl); err != nil {
					logger.Error("invalid relay control", zap.Error(err))
					continue
				}
				for _, s := range ctl.Sessions {
					r.RegisterSession(s.SessionID, s.Token)
				}
				if ctl.Shutdown {
					errc <- ErrRelayShutdown
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-errc:
			return err
		case <-ticker.C:
			*lastConnected = time.Now()
			if err := sendStatus(); err != nil {
				return err
			}
		}
	}
}

func parseRelayTestSession(s string) (uint32, uint64, error) {
	idStr, tokenStr, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, errors.New("want <session_id>:<hex token>")
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	token, err := strconv.ParseUint(tokenStr, 16, 64)
	if err != nil {
		return 0, 0, err
	}
	return uint32(id), token, nil
}

type relaySession struct {
	id         uint32
	token      uint64
	peers      [relayMaxPeers]relayPeer
	created    time.Time
	lastActive time.Time
	forwarded  uint64
}

// relayPeer is a player bound to a session, by up to one address per IP family.
type relayPeer struct {
	addrs   [2]netip.AddrPort // IPv4, IPv6; zero value while unbound
	active  netip.AddrPort    // where packets for this player go
	playing bool              // active is where it sends game packets from, not just where it last pinged from
}

func relayFamily(addr netip.AddrPort) int {
	if addr.Addr().Is4() {
		return 0
	}
	return 1
}

// unbind forgets addr, one of the player's addresses.
func (p *relayPeer) unbind(addr netip.AddrPort) {
	p.addrs[relayFamily(addr)] = netip.AddrPort{}
	if p.active == addr {
		p.active = netip.AddrPort{}
		p.playing = false
	}
}

type relayBinding struct {
	session *relaySession
	peer    uint8
}

// Relay keys everything by netip.AddrPort, a plain value, so forwarding a packet allocates nothing.
type Relay struct {
	mtx      sync.Mutex
	sessions map[uint32]*relaySession
	bindings map[netip.AddrPort]relayBinding
	now      func() time.Time
}

func NewRelay() *Relay {
	return &Relay{
		sessions: map[uint32]*relaySession{},
		bindings: map[netip.AddrPort]relayBinding{},
		now:      time.Now,
	}
}

// listenRelay opens the relay socket. Without a host in addr it takes IPv4 and IPv6 alike.
func listenRelay(addr string) (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", udpAddr)
}

// RegisterSession allows the players of a match to use the relay. Registering again replaces the token.
func (r *Relay) RegisterSession(id uint32, token uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if s, ok := r.sessions[id]; ok {
		r.removeSessionLocked(s)
	}
	now := r.now()
	r.sessions[id] = &relaySession{id: id, token: token, created: now, lastActive: now}
	logger.Info("relay session registered", zap.Uint32("session_id", id))
}

func (r *Relay) ActiveSessions() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return len(r.sessions)
}

func (r *Relay) removeSessionLocked(s *relaySession) {
	for _, p := range s.peers {
		for _, addr := range p.addrs {
			if addr.IsValid() {
				delete(r.bindings, addr)
			}
		}
	}
	delete(r.sessions, s.id)
	logger.Info("relay session removed", zap.Uint32("session_id", s.id), zap.Uint64("forwarded", s.forwarded))
}

func (r *Relay) RemoveStaleSessions() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	now := r.now()
	for _, s := range r.sessions {
		if relayIdleTimeout < now.Sub(s.lastActive) || relayMaxLifetime < now.Sub(s.created) {
			r.removeSessionLocked(s)
		}
	}
}

func (r *Relay) Serve(conn *net.UDPConn) {
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			logger.Error("relay read failed", zap.Error(err))
			return
		}
		// A dual-stack socket reports IPv4 senders as IPv4-mapped IPv6; key them as plain IPv4.
		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())
		if dst, out := r.handle(buf[:n], from); dst.IsValid() {
			_, _ = conn.WriteToUDPAddrPort(out, dst)
		}
	}
}

// handle processes one datagram in place and returns where to send it (the pong or the forwarded packet), if anywhere.
func (r *Relay) handle(p []byte, from netip.AddrPort) (netip.AddrPort, []byte) {
	if len(p) == relayPingSize && binary.LittleEndian.Uint32(p[0:]) == relayPingMagic {
		return r.handlePing(p, from)
	}
	if ggpoHeaderSize <= len(p) &&
		binary.LittleEndian.Uint16(p[0:]) == ggpoConstMagic &&
		p[7] == ggpoTypeRelay &&
		binary.LittleEndian.Uint16(p[8:]) == ggpoRelayMagic {
		return r.handleGgpo(p, from)
	}
	return netip.AddrPort{}, nil
}

func (r *Relay) handlePing(p []byte, from netip.AddrPort) (netip.AddrPort, []byte) {
	if p[4] != relayTypePing {
		return netip.AddrPort{}, nil
	}
	peer := p[5]
	sessionID := binary.LittleEndian.Uint32(p[8:])
	token := binary.LittleEndian.Uint64(p[12:])
	if relayMaxPeers <= peer {
		return netip.AddrPort{}, nil
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	s, ok := r.sessions[sessionID]
	if !ok || s.token != token {
		return netip.AddrPort{}, nil
	}

	sp := &s.peers[peer]
	if old := sp.addrs[relayFamily(from)]; old != from {
		if old.IsValid() {
			delete(r.bindings, old)
			sp.unbind(old)
		}
		if b, ok := r.bindings[from]; ok {
			// The address moved to another peer or session; forget where it was.
			b.session.peers[b.peer].unbind(from)
		}
		sp.addrs[relayFamily(from)] = from
		r.bindings[from] = relayBinding{session: s, peer: peer}
		logger.Info("relay peer bound", zap.Uint32("session_id", sessionID), zap.Uint8("peer", peer),
			zap.String("addr", from.String()))
	}
	if !sp.playing {
		sp.active = from
	}
	s.lastActive = r.now()

	// The pong is the ping with its type changed.
	p[4] = relayTypePong
	return from, p
}

func (r *Relay) handleGgpo(p []byte, from netip.AddrPort) (netip.AddrPort, []byte) {
	to := p[10]
	orgType := p[11]
	if relayMaxPeers <= to || orgType < ggpoMinType || ggpoMaxType < orgType {
		return netip.AddrPort{}, nil
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	b, ok := r.bindings[from]
	if !ok || to == b.peer {
		return netip.AddrPort{}, nil
	}
	// Answer the sender where it sends from, so its NAT mapping for that family stays open.
	src := &b.session.peers[b.peer]
	src.active, src.playing = from, true
	dst := b.session.peers[to].active
	if !dst.IsValid() {
		return netip.AddrPort{}, nil
	}
	b.session.lastActive = r.now()
	b.session.forwarded++

	// Deliver it as the plain GGPO message, as a relaying peer does.
	p[7] = orgType
	p[8], p[9], p[10], p[11] = 0, 0, 0, 0
	return dst, p
}
