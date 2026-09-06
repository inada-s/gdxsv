package main

import (
	"crypto/rand"
	"math"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

// maxSpectatorPushFrames caps how many frames one push may carry. A healthy
// match sends one frame per push; this only grows when acks lag or packets
// drop. 128 frames is ~2s at 60fps, double GGPO's own pending-output window,
// and lands around 1KB - well inside one datagram, so no IP fragmentation.
const maxSpectatorPushFrames = 128

// maxSpectatorPendingFrames bounds the out-of-order receive window,
// measured from the next missing frame.
const maxSpectatorPendingFrames = 256

// maxSpectatorRounds is the game's maximum number of rounds per battle.
const maxSpectatorRounds = 10

const defaultSpectatorMaxSubscribers = 4096
const defaultSpectatorMaxSubscribersPerBattle = 512

// maxPatchChunkBytes keeps one patch chunk inside a single datagram, under a
// 1500 byte MTU with room for IP/UDP and the Packet around it.
//
// A chunk always carries at least one patch even if that patch busts the
// budget. Patches are indivisible, and one of them fragmenting beats the
// whole list doing so.
const maxPatchChunkBytes = 1000

// spectatorSessionRetention is how long a closed session's assembled log is
// kept around after the battle ends, so a late reconciliation pass or a
// spectator's final backfill can still read it.
const spectatorSessionRetention = 10 * time.Minute

// How long a session with no push from any peer is kept before it is treated
// as dead. Peers push continuously while a battle runs; the longest natural
// gap is a scene transition, seconds not minutes. Only reached when a session
// never closes, which needs every peer to vanish without reporting.
const spectatorSessionIdleTimeout = 10 * time.Minute

// How often the fan-out loop sweeps expired sessions. Sweeping only when a new
// battle starts leaves the last battles of a quiet night held until someone
// plays again.
const spectatorSweepInterval = time.Minute

// silentPushGrace is how many pushes in a row may go unacked before a
// subscriber is tapered. Ordinary packet loss heals in a push or two, since
// every push is cumulative, so the grace window keeps a healthy spectator
// that dropped a datagram from ever being slowed down.
const silentPushGrace = 8

// maxSilentBackoffShift caps the taper at 1<<6 = 64 skipped fanouts, about a
// second. Past the grace window each further unacked push doubles the wait:
// 2, 4, 8, 16, 32, then 64 until the subscriber acks or is swept.
//
// This is a receiver-liveness cutoff, not congestion control. It estimates no
// RTT and retransmits nothing - a skipped push costs nothing because the next
// one carries the current state anyway. It only stops us pushing ~1KB 60 times
// a second at something that has gone quiet, which without it we would keep
// doing for the full spectatorSubscriberTimeout. That matters most when the
// silence is correlated, since one browned-out uplink silences every spectator
// behind it at once.
const maxSilentBackoffShift = 6

// spectatorSubscriberTimeout is how long a downlink subscriber may go
// without resending SpectatorSubscribeRequest (its keepalive) before it is
// dropped. There is no TCP-level "connection closed" signal on this UDP
// channel, so staleness is the only way to notice a spectator went away.
const spectatorSubscriberTimeout = 10 * time.Second

// spectatorFanoutInterval is how often SpectatorRegistry.StartFanoutLoop
// resends each subscriber's unacked tail, in case a push (or its ack) was
// lost. New data is also pushed promptly via the loop's wake channel rather
// than waiting for the next tick.
const spectatorFanoutInterval = 50 * time.Millisecond

// SpectatorSession holds the live battle log for one in-progress P2P battle,
// keyed by battle_code. All 4 clients feed it redundantly over LBS's UDP
// channel. The result has the same shape as the BattleLogFile a client would
// upload at battle end.
type SpectatorSession struct {
	mtx sync.RWMutex

	battleCode string
	sessionID  int32

	log *proto.BattleLogFile

	// pendingFrames holds input frames received ahead of the current
	// contiguous frontier (log.Inputs), keyed by frame index, until the
	// gap in front of them is filled. Entries are folded into log.Inputs
	// and removed as soon as they become contiguous. The receive window
	// limits this map to maxSpectatorPendingFrames positions.
	pendingFrames map[int32]uint64

	// roundEventSeen dedups SpectatorRoundEvent pushes: all 4 peers send
	// the same event, keyed by the frame it occurred at.
	roundEventSeen map[int32]bool

	// roundStateVersion increments every time StartMsgIndexes/StartMsgRandoms/
	// RoundData changes, so the fan-out loop (buildPush) only needs to
	// piggyback round state on a subscriber's push when that subscriber
	// hasn't already been sent the latest version - see downlinkSubscriber.
	roundStateVersion int32

	// downlinks holds one entry per connected spectator (see
	// SpectatorRegistry.HandleSubscribe), keyed by the subscriber's UDP
	// source address (remoteAddr.String()).
	downlinks map[string]*downlinkSubscriber

	closed   bool
	closedAt time.Time

	// Last time the peers pushed anything. Close() depends on a
	// P2PMatchingReport arriving, and if every peer dies at once none does -
	// so without this a session that never closes is never freed.
	lastPushAt time.Time
}

// downlinkSubscriber tracks one spectator's live-push state within a
// SpectatorSession. All fields are guarded by the owning SpectatorSession's
// mtx (same lock PushInputs et al. already use).
type downlinkSubscriber struct {
	remoteAddr *net.UDPAddr

	// ackedFrame is the highest frame this subscriber received with no gaps
	// before it. Resends are just log.Inputs[ackedFrame:], capped at
	// maxSpectatorPushFrames. No separate backlog buffer is needed here -
	// log.Inputs already is one.
	ackedFrame int32

	// sentHeader tracks whether this subscriber has been sent the one-off
	// BattleLogFile header it needs before it can start playback at all.
	sentHeader bool

	// ackedPatches is how many patches this subscriber has received, counting
	// contiguously from the start. Chunks resend from here, so a lost chunk
	// costs one chunk, not the whole list.
	ackedPatches int32

	// sentRoundStateVersion is the round-state version this subscriber has
	// already been sent, so buildPush only piggybacks round data again once
	// it has actually changed.
	sentRoundStateVersion int32

	// probeDue lets a validated keepalive trigger a push through the silent
	// backoff, so a spectator whose acks were lost can recover.
	probeDue bool

	// silentPushes counts pushes sent since this subscriber last acked. Past
	// silentPushGrace it drives the taper - see maxSilentBackoffShift.
	silentPushes int32

	// skipFanouts is how many fanouts to pass over before pushing again.
	skipFanouts int32

	lastSeen time.Time
}

// shouldSkipPush reports whether this fanout should pass over the subscriber
// because it has gone quiet, consuming one unit of the wait if so. A pending
// probe always pushes through, so a keepalive subscribe restores full rate.
func (sub *downlinkSubscriber) shouldSkipPush() bool {
	if sub.probeDue || sub.skipFanouts <= 0 {
		return false
	}
	sub.skipFanouts--
	return true
}

// notePushSent records that a push went out and lengthens the wait if the
// subscriber still has not acked. Waits double past silentPushGrace and hold
// at 1<<maxSilentBackoffShift.
func (sub *downlinkSubscriber) notePushSent() {
	sub.silentPushes++
	if sub.silentPushes < silentPushGrace {
		return
	}
	shift := sub.silentPushes - silentPushGrace + 1
	if maxSilentBackoffShift < shift {
		shift = maxSilentBackoffShift
	}
	sub.skipFanouts = 1 << shift
}

func newSpectatorSession(matching *proto.P2PMatching, gameDisk string, patches *proto.GamePatchList) *SpectatorSession {
	log := &proto.BattleLogFile{
		GameDisk:       gameDisk,
		BattleCode:     matching.GetBattleCode(),
		LogFileVersion: 20250101,
		RuleBin:        matching.GetRuleBin(),
		Users:          matching.GetUsers(),
		StartAt:        time.Now().Unix(),
	}
	if patches != nil {
		log.Patches = patches.GetPatches()
	}
	return &SpectatorSession{
		battleCode:     matching.GetBattleCode(),
		sessionID:      matching.GetSessionId(),
		log:            log,
		pendingFrames:  make(map[int32]uint64),
		roundEventSeen: make(map[int32]bool),
		downlinks:      make(map[string]*downlinkSubscriber),
		lastPushAt:     time.Now(),
	}
}

// PushInputs folds a peer's backlog into the session's contiguous input log,
// deduping by frame index - first arrival across the redundant streams wins.
//
// Returns the next frame the session still needs, which the caller sends back
// so the peer can evict acked entries, and whether the log actually grew, so
// the caller knows whether to wake the fanout loop.
func (s *SpectatorSession) PushInputs(startFrame int32, inputs []uint64) (ackFrame int32, advanced bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	before := len(s.log.Inputs)
	endFrame := int64(startFrame) + int64(len(inputs))
	// Keep both frame indexes and the next-frame ACK representable as int32.
	if s.closed || startFrame < 0 || len(inputs) == 0 || endFrame > math.MaxInt32 {
		return int32(before), false
	}

	// Only out-of-order input needs pending storage. Validate its whole range.
	if int64(startFrame) > int64(before) && endFrame > int64(before)+maxSpectatorPendingFrames {
		return int32(before), false
	}

	s.lastPushAt = time.Now()

	for i, v := range inputs {
		f := startFrame + int32(i)
		if f < int32(len(s.log.Inputs)) {
			continue // already folded into the contiguous log
		}
		if f == int32(len(s.log.Inputs)) {
			// Append directly, preserving any earlier buffered value.
			if pending, exists := s.pendingFrames[f]; exists {
				v = pending
				delete(s.pendingFrames, f)
			}
			s.log.Inputs = append(s.log.Inputs, v)
		} else if _, exists := s.pendingFrames[f]; !exists {
			s.pendingFrames[f] = v
		}
	}

	for {
		next := int32(len(s.log.Inputs))
		v, ok := s.pendingFrames[next]
		if !ok {
			break
		}
		s.log.Inputs = append(s.log.Inputs, v)
		delete(s.pendingFrames, next)
	}

	return int32(len(s.log.Inputs)), len(s.log.Inputs) > before
}

// PushRoundEvent records a round-start RNG seed (mirrors
// GdxsvBackendRollback's start_msg_indexes_/start_msg_randoms_), deduped by
// frame since all 4 peers send the same event.
func (s *SpectatorSession) PushRoundEvent(frame int32, randomValue uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.lastPushAt = time.Now()

	if s.closed || s.roundEventSeen[frame] {
		return
	}
	s.roundEventSeen[frame] = true
	s.log.StartMsgIndexes = append(s.log.StartMsgIndexes, frame)
	s.log.StartMsgRandoms = append(s.log.StartMsgRandoms, randomValue)
	s.log.RoundData = append(s.log.RoundData, &proto.BattleLogRound{})
	s.roundStateVersion++
}

// PushRoundResult records a round's outcome once it becomes known. Mirrors
// the client's "first WinTeam transition wins" semantics: once a round's
// result is set it is not overwritten by a later (redundant) push.
func (s *SpectatorSession) PushRoundResult(roundIndex int32, round *proto.BattleLogRound) {
	if roundIndex < 0 || roundIndex >= maxSpectatorRounds || round == nil {
		return
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.lastPushAt = time.Now()

	if s.closed {
		return
	}
	for int32(len(s.log.RoundData)) <= roundIndex {
		s.log.RoundData = append(s.log.RoundData, &proto.BattleLogRound{})
	}
	if s.log.RoundData[roundIndex].GetWinTeam() == 0 {
		s.log.RoundData[roundIndex] = round
		s.roundStateVersion++
	}
}

// Close marks the session's battle as finished. Idempotent: only the first
// call records the reason, since multiple peers report the same battle end
// redundantly (mirroring how P2PMatchingReport already arrives from all 4
// clients today).
func (s *SpectatorSession) Close(reason string, disconnectPeerID int32) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.closed {
		return
	}
	s.closed = true
	s.closedAt = time.Now()
	s.log.CloseReason = reason
	s.log.DisconnectUserIndex = disconnectPeerID
	s.log.EndAt = s.closedAt.Unix()
}

// subscribeLocked installs or refreshes an admitted subscriber. The caller
// holds both registry and session locks and has checked its cookie and limits.
// Stale resume points never undo progress recorded by a later ack.
func (s *SpectatorSession) subscribeLocked(remoteAddr *net.UDPAddr, fromFrame int32, now time.Time) {
	key := remoteAddr.String()
	sub, exists := s.downlinks[key]
	if !exists {
		sub = &downlinkSubscriber{remoteAddr: remoteAddr}
		s.downlinks[key] = sub
		logger.Info("spectator subscribed",
			zap.String("battle_code", s.battleCode),
			zap.String("addr", key),
			zap.Int32("from_frame", fromFrame),
			zap.Int("subscribers", len(s.downlinks)))
	}
	if !exists || fromFrame > sub.ackedFrame {
		sub.ackedFrame = fromFrame
	}
	if fromFrame == 0 {
		// Still bootstrapping: re-arm the header. It is sent optimistically
		// and never retransmitted like inputs are, so a single lost header
		// datagram would otherwise strand this subscriber forever - it stays
		// registered, so re-subscribing would not produce another one. A
		// client that has folded anything in asks from a non-zero frame, so
		// this stops on its own.
		sub.sentHeader = false
	}
	sub.probeDue = true
	sub.lastSeen = now
}

// Ack advances an existing subscriber's receive position. Address validation
// happens at admission; an ACK alone cannot create a subscription or prove
// that its UDP source address is genuine.
func (s *SpectatorSession) Ack(addrKey string, ackFrame, patchAck int32) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	sub, ok := s.downlinks[addrKey]
	if !ok {
		return
	}
	if ackFrame < 0 || int64(ackFrame) > int64(len(s.log.Inputs)) ||
		patchAck < 0 || int64(patchAck) > int64(len(s.log.Patches)) {
		return
	}
	// Receipt feedback clears the taper; a keepalive only requests one probe.
	sub.silentPushes = 0
	sub.skipFanouts = 0
	if ackFrame > sub.ackedFrame {
		sub.ackedFrame = ackFrame
	}
	if patchAck > sub.ackedPatches {
		sub.ackedPatches = patchAck
	}
	sub.lastSeen = time.Now()
}

// buildPush constructs the next SpectatorInputPush to send a subscriber, or
// returns ok=false if there is nothing new for it (already caught up on
// inputs, round state, and close). Round data is piggybacked optimistically -
// sentRoundStateVersion is updated here rather than waiting for the ack,
// since a redundant resend on the next tick (if this push is lost) is cheap
// and idempotent on the receiving end.
func (s *SpectatorSession) buildPush(sub *downlinkSubscriber) (*proto.SpectatorInputPush, bool) {
	// The header goes in its own push to stay clear of IP fragmentation.
	if !sub.sentHeader {
		header, ok := pb.Clone(s.log).(*proto.BattleLogFile)
		if !ok {
			return nil, false
		}
		// Inputs, round state and patches all stream through their own
		// fields; the header carries only the small remainder.
		header.Inputs = nil
		header.StartMsgIndexes = nil
		header.StartMsgRandoms = nil
		header.RoundData = nil
		header.Patches = nil
		sub.sentHeader = true
		return &proto.SpectatorInputPush{
			BattleCode: s.battleCode,
			StartFrame: sub.ackedFrame,
			Header:     header,
		}, true
	}

	// Patches next, before any inputs: the spectator cannot simulate a frame
	// correctly until it has applied them all.
	if sub.ackedPatches < int32(len(s.log.Patches)) {
		var (
			chunk []*proto.GamePatch
			bytes int
		)
		for i := sub.ackedPatches; i < int32(len(s.log.Patches)); i++ {
			p := s.log.Patches[i]
			n := pb.Size(p)
			if 0 < len(chunk) && maxPatchChunkBytes < bytes+n {
				break
			}
			chunk = append(chunk, p)
			bytes += n
		}
		return &proto.SpectatorInputPush{
			BattleCode: s.battleCode,
			StartFrame: sub.ackedFrame,
			Patches:    chunk,
			PatchStart: sub.ackedPatches,
			PatchTotal: int32(len(s.log.Patches)),
		}, true
	}

	have := int32(len(s.log.Inputs))
	end := have
	if have-sub.ackedFrame > maxSpectatorPushFrames {
		end = sub.ackedFrame + maxSpectatorPushFrames
	}

	needsInputs := sub.ackedFrame < end
	needsRoundState := sub.sentRoundStateVersion != s.roundStateVersion
	// Only once this subscriber holds every input. The spectator stops its
	// downlink on close, so sending it early truncates the match to whatever
	// had arrived. Resent every fanout because close is never acked.
	needsClose := s.closed && sub.ackedFrame >= int32(len(s.log.Inputs))

	if !needsInputs && !needsRoundState && !needsClose {
		return nil, false
	}

	push := &proto.SpectatorInputPush{
		BattleCode: s.battleCode,
		StartFrame: sub.ackedFrame,
	}
	if needsInputs {
		push.Inputs = append([]uint64(nil), s.log.Inputs[sub.ackedFrame:end]...)
	}
	if needsRoundState {
		push.StartMsgIndexes = append([]int32(nil), s.log.StartMsgIndexes...)
		push.StartMsgRandoms = append([]uint64(nil), s.log.StartMsgRandoms...)
		push.RoundData = append([]*proto.BattleLogRound(nil), s.log.RoundData...)
		sub.sentRoundStateVersion = s.roundStateVersion
	}
	if needsClose {
		push.CloseReason = s.log.CloseReason
		push.DisconnectUserIndex = s.log.DisconnectUserIndex
	}

	return push, true
}

// sweepSubscribersLocked drops downlink subscribers that haven't sent a
// SpectatorSubscribeRequest (their keepalive) within spectatorSubscriberTimeout,
// returning their address keys so the caller (SpectatorRegistry) can also
// remove them from its own subscriberIndex.
func (s *SpectatorSession) sweepSubscribersLocked(now time.Time) []string {
	var dropped []string
	for key, sub := range s.downlinks {
		if now.Sub(sub.lastSeen) > spectatorSubscriberTimeout {
			delete(s.downlinks, key)
			dropped = append(dropped, key)
			// acked_frame against the log length is how far behind the
			// spectator was when it went silent, which distinguishes a
			// clean exit from one that could not keep up.
			logger.Info("spectator dropped",
				zap.String("battle_code", s.battleCode),
				zap.String("addr", key),
				zap.Int32("acked_frame", sub.ackedFrame),
				zap.Int("log_frames", len(s.log.Inputs)),
				zap.Duration("silent_for", now.Sub(sub.lastSeen)),
				zap.Int("subscribers", len(s.downlinks)))
		}
	}
	return dropped
}

// Snapshot returns a deep copy of the session's live-assembled log, safe
// for the caller to read or serialize without racing further pushes.
func (s *SpectatorSession) Snapshot() *proto.BattleLogFile {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return pb.Clone(s.log).(*proto.BattleLogFile)
}

// SpectatorRegistry tracks one SpectatorSession per currently (or recently)
// live battle_code. It is independent of Lbs's central single-threaded
// event loop (see Lbs.Locked) by design: input pushes arrive at up to
// 60/sec per peer and must not queue behind unrelated lobby/matchmaking
// work, so the registry guards its own state with its own lock instead.
type SpectatorRegistry struct {
	// Operations touching both maps lock the registry before the session.
	mtx      sync.RWMutex
	sessions map[string]*SpectatorSession

	// subscriberIndex resolves an inbound SpectatorInputAck to the session
	// it belongs to, purely by the packet's source address - SpectatorInputAck
	// carries no session_id to look up by. Kept in sync with each session's
	// own downlinks map by HandleSubscribe and StartFanoutLoop's sweep.
	subscriberIndex map[string]*SpectatorSession

	cookieKey [32]byte
	// Set before the server starts; cookies do not limit reachable clients.
	maxSubscribers          int
	maxSubscribersPerBattle int

	// wake is nudged (non-blocking) whenever a session gains new data a
	// subscriber might want, so StartFanoutLoop can deliver it promptly
	// instead of waiting for its next tick.
	wake chan struct{}
}

var spectatorRegistry = newSpectatorRegistry()

func newSpectatorRegistry() *SpectatorRegistry {
	r := &SpectatorRegistry{
		sessions:                make(map[string]*SpectatorSession),
		subscriberIndex:         make(map[string]*SpectatorSession),
		maxSubscribers:          defaultSpectatorMaxSubscribers,
		maxSubscribersPerBattle: defaultSpectatorMaxSubscribersPerBattle,
		wake:                    make(chan struct{}, 1),
	}
	if _, err := rand.Read(r.cookieKey[:]); err != nil {
		panic("cannot initialize spectator cookie secret: " + err.Error())
	}
	return r
}

// wakeFanout nudges the fan-out loop without blocking if it's already
// pending a wake.
func (r *SpectatorRegistry) wakeFanout() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// HandleSubscribe challenges a sender statelessly before admitting it to
// either subscriber map. Spectating is public and needs no player credentials.
func (r *SpectatorRegistry) HandleSubscribe(remoteAddr *net.UDPAddr, m *proto.SpectatorSubscribeRequest) *proto.SpectatorSubscribeChallenge {
	if remoteAddr == nil || remoteAddr.IP.To16() == nil || remoteAddr.Port <= 0 || remoteAddr.Port > 65535 ||
		m.GetFromFrame() < 0 || len(m.GetCookie()) != spectatorCookieBytes {
		return nil
	}
	if _, ok := r.GetAny(m.GetBattleCode()); !ok {
		return nil
	}
	now := time.Now()
	if !r.validSubscribeCookie(m.GetBattleCode(), remoteAddr, m.GetCookie(), now) {
		return &proto.SpectatorSubscribeChallenge{
			BattleCode: m.GetBattleCode(),
			Cookie: r.subscribeCookie(m.GetBattleCode(), remoteAddr,
				now.Unix()/int64(spectatorCookieWindow/time.Second)),
		}
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	s, ok := r.sessions[m.GetBattleCode()]
	if !ok {
		return nil
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if int64(m.GetFromFrame()) > int64(len(s.log.Inputs)) {
		return nil
	}
	key := remoteAddr.String()
	previous := r.subscriberIndex[key]
	if s.downlinks[key] == nil {
		if len(s.downlinks) >= r.maxSubscribersPerBattle ||
			(previous == nil && len(r.subscriberIndex) >= r.maxSubscribers) {
			return nil
		}
	}
	// One endpoint watches one battle. Moving it must not leave a second
	// stream behind or let the server-wide ceiling be bypassed.
	if previous != nil && previous != s {
		previous.mtx.Lock()
		delete(previous.downlinks, key)
		previous.mtx.Unlock()
	}
	s.subscribeLocked(remoteAddr, m.GetFromFrame(), now)
	r.subscriberIndex[key] = s

	r.wakeFanout()
	return nil
}

// HandleAck routes an inbound SpectatorInputAck (sent by a spectator, not a
// streaming peer - see SpectatorInputAck's doc) to the right session by the
// packet's source address. A no-op if the address isn't a known subscriber.
func (r *SpectatorRegistry) HandleAck(remoteAddr *net.UDPAddr, m *proto.SpectatorInputAck) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	s, ok := r.subscriberIndex[remoteAddr.String()]

	if !ok || s.battleCode != m.GetBattleCode() {
		return
	}
	s.Ack(remoteAddr.String(), m.GetAckFrame(), m.GetPatchAck())
}

// StartFanoutLoop sends admitted subscribers their missing data off the UDP
// read loop, driven by a ticker and new-input wakes. Small stateless cookie
// challenges are answered directly by the read loop.
func (r *SpectatorRegistry) StartFanoutLoop(udpConn *net.UDPConn) {
	ticker := time.NewTicker(spectatorFanoutInterval)
	defer ticker.Stop()
	sweep := time.NewTicker(spectatorSweepInterval)
	defer sweep.Stop()

	for {
		select {
		case <-ticker.C:
		case <-sweep.C:
			r.mtx.Lock()
			r.sweepLocked()
			r.mtx.Unlock()
			continue
		case <-r.wake:
		}
		r.fanoutOnce(udpConn)
	}
}

func (r *SpectatorRegistry) fanoutOnce(udpConn *net.UDPConn) {
	now := time.Now()

	r.mtx.RLock()
	sessions := make([]*SpectatorSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	r.mtx.RUnlock()

	for _, s := range sessions {
		r.mtx.Lock()
		s.mtx.Lock()
		for _, key := range s.sweepSubscribersLocked(now) {
			if r.subscriberIndex[key] == s {
				delete(r.subscriberIndex, key)
			}
		}
		r.mtx.Unlock()
		for _, sub := range s.downlinks {
			// A subscriber that has gone quiet is tapered rather than pushed
			// at full rate until it times out.
			if sub.shouldSkipPush() {
				continue
			}
			push, ok := s.buildPush(sub)
			if !ok {
				continue
			}
			sub.probeDue = false
			sub.notePushSent()
			pkt := &proto.Packet{
				Type:                   proto.MessageType_SpectatorInputPushType,
				SpectatorInputPushData: push,
			}
			bin, err := pb.Marshal(pkt)
			if err != nil {
				logger.Warn("spectator downlink push marshal failed", zap.Error(err))
				continue
			}
			if _, err := udpConn.WriteToUDP(bin, sub.remoteAddr); err != nil {
				logger.Warn("spectator downlink push send failed", zap.Error(err))
			}
		}
		s.mtx.Unlock()
	}
}

func handleSpectatorSubscribe(udpConn *net.UDPConn, remoteAddr *net.UDPAddr, m *proto.SpectatorSubscribeRequest) {
	challenge := spectatorRegistry.HandleSubscribe(remoteAddr, m)
	if challenge == nil {
		return
	}
	pkt := &proto.Packet{
		Type:                            proto.MessageType_SpectatorSubscribeChallengeType,
		SpectatorSubscribeChallengeData: challenge,
	}
	bin, err := pb.Marshal(pkt)
	if err != nil {
		logger.Warn("spectator challenge marshal failed", zap.Error(err))
		return
	}
	if _, err := udpConn.WriteToUDP(bin, remoteAddr); err != nil {
		logger.Warn("spectator challenge send failed", zap.Error(err))
	}
}

// Open seeds a new live-capture session for a battle that is about to
// start, using the same matching metadata LBS already generated for the
// participants (see LbsLobby.makeP2PMatchingMsg). Training games are
// skipped, mirroring GdxsvBackendRollback::SaveReplay()'s own skip. Safe to
// call multiple times for the same battle_code; later calls are no-ops.
func (r *SpectatorRegistry) Open(matching *proto.P2PMatching, gameDisk string, patches *proto.GamePatchList) {
	if matching.GetIsTrainingGame() || matching.GetBattleCode() == "" {
		return
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.sweepLocked()

	if _, ok := r.sessions[matching.GetBattleCode()]; ok {
		return
	}
	r.sessions[matching.GetBattleCode()] = newSpectatorSession(matching, gameDisk, patches)
}

// Get returns the session for battleCode, if one exists and its
// session_id matches (a lightweight guard against a stray/spoofed
// battle_code, not a real auth mechanism).
func (r *SpectatorRegistry) Get(battleCode string, sessionID int32) (*SpectatorSession, bool) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	s, ok := r.sessions[battleCode]
	if !ok || s.sessionID != sessionID {
		return nil, false
	}
	return s, true
}

// GetAny returns the session for battleCode without checking session_id.
// For callers that have no peer-supplied session_id to check against.
func (r *SpectatorRegistry) GetAny(battleCode string) (*SpectatorSession, bool) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()

	s, ok := r.sessions[battleCode]
	return s, ok
}

// LiveStatus reports whether battleCode can be spectated live right now, and
// how many spectators are attached.
//
// Producing, not merely open: a session is created for every battle, but only
// peers running a build with the spectator uplink actually push. One such peer
// is enough - each pushes the whole input log, and the session dedups by frame
// - so the answer is simply whether any input has arrived.
func (r *SpectatorRegistry) LiveStatus(battleCode string) (live bool, spectators int) {
	r.mtx.RLock()
	s, ok := r.sessions[battleCode]
	r.mtx.RUnlock()
	if !ok {
		return false, 0
	}

	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return !s.closed && 0 < len(s.log.Inputs), len(s.downlinks)
}

// Close marks the session for battleCode closed, if one exists.
func (r *SpectatorRegistry) Close(battleCode, reason string, disconnectPeerID int32) {
	r.mtx.RLock()
	s, ok := r.sessions[battleCode]
	r.mtx.RUnlock()

	if ok {
		s.Close(reason, disconnectPeerID)
		r.wakeFanout()
	}
}

// handleSpectatorInputPush processes one SpectatorInputPush datagram
// received on LBS's shared UDP channel (see Lbs.serveUDP) and acks back to
// the sender's source address.
func handleSpectatorInputPush(udpConn *net.UDPConn, remoteAddr *net.UDPAddr, m *proto.SpectatorInputPush) {
	if m.GetBattleCode() == "" || len(m.GetInputs()) == 0 || len(m.GetInputs()) > maxSpectatorPushFrames {
		return
	}

	s, ok := spectatorRegistry.Get(m.GetBattleCode(), m.GetSessionId())
	if !ok {
		return
	}
	ackFrame, advanced := s.PushInputs(m.GetStartFrame(), m.GetInputs())
	if advanced {
		spectatorRegistry.wakeFanout()
	}

	ack := &proto.Packet{
		Type: proto.MessageType_SpectatorInputAckType,
		SpectatorInputAckData: &proto.SpectatorInputAck{
			BattleCode: m.GetBattleCode(),
			AckFrame:   ackFrame,
		},
	}
	bin, err := pb.Marshal(ack)
	if err != nil {
		logger.Warn("spectator input ack marshal failed", zap.Error(err))
		return
	}
	if _, err := udpConn.WriteToUDP(bin, remoteAddr); err != nil {
		logger.Warn("spectator input ack send failed", zap.Error(err))
	}
}

// handleSpectatorRoundEvent processes one SpectatorRoundEvent datagram.
func handleSpectatorRoundEvent(m *proto.SpectatorRoundEvent) {
	s, ok := spectatorRegistry.Get(m.GetBattleCode(), m.GetSessionId())
	if !ok {
		return
	}
	s.PushRoundEvent(m.GetFrame(), m.GetRandomValue())
	spectatorRegistry.wakeFanout()
}

// handleSpectatorRoundResult processes one SpectatorRoundResult datagram.
func handleSpectatorRoundResult(m *proto.SpectatorRoundResult) {
	s, ok := spectatorRegistry.Get(m.GetBattleCode(), m.GetSessionId())
	if !ok {
		return
	}
	s.PushRoundResult(m.GetRoundIndex(), m.GetRound())
	spectatorRegistry.wakeFanout()
}

// sweepLocked drops sessions that closed more than spectatorSessionRetention
// ago, and sessions that have gone silent without ever closing. Called from
// the fan-out loop's own timer and from Open. Session churn is low - one entry
// per battle - so the O(n) scan is cheap.
func (r *SpectatorRegistry) sweepLocked() {
	now := time.Now()
	for code, s := range r.sessions {
		s.mtx.Lock()
		expired := s.closed && now.Sub(s.closedAt) > spectatorSessionRetention
		// A session only closes when a peer reports the battle ended. If every
		// peer vanishes at once nobody reports, so fall back to silence.
		idle := !s.closed && now.Sub(s.lastPushAt) > spectatorSessionIdleTimeout
		if expired || idle {
			if idle {
				logger.Info("spectator session dropped after going silent",
					zap.String("battle_code", code),
					zap.Duration("silent_for", now.Sub(s.lastPushAt)))
			}
			delete(r.sessions, code)
			for key := range s.downlinks {
				if r.subscriberIndex[key] == s {
					delete(r.subscriberIndex, key)
				}
				delete(s.downlinks, key)
			}
		}
		s.mtx.Unlock()
	}
}
