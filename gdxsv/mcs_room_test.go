package main

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	pb "google.golang.org/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

// fakeMcsPeer is an in-process McsPeer that records relayed messages.
type fakeMcsPeer struct {
	BaseMcsPeer

	mtx  sync.Mutex
	recv []*proto.BattleMessage
}

func newFakeMcsPeer(userID, sessionID string) *fakeMcsPeer {
	p := &fakeMcsPeer{}
	p.userID = userID
	p.sessionID = sessionID
	p.logger = logger
	return p
}

func (p *fakeMcsPeer) AddSendData([]byte) {}

func (p *fakeMcsPeer) AddSendMessage(msg *proto.BattleMessage) {
	p.mtx.Lock()
	p.recv = append(p.recv, msg)
	p.mtx.Unlock()
}

func (p *fakeMcsPeer) received() []*proto.BattleMessage {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return append([]*proto.BattleMessage(nil), p.recv...)
}

func (p *fakeMcsPeer) Address() string { return "fake:" + p.userID }
func (p *fakeMcsPeer) Close() error    { return nil }

var _ McsPeer = (*fakeMcsPeer)(nil)

func newTestMcsGame(battleCode string) *McsGame {
	return &McsGame{
		BattleCode: battleCode,
		McsAddr:    "127.0.0.1:1234",
		GameDisk:   GameDiskDC2,
		RuleBin:    SerializeRule(&DefaultRule),
		PatchList:  &proto.GamePatchList{},
		UpdatedAt:  time.Now(),
	}
}

func newTestMcsUser(battleCode, userID, sessionID string, pos int) *McsUser {
	return &McsUser{
		BattleCode: battleCode,
		UserID:     userID,
		Name:       "name-" + userID,
		PilotName:  "pilot-" + userID,
		GameDisk:   GameDiskDC2,
		SessionID:  sessionID,
		Pos:        pos,
		Team:       uint16(pos / 2),
		UpdatedAt:  time.Now(),
	}
}

// newTestMcsRoom builds a room directly (bypassing sharedData) with n joined peers.
func newTestMcsRoom(t *testing.T, battleCode string, n int) (*Mcs, *McsRoom, []*fakeMcsPeer) {
	t.Helper()
	conf.BattleLogPath = t.TempDir()
	mcs := NewMcs(0)
	room := newMcsRoom(mcs, newTestMcsGame(battleCode))
	mcs.rooms[battleCode] = room

	peers := make([]*fakeMcsPeer, 0, n)
	for i := 0; i < n; i++ {
		userID := fmt.Sprintf("USER%d", i)
		sessionID := fmt.Sprintf("SESS%s%d", battleCode, i)
		p := newFakeMcsPeer(userID, sessionID)
		room.Join(p, newTestMcsUser(battleCode, userID, sessionID, i))
		peers = append(peers, p)
	}
	return mcs, room, peers
}

func mcsRoomCount(mcs *Mcs) int {
	mcs.mtx.Lock()
	defer mcs.mtx.Unlock()
	return len(mcs.rooms)
}

func waitRoomClosed(t *testing.T, mcs *Mcs) {
	t.Helper()
	waitFor(t, 3*time.Second, func() bool { return mcsRoomCount(mcs) == 0 })
}

func battleMsg(seq uint32, body string) *proto.BattleMessage {
	return &proto.BattleMessage{Seq: seq, UserId: "x", Body: []byte(body)}
}

func TestMcsRoom_JoinAssignsPositionsAndLogsUsers(t *testing.T) {
	_, room, peers := newTestMcsRoom(t, "ROOMJOIN", 4)

	assertEq(t, 4, room.PeerCount())
	assertEq(t, false, room.IsClosing())
	for i, p := range peers {
		assertEq(t, i, p.Position())
		assertEq(t, "ROOMJOIN", p.McsRoomID())
	}

	room.logMtx.RLock()
	users := room.battleLog.Users
	room.logMtx.RUnlock()
	assertEq(t, 4, len(users))
	for i, u := range users {
		assertEq(t, fmt.Sprintf("USER%d", i), u.UserId)
		assertEq(t, fmt.Sprintf("name-USER%d", i), u.UserName)
		assertEq(t, fmt.Sprintf("pilot-USER%d", i), u.PilotName)
		assertEq(t, int32(i), u.Pos)
		assertEq(t, int32(i/2), u.Team)
	}
}

func TestMcsRoom_SendMessageFansOutExceptSender(t *testing.T) {
	_, room, peers := newTestMcsRoom(t, "ROOMSEND", 4)

	msg := battleMsg(1, "hello")
	room.SendMessage(peers[1], msg)

	for i, p := range peers {
		got := p.received()
		if i == 1 {
			assertEq(t, 0, len(got))
			continue
		}
		assertEq(t, 1, len(got))
		assertEq(t, true, got[0] == msg)
	}

	room.logMtx.RLock()
	data := room.battleLog.BattleData
	room.logMtx.RUnlock()
	assertEq(t, 1, len(data))
	assertEq(t, true, data[0] == msg)

	// after peer 2 leaves, it must no longer receive anything
	room.Leave(peers[2])
	assertEq(t, true, room.IsClosing())
	msg2 := battleMsg(2, "second")
	room.SendMessage(peers[0], msg2)
	assertEq(t, 1, len(peers[2].received()))
	assertEq(t, 1, len(peers[1].received()))
	assertEq(t, 2, len(peers[3].received()))
	assertEq(t, 1, len(peers[0].received()))
}

func TestMcsRoom_SendMessageAfterFinalizeDoesNotPanic(t *testing.T) {
	mcs, room, peers := newTestMcsRoom(t, "ROOMFIN", 2)

	room.Finalize()
	waitRoomClosed(t, mcs)

	// A peer still draining after the room closed relays into a dead room.
	room.SendMessage(peers[0], battleMsg(1, "late"))
	assertEq(t, 0, len(peers[1].received()))
	assertEq(t, 0, room.PeerCount())
}

func TestMcsRoom_ConcurrentRelayDuringFinalize(t *testing.T) {
	mcs, room, peers := newTestMcsRoom(t, "ROOMRACE", 2)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for seq := uint32(0); ; seq++ {
			select {
			case <-stop:
				return
			default:
			}
			room.SendMessage(peers[0], battleMsg(seq, "spam"))
		}
	}()

	room.Leave(peers[1])
	room.Leave(peers[0])
	waitRoomClosed(t, mcs)
	// keep relaying a bit after the room is gone
	time.Sleep(20 * time.Millisecond)
	close(stop)
	<-done
}

func TestMcsRoom_LeaveOfUnjoinedPeerDoesNotEvictSlotZero(t *testing.T) {
	_, room, peers := newTestMcsRoom(t, "ROOMSTRAY", 2)

	stray := newFakeMcsPeer("STRAY", "SESSSTRAY")
	assertEq(t, 0, stray.Position()) // never joined => zero value

	room.Leave(stray)

	assertEq(t, 2, room.PeerCount())
	assertEq(t, false, room.IsClosing())
	room.mtx.RLock()
	assertEq(t, true, room.peers[0] == McsPeer(peers[0]))
	room.mtx.RUnlock()

	// peer 0 still receives relayed traffic
	room.SendMessage(peers[1], battleMsg(1, "still-here"))
	assertEq(t, 1, len(peers[0].received()))
}

func TestMcsRoom_DoubleLeaveFinalizesOnce(t *testing.T) {
	mcs, room, peers := newTestMcsRoom(t, "ROOMDOUBLE", 2)

	room.Leave(peers[0])
	assertEq(t, true, room.IsClosing())
	assertEq(t, 2, room.PeerCount())
	assertEq(t, 1, mcsRoomCount(mcs))

	// Second leave of the same peer: must not spawn a Finalize while peer 1
	// is still in the battle.
	room.Leave(peers[0])
	time.Sleep(20 * time.Millisecond)
	assertEq(t, 1, mcsRoomCount(mcs))
	assertEq(t, 2, room.PeerCount())

	room.Leave(peers[1])
	room.Leave(peers[1])
	waitRoomClosed(t, mcs)

	// Any extra Finalize would delete a room re-registered under the same code.
	again := newMcsRoom(mcs, newTestMcsGame("ROOMDOUBLE"))
	mcs.mtx.Lock()
	mcs.rooms["ROOMDOUBLE"] = again
	mcs.mtx.Unlock()
	room.Leave(peers[0])
	room.Leave(peers[1])
	time.Sleep(50 * time.Millisecond)
	assertEq(t, 1, mcsRoomCount(mcs))
	room.Finalize() // explicit re-finalize is a no-op
	assertEq(t, 1, mcsRoomCount(mcs))
}

func TestMcsRoom_IsClosingAndPeerCountAfterPartialLeaves(t *testing.T) {
	mcs, room, peers := newTestMcsRoom(t, "ROOMPARTIAL", 3)

	assertEq(t, 3, room.PeerCount())
	assertEq(t, false, room.IsClosing())

	room.Leave(peers[1])
	assertEq(t, 3, room.PeerCount()) // slots are kept, only nil-ed
	assertEq(t, true, room.IsClosing())
	assertEq(t, 1, mcsRoomCount(mcs))

	room.Leave(peers[0])
	assertEq(t, true, room.IsClosing())
	assertEq(t, 1, mcsRoomCount(mcs))

	room.Leave(peers[2])
	waitRoomClosed(t, mcs)
	assertEq(t, 0, room.PeerCount())
	assertEq(t, false, room.IsClosing())
}

func TestMcsRoom_FinalizeWritesSortedBattleLog(t *testing.T) {
	mcs, room, peers := newTestMcsRoom(t, "ROOMLOG", 4)

	// Users are appended in join order; scramble their Pos to verify sorting.
	room.logMtx.Lock()
	room.battleLog.Users[0].Pos = 3
	room.battleLog.Users[1].Pos = 1
	room.battleLog.Users[2].Pos = 2
	room.battleLog.Users[3].Pos = 0
	room.logMtx.Unlock()

	m1 := battleMsg(1, "a")
	m2 := battleMsg(2, "b")
	room.SendMessage(peers[0], m1)
	room.SendMessage(peers[3], m2)

	before := time.Now().UnixNano()
	for _, p := range peers {
		room.Leave(p)
	}
	waitRoomClosed(t, mcs)

	fileName := path.Join(conf.BattleLogPath, fmt.Sprintf("disk%v-%v.pb", GameDiskDC2, "ROOMLOG"))
	raw, err := os.ReadFile(fileName)
	must(t, err)

	var logFile proto.BattleLogFile
	must(t, pb.Unmarshal(raw, &logFile))

	assertEq(t, int32(20210803), logFile.GetLogFileVersion())
	assertEq(t, GameDiskDC2, logFile.GetGameDisk())
	assertEq(t, "ROOMLOG", logFile.GetBattleCode())
	assertEq(t, SerializeRule(&DefaultRule), logFile.GetRuleBin())
	assertEq(t, true, logFile.GetStartAt() <= before)
	assertEq(t, true, before <= logFile.GetEndAt())

	assertEq(t, 4, len(logFile.GetUsers()))
	for i, u := range logFile.GetUsers() {
		assertEq(t, int32(i), u.GetPos())
	}
	assertEq(t, "USER3", logFile.GetUsers()[0].GetUserId())
	assertEq(t, "USER0", logFile.GetUsers()[3].GetUserId())

	assertEq(t, 2, len(logFile.GetBattleData()))
	assertEq(t, uint32(1), logFile.GetBattleData()[0].GetSeq())
	assertEq(t, []byte("a"), logFile.GetBattleData()[0].GetBody())
	assertEq(t, uint32(2), logFile.GetBattleData()[1].GetSeq())
	assertEq(t, []byte("b"), logFile.GetBattleData()[1].GetBody())
}

func TestMcsJoin_UnknownSessionOrGame(t *testing.T) {
	conf.BattleLogPath = t.TempDir()
	mcs := NewMcs(0)

	t.Run("unknown session", func(t *testing.T) {
		p := newFakeMcsPeer("", "")
		assertEq(t, true, mcs.Join(p, "NOSUCHSESS") == nil)
		assertEq(t, 0, mcsRoomCount(mcs))
		assertEq(t, "", p.UserID())
	})

	t.Run("known session without game info", func(t *testing.T) {
		sharedData.ShareMcsUser(newTestMcsUser("NOGAME", "ORPHAN", "SESSORPHAN", 0))
		p := newFakeMcsPeer("", "")
		assertEq(t, true, mcs.Join(p, "SESSORPHAN") == nil)
		assertEq(t, 0, mcsRoomCount(mcs))
		assertEq(t, "", p.UserID())
		u, ok := sharedData.GetBattleUserInfo("SESSORPHAN")
		assertEq(t, true, ok)
		assertEq(t, McsUserStateCreated, u.State)
	})
}

func TestMcsJoin_FullLifecycle(t *testing.T) {
	conf.BattleLogPath = t.TempDir()
	mcs := NewMcs(0)
	battleCode := "MCSLIFE"

	sharedData.ShareMcsGame(newTestMcsGame(battleCode))
	peers := make([]*fakeMcsPeer, 0, 4)
	for i := 0; i < 4; i++ {
		userID := fmt.Sprintf("LIFE%d", i)
		sessionID := fmt.Sprintf("SESSLIFE%d", i)
		sharedData.ShareMcsUser(newTestMcsUser(battleCode, userID, sessionID, i))

		p := newFakeMcsPeer("", "")
		room := mcs.Join(p, sessionID)
		assertEq(t, true, room != nil)
		assertEq(t, 1, mcsRoomCount(mcs))
		assertEq(t, userID, p.UserID())
		assertEq(t, sessionID, p.SessionID())
		assertEq(t, i, p.Position())
		assertEq(t, battleCode, p.McsRoomID())
		assertEq(t, i+1, room.PeerCount())

		u, ok := sharedData.GetBattleUserInfo(sessionID)
		assertEq(t, true, ok)
		assertEq(t, McsUserStateJoined, u.State)
		peers = append(peers, p)
	}

	g, ok := sharedData.GetBattleGameInfo(battleCode)
	assertEq(t, true, ok)
	assertEq(t, McsGameStateOpened, g.State)

	room := mcs.rooms[battleCode]
	for i, p := range peers {
		p.SetCloseReason(fmt.Sprintf("reason%d", i))
		room.Leave(p)
		u, ok := sharedData.GetBattleUserInfo(p.SessionID())
		assertEq(t, true, ok)
		assertEq(t, McsUserStateLeft, u.State)
		assertEq(t, fmt.Sprintf("reason%d", i), u.CloseReason)
	}
	waitRoomClosed(t, mcs)

	g, ok = sharedData.GetBattleGameInfo(battleCode)
	assertEq(t, true, ok)
	assertEq(t, McsGameStateClosed, g.State)

	_, err := os.Stat(path.Join(conf.BattleLogPath, fmt.Sprintf("disk%v-%v.pb", GameDiskDC2, battleCode)))
	must(t, err)
}
