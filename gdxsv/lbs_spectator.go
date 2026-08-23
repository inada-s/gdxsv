package main

import (
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

// spectatorSessionRetention is how long a closed session's assembled log is
// kept around after the battle ends, so a late reconciliation pass or a
// spectator's final backfill can still read it.
const spectatorSessionRetention = 10 * time.Minute

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
	// and removed as soon as they become contiguous.
	pendingFrames map[int32]uint64

	// roundEventSeen dedups SpectatorRoundEvent pushes: all 4 peers send
	// the same event, keyed by the frame it occurred at.
	roundEventSeen map[int32]bool

	closed   bool
	closedAt time.Time
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
	}
}

// PushInputs folds a peer-reported backlog of confirmed frames into the
// session's contiguous input log, deduping by frame index (first arrival
// across any of the redundant peer streams wins). It returns the session's
// current ack frame - the next frame index the session still needs - which
// the caller should send back to the peer so it can evict acked entries
// from its own resend buffer.
func (s *SpectatorSession) PushInputs(startFrame int32, inputs []uint64) int32 {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.closed {
		return int32(len(s.log.Inputs))
	}

	for i, v := range inputs {
		f := startFrame + int32(i)
		if f < int32(len(s.log.Inputs)) {
			continue // already folded into the contiguous log
		}
		if _, exists := s.pendingFrames[f]; !exists {
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

	return int32(len(s.log.Inputs))
}

// PushRoundEvent records a round-start RNG seed (mirrors
// GdxsvBackendRollback's start_msg_indexes_/start_msg_randoms_), deduped by
// frame since all 4 peers send the same event.
func (s *SpectatorSession) PushRoundEvent(frame int32, randomValue uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.closed || s.roundEventSeen[frame] {
		return
	}
	s.roundEventSeen[frame] = true
	s.log.StartMsgIndexes = append(s.log.StartMsgIndexes, frame)
	s.log.StartMsgRandoms = append(s.log.StartMsgRandoms, randomValue)
	s.log.RoundData = append(s.log.RoundData, &proto.BattleLogRound{})
}

// PushRoundResult records a round's outcome once it becomes known. Mirrors
// the client's "first WinTeam transition wins" semantics: once a round's
// result is set it is not overwritten by a later (redundant) push.
func (s *SpectatorSession) PushRoundResult(roundIndex int32, round *proto.BattleLogRound) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.closed || roundIndex < 0 || round == nil {
		return
	}
	for int32(len(s.log.RoundData)) <= roundIndex {
		s.log.RoundData = append(s.log.RoundData, &proto.BattleLogRound{})
	}
	if s.log.RoundData[roundIndex].GetWinTeam() == 0 {
		s.log.RoundData[roundIndex] = round
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
	mtx      sync.RWMutex
	sessions map[string]*SpectatorSession
}

var spectatorRegistry = &SpectatorRegistry{
	sessions: make(map[string]*SpectatorSession),
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

// Close marks the session for battleCode closed, if one exists.
func (r *SpectatorRegistry) Close(battleCode, reason string, disconnectPeerID int32) {
	r.mtx.RLock()
	s, ok := r.sessions[battleCode]
	r.mtx.RUnlock()

	if ok {
		s.Close(reason, disconnectPeerID)
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
	ackFrame := s.PushInputs(m.GetStartFrame(), m.GetInputs())

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
}

// handleSpectatorRoundResult processes one SpectatorRoundResult datagram.
func handleSpectatorRoundResult(m *proto.SpectatorRoundResult) {
	s, ok := spectatorRegistry.Get(m.GetBattleCode(), m.GetSessionId())
	if !ok {
		return
	}
	s.PushRoundResult(m.GetRoundIndex(), m.GetRound())
}

// sweepLocked drops sessions that closed more than spectatorSessionRetention
// ago. Called opportunistically from Open rather than on a timer: session
// churn is low (one entry per battle), so an O(n) scan on every new battle
// start is cheap and avoids a background goroutine for M0.
func (r *SpectatorRegistry) sweepLocked() {
	now := time.Now()
	for code, s := range r.sessions {
		s.mtx.RLock()
		expired := s.closed && now.Sub(s.closedAt) > spectatorSessionRetention
		s.mtx.RUnlock()
		if expired {
			delete(r.sessions, code)
		}
	}
}
