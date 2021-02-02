package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"go.uber.org/zap"
	"sync"
	"time"
)

// SharedData holds games and users in matching and shares the information between lbs and mcs.
type SharedData struct {
	sync.Mutex
	mcsUsers map[string]*McsUser // session_id -> user info
	mcsGames map[string]*McsGame // battle_code -> game info
}

var sharedData = SharedData{
	mcsUsers: map[string]*McsUser{},
	mcsGames: map[string]*McsGame{},
}

const (
	McsGameStateCreated = 0
	McsGameStateOpened  = 1
	McsGameStateClosed  = 2

	McsUserStateCreated = 0
	McsUserStateJoined  = 1
	McsUserStateLeft    = 2
)

type McsUser struct {
	BattleCode  string `json:"battle_code,omitempty"`
	McsRegion   string `json:"mcs_region,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	Name        string `json:"name,omitempty"`
	PilotName   string `json:"pilot_name,omitempty"`
	GameParam   []byte `json:"game_param,omitempty"`
	Platform    string `json:"platform"`
	GameDisk    string `json:"game_disk"`
	BattleCount int    `json:"battle_count,omitempty"`
	WinCount    int    `json:"win_count,omitempty"`
	LoseCount   int    `json:"lose_count,omitempty"`
	Team        uint16 `json:"team,omitempty"`
	SessionID   string `json:"session_id,omitempty"`

	State       int       `json:"state,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type McsGame struct {
	BattleCode string `json:"battle_code,omitempty"`
	McsAddr    string `json:"mcs_addr,omitempty"`
	GameDisk   string `json:"game_disk"`
	Rule       Rule  `json:"rule,omitempty"`

	State     int       `json:"state,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type McsStatus struct {
	Region     string     `json:"region,omitempty"`
	PublicAddr string     `json:"public_addr,omitempty"`
	Users      []*McsUser `json:"users,omitempty"`
	Games      []*McsGame `json:"games,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at,omitempty"`
}

type LbsStatus struct {
	McsUsers []*McsUser `json:"mcs_users,omitempty"`
	McsGames []*McsGame `json:"mcs_games,omitempty"`
}

func (s *SharedData) ShareMcsGame(g *McsGame) {
	s.Lock()
	defer s.Unlock()
	s.mcsGames[g.BattleCode] = g
}

func (s *SharedData) ShareMcsUser(u *McsUser) {
	s.Lock()
	defer s.Unlock()
	s.mcsUsers[u.SessionID] = u
}

func (s *SharedData) SyncMcsToLbs(status *McsStatus) {
	s.Lock()
	defer s.Unlock()

	for _, u := range status.Users {
		_, ok := s.mcsUsers[u.SessionID]
		if ok {
			s.mcsUsers[u.SessionID] = u
		}
	}

	for _, g := range status.Games {
		_, ok := s.mcsGames[g.BattleCode]
		if ok {
			s.mcsGames[g.BattleCode] = g
		}
	}
}

func (s *SharedData) SyncLbsToMcs(status *LbsStatus) {
	s.Lock()
	defer s.Unlock()

	activeBattleCodes := map[string]bool{}
	activeSessionIDs := map[string]bool{}

	for _, g := range status.McsGames {
		if g.McsAddr != conf.BattlePublicAddr {
			continue // not my game
		}
		activeBattleCodes[g.BattleCode] = true

		if _, ok := s.mcsGames[g.BattleCode]; ok {
			continue // already exist
		}
		s.mcsGames[g.BattleCode] = g
	}

	for _, u := range status.McsUsers {
		activeSessionIDs[u.SessionID] = true

		if _, ok := s.mcsUsers[u.SessionID]; ok {
			continue // already exist
		}
		s.mcsUsers[u.SessionID] = u
	}

	for k, g := range s.mcsGames {
		if !activeBattleCodes[g.BattleCode] {
			delete(s.mcsGames, k)
		}
	}

	for k, u := range s.mcsUsers {
		if !activeSessionIDs[u.SessionID] {
			delete(s.mcsUsers, k)
		}
	}
}

func (s *SharedData) GetMcsUsers() []*McsUser {
	s.Lock()
	defer s.Unlock()

	var ret []*McsUser

	for _, u := range s.mcsUsers {
		v := *u
		ret = append(ret, &v)
	}

	return ret
}

func (s *SharedData) GetMcsGames() []*McsGame {
	s.Lock()
	defer s.Unlock()

	var ret []*McsGame

	for _, g := range s.mcsGames {
		h := *g
		ret = append(ret, &h)
	}

	return ret
}

func (s *SharedData) getLbsStatusFiltered(mcsAddr string) *LbsStatus {
	s.Lock()
	defer s.Unlock()

	st := new(LbsStatus)

	targetBattleCodes := map[string]bool{}

	for _, g := range s.mcsGames {
		if g.McsAddr == mcsAddr {
			st.McsGames = append(st.McsGames, g)
			targetBattleCodes[g.BattleCode] = true
		}
	}

	for _, u := range s.mcsUsers {
		if targetBattleCodes[u.BattleCode] {
			st.McsUsers = append(st.McsUsers, u)
		}
	}

	return st
}

func (s *SharedData) NotifyLatestLbsStatus(mcs *LbsPeer) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	jw := json.NewEncoder(zw)

	lbsStatus := s.getLbsStatusFiltered(mcs.mcsStatus.PublicAddr)
	logger.Info("NotifyLatestLbsStatus", zap.Any("lbs_status", lbsStatus), zap.String("public_addr", mcs.mcsStatus.PublicAddr))

	err := jw.Encode(lbsStatus)
	if err != nil {
		logger.Error("json encode failed", zap.Error(err))
		return
	}

	err = zw.Close()
	if err != nil {
		logger.Error("gzip close failed", zap.Error(err))
		return
	}

	if (1 << 16) <= buf.Len() {
		logger.Error("too large data", zap.Int("size", buf.Len()))
		return
	}

	mcs.SendMessage(NewServerNotice(lbsExtSyncSharedData).Writer().WriteBytes(buf.Bytes()).Msg())
}

func (s *SharedData) GetBattleGameInfo(battleCode string) (*McsGame, bool) {
	s.Lock()
	defer s.Unlock()
	g, ok := s.mcsGames[battleCode]
	return g, ok
}

func (s *SharedData) GetBattleUserInfo(sessionID string) (*McsUser, bool) {
	s.Lock()
	defer s.Unlock()
	u, ok := s.mcsUsers[sessionID]
	return u, ok
}

func (s *SharedData) UpdateMcsGameState(battleCode string, newState int) {
	s.Lock()
	defer s.Unlock()
	g, ok := s.mcsGames[battleCode]
	if ok && g.State < newState {
		logger.Info("UpdateMcsGameState",
			zap.String("battle_code", battleCode),
			zap.Int("from", g.State),
			zap.Int("to", newState))
		g.State = newState
		g.UpdatedAt = time.Now()
		s.mcsGames[battleCode] = g
	}
}

func (s *SharedData) UpdateMcsUserState(sessionID string, newState int) {
	s.Lock()
	defer s.Unlock()
	u, ok := s.mcsUsers[sessionID]
	if ok && u.State < newState {
		logger.Info("UpdateMcsUserState",
			zap.String("session_id", sessionID),
			zap.Int("from", u.State),
			zap.Int("to", newState))
		u.State = newState
		u.UpdatedAt = time.Now()
		s.mcsUsers[sessionID] = u
	}
}

func (s *SharedData) SetMcsUserCloseReason(sessionID string, closeReason string) {
	s.Lock()
	defer s.Unlock()
	if u, ok := s.mcsUsers[sessionID]; ok {
		if u.CloseReason == "" {
			u.CloseReason = closeReason
			s.mcsUsers[sessionID] = u
		}
	}
}

func (s *SharedData) RemoveStaleData() {
	s.Lock()
	defer s.Unlock()

	for key, u := range s.mcsUsers {
		if 1.0 <= time.Since(u.UpdatedAt).Hours() {
			delete(s.mcsUsers, key)
			logger.Warn("remove old zombie battle user", zap.String("session_id", key))
		}
	}

	for key, g := range s.mcsGames {
		if 1.0 <= time.Since(g.UpdatedAt).Hours() {
			delete(s.mcsGames, key)
			logger.Warn("remove old zombie game", zap.String("battle_code", key))
		}
	}

	for _, g := range s.mcsGames {
		if g.State == McsGameStateClosed {
			delete(s.mcsGames, g.BattleCode)
			logger.Info("remove mcs game", zap.String("battle_code", g.BattleCode))

			for _, u := range s.mcsUsers {
				if g.BattleCode == u.BattleCode {
					delete(s.mcsUsers, u.SessionID)
					logger.Info("remove mcs user",
						zap.String("session_id", u.SessionID),
						zap.String("user_id", u.UserID),
						zap.String("name", u.Name),
						zap.Time("updated_at", u.UpdatedAt),
						zap.Int("state", u.State),
						zap.String("close_reason", u.CloseReason))
				}
			}
		}
	}
}
