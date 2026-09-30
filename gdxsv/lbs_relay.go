package main

import (
	crand "crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"gdxsv/gdxsv/proto"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	relayMinUsers = 4 // a match's worth of players who could use a relay

	relayIdleShutdown   = 30 * time.Minute
	relayUpdateInterval = 10 * time.Second

	localRelayRegion  = "lbs"
	maxRelaysPerMatch = 4 // what clients accept
)

// relayEndpoint is a relay a match can be offered: a relay VM registered with the lobby, or the lobby's own.
type relayEndpoint struct {
	region      string
	publicAddr  string
	publicAddr6 string
	register    func(sessionID uint32, token uint64)
}

func peerRelayEndpoint(p *LbsPeer) *relayEndpoint {
	return &relayEndpoint{
		region:      p.relayStatus.Region,
		publicAddr:  p.relayStatus.PublicAddr,
		publicAddr6: p.relayStatus.PublicAddr6,
		register: func(sessionID uint32, token uint64) {
			sendRelayControl(p, &RelayControl{Sessions: []RelayControlSession{{SessionID: sessionID, Token: token}}})
		},
	}
}

// StartLocalRelay runs a relay inside the lobby process. Matches get it when no relay VM is running.
func (lbs *Lbs) StartLocalRelay(addr, publicAddr, publicAddr6 string) error {
	conn, err := listenRelay(addr)
	if err != nil {
		return err
	}
	relay := NewRelay()
	go relay.Serve(conn)
	go func() {
		for range time.Tick(10 * time.Second) {
			relay.RemoveStaleSessions()
		}
	}()
	lbs.localRelay = &relayEndpoint{
		region:      localRelayRegion,
		publicAddr:  publicAddr,
		publicAddr6: publicAddr6,
		register:    relay.RegisterSession,
	}
	logger.Info("local relay listening", zap.String("addr", addr),
		zap.String("public_addr", publicAddr), zap.String("public_addr6", publicAddr6))
	return nil
}

var _ = register(lbsExtRelayStatus, func(p *LbsPeer, m *LbsMessage) {
	var status RelayStatus
	if err := json.Unmarshal(m.Reader().ReadBytes(), &status); err != nil {
		p.logger.Error("invalid relay status", zap.Error(err))
		return
	}
	if status.PublicAddr == "" {
		return
	}
	// Anyone can connect to the lobby, and the lobby hands relays to players, so only accept the ones that know the
	// secret.
	if conf.RelaySecret == "" || subtle.ConstantTimeCompare([]byte(status.Secret), []byte(conf.RelaySecret)) != 1 {
		p.logger.Warn("relay status with a wrong secret", zap.String("public_addr", status.PublicAddr))
		return
	}
	status.Secret = ""
	if p.relayStatus == nil {
		p.logger.Info("relay registered", zap.String("public_addr", status.PublicAddr), zap.String("region", status.Region))
		// A relay that was already running when the lobby started gets the full grace period.
		if _, ok := p.app.relayLastNeeded[status.Region]; !ok {
			p.app.relayLastNeeded[status.Region] = time.Now()
		}
	}
	p.relayStatus = &status
	p.app.relayPeers[status.PublicAddr] = p
})

func gcpLatency(p *LbsPeer, region string) int {
	v, err := strconv.Atoi(p.PlatformInfo[region])
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// relayRegions returns the regions GDXSV_RELAY_REGIONS runs relay VMs in.
func relayRegions() []string {
	var regions []string
	for _, r := range strings.Split(conf.RelayRegions, ",") {
		if r = strings.TrimSpace(r); r != "" {
			regions = append(regions, r)
		}
	}
	return regions
}

// relayNeeded reports whether enough players who could use a relay are online, from at least two region groups,
// so some of them may play across a long route.
func (lbs *Lbs) relayNeeded() bool {
	users := 0
	first, mixed := "", false
	for _, p := range lbs.userPeers {
		if p.PlatformInfo["relay_server"] != "1" {
			continue
		}
		users++
		if g := gcpRegionGroup[p.bestRegion]; g == "" {
			continue
		} else if first == "" {
			first = g
		} else if g != first {
			mixed = true
		}
	}
	return relayMinUsers <= users && mixed
}

func (lbs *Lbs) findRelay(region string) *LbsPeer {
	for _, p := range lbs.relayPeers {
		if p.relayStatus.Region == region {
			return p
		}
	}
	return nil
}

// matchRelays lists the relays offered to a match, best first: relay VMs by their worst relayed RTT between
// participants, then the lobby's own relay. Clients pick the relay for each peer by the lowest sum of both RTTs to
// it, and settle disagreements on the earlier one, so the order must be the same for every participant.
func (lbs *Lbs) matchRelays(participants []*LbsPeer) []*relayEndpoint {
	type scored struct {
		p     *LbsPeer
		worst int
	}
	var vms []scored
	for _, p := range lbs.relayPeers {
		worst := 0
		for i := range participants {
			for j := i + 1; j < len(participants); j++ {
				a, b := gcpLatency(participants[i], p.relayStatus.Region), gcpLatency(participants[j], p.relayStatus.Region)
				via := 999
				if 0 < a && 0 < b {
					via = a + b
				}
				if worst < via {
					worst = via
				}
			}
		}
		vms = append(vms, scored{p, worst})
	}
	sort.Slice(vms, func(i, j int) bool {
		if vms[i].worst != vms[j].worst {
			return vms[i].worst < vms[j].worst
		}
		return vms[i].p.relayStatus.PublicAddr < vms[j].p.relayStatus.PublicAddr
	})

	var relays []*relayEndpoint
	for _, v := range vms {
		relays = append(relays, peerRelayEndpoint(v.p))
	}
	if lbs.localRelay != nil {
		relays = append(relays, lbs.localRelay)
	}
	if maxRelaysPerMatch < len(relays) {
		relays = relays[:maxRelaysPerMatch]
	}
	return relays
}

// updateRelay starts a relay VM in every region of GDXSV_RELAY_REGIONS while one may be needed, and stops each once
// it has been unneeded and idle for a while.
func (lbs *Lbs) updateRelay(now time.Time) {
	regions := relayRegions()
	if len(regions) == 0 {
		return
	}
	if lbs.relayNeeded() {
		for _, r := range regions {
			lbs.relayLastNeeded[r] = now
			if lbs.findRelay(r) == nil && McsFuncEnabled() {
				GoMcsFuncAllocRole(r, "relay")
			}
		}
	}
	for _, p := range lbs.relayPeers {
		if now.Sub(lbs.relayLastNeeded[p.relayStatus.Region]) < relayIdleShutdown || p.relayStatus.Sessions != 0 {
			continue
		}
		p.logger.Info("relay no longer needed, shutting it down", zap.String("region", p.relayStatus.Region))
		sendRelayControl(p, &RelayControl{Shutdown: true})
	}
}

func sendRelayControl(p *LbsPeer, ctl *RelayControl) {
	body, err := json.Marshal(ctl)
	if err != nil {
		p.logger.Error("json.Marshal", zap.Error(err))
		return
	}
	p.SendMessage(NewServerNotice(lbsExtRelayControl).Writer().WriteBytes(body).Msg())
}

func allSupportRelay(participants []*LbsPeer) bool {
	for _, p := range participants {
		if p.PlatformInfo["relay_server"] != "1" {
			return false
		}
	}
	return true
}

// openRelaySession admits the players of a match to the relay and returns what they need to use it.
func openRelaySession(relay *relayEndpoint, sessionID uint32) (*proto.RelayServer, error) {
	host, portStr, err := net.SplitHostPort(relay.publicAddr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	// Clients take a numeric IPv4 address. No name lookup here: this runs in the event loop.
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return nil, errors.New("relay address is not a numeric IPv4 address")
	}

	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return nil, err
	}
	token := binary.LittleEndian.Uint64(b[:])

	relay.register(sessionID, token)
	return &proto.RelayServer{
		Region: relay.region,
		Ip:     ip.To4().String(),
		Ip6:    relayIPv6(relay.publicAddr6, port),
		Port:   int32(port),
		Token:  token,
	}, nil
}

// relayIPv6 returns the relay's numeric IPv6 address, or "" if it has none on the same port.
func relayIPv6(publicAddr6 string, port int) string {
	if publicAddr6 == "" {
		return ""
	}
	host, portStr, err := net.SplitHostPort(publicAddr6)
	if err != nil || portStr != strconv.Itoa(port) {
		logger.Warn("ignoring relay IPv6 address", zap.String("public_addr6", publicAddr6))
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		logger.Warn("ignoring relay IPv6 address", zap.String("public_addr6", publicAddr6))
		return ""
	}
	return ip.String()
}
