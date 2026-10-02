package main

import (
	"math"
	"time"

	"go.uber.org/zap"
)

// A participant uploads its own confirmed inputs, indexed by its own input
// count. Those counts differ slightly between peers: the game sends a KeyMsg1
// between rounds that a peer taking a timesync skip there never records. So
// the recording follows one participant and uses its indexes and round starts
// as they are. P2PMatching.spectator_uplink asks only the one nearest lbs to
// upload; clients from before it upload regardless. Either way the recording
// follows the first whose upload arrives, and the others are acknowledged so
// they stop resending, and dropped.

// pickSpectatorUplink returns the participant with the lowest latency to
// region, the GCP region nearest lbs. Unmeasured participants come last, and
// ties go to the lower index.
func pickSpectatorUplink(participants []*LbsPeer, region string) int {
	best, bestRtt := 0, 0
	for i, p := range participants {
		rtt := gcpLatency(p, region)
		if 0 < rtt && (bestRtt == 0 || rtt < bestRtt) {
			best, bestRtt = i, rtt
		}
	}
	return best
}

// The publisher's last inputs can arrive after another participant's close
// report. They are still accepted, and the close held back from spectators,
// for this long. Clients drain their uplink for up to two seconds first.
const spectatorPublisherCloseGrace = 3 * time.Second

// publisherLocked reports whether address is the participant the recording
// follows, choosing it if there is none yet.
func (s *SpectatorSession) publisherLocked(address string) bool {
	if s.publisher == "" {
		s.publisher = address
		logger.Info("spectator publisher selected",
			zap.String("battle_code", s.battleCode), zap.String("publisher", address))
	}
	return s.publisher == address
}

func (s *SpectatorSession) acceptsPublisherLocked(now time.Time) bool {
	return !s.closed || now.Before(s.closedAt.Add(spectatorPublisherCloseGrace))
}

// publisherFinishedLocked reports whether the publisher can send nothing more,
// so spectators may be told the battle closed.
func (s *SpectatorSession) publisherFinishedLocked(now time.Time) bool {
	return s.closed && (s.publisher == "" || !now.Before(s.closedAt.Add(spectatorPublisherCloseGrace)))
}

// PushParticipantInputs handles one participant's input upload. It returns
// the ACK for that participant: the recording's contiguous length for the
// publisher, everything sent for the others.
func (s *SpectatorSession) PushParticipantInputs(address string, startFrame int32, inputs []uint64) (ackFrame int32, advanced bool) {
	return s.pushParticipantInputsAt(address, startFrame, inputs, time.Now())
}

func (s *SpectatorSession) pushParticipantInputsAt(address string, startFrame int32, inputs []uint64, now time.Time) (int32, bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	end := int64(startFrame) + int64(len(inputs))
	if address == "" || startFrame < 0 || len(inputs) == 0 || end > math.MaxInt32 || !s.acceptsPublisherLocked(now) {
		return int32(len(s.log.Inputs)), false
	}
	if !s.publisherLocked(address) {
		return int32(end), false
	}
	return s.appendInputsLocked(startFrame, inputs)
}

// PushParticipantRoundEvent handles one participant's round start. It
// reports whether to acknowledge it.
func (s *SpectatorSession) PushParticipantRoundEvent(address string, frame int32, seed uint64) bool {
	return s.pushParticipantRoundEventAt(address, frame, seed, time.Now())
}

func (s *SpectatorSession) pushParticipantRoundEventAt(address string, frame int32, seed uint64, now time.Time) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if address == "" || frame < 0 {
		return false
	}
	if s.roundEventSeen[frame] && s.publisher == address {
		return true // Recover a lost ACK, including after close.
	}
	if !s.acceptsPublisherLocked(now) {
		return false
	}
	if !s.publisherLocked(address) {
		return true
	}
	return s.appendRoundStartLocked(frame, seed)
}
