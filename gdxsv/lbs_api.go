package main

import (
	"encoding/json"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var httpRequestGroup singleflight.Group

func (lbs *Lbs) RegisterHTTPHandlers() {
	teamName := func(team int) string {
		if team == TeamRenpo {
			return "renpo"
		}
		if team == TeamZeon {
			return "zeon"
		}
		return ""
	}

	gameStateName := func(state int) string {
		if state == McsGameStateCreated {
			return "created"
		}
		if state == McsGameStateOpened {
			return "opened"
		}
		if state == McsGameStateClosed {
			return "closed"
		}
		return "unknown"
	}

	http.HandleFunc("/lbs/status", func(w http.ResponseWriter, r *http.Request) {
		// Public API: get lobby status

		type onlineUser struct {
			UserID     string `json:"user_id,omitempty"`
			Name       string `json:"name,omitempty"`
			Team       string `json:"team,omitempty"`
			LobbyID    uint16 `json:"lobby_id,omitempty"`
			BattleCode string `json:"battle_code,omitempty"`
			BattlePos  uint8  `json:"battle_pos,omitempty"`
			Platform   string `json:"platform,omitempty"`
			Disk       string `json:"disk,omitempty"`
			Flycast    string `json:"flycast,omitempty"`
		}

		type activeGame struct {
			BattleCode string    `json:"battle_code,omitempty"`
			Region     string    `json:"region,omitempty"`
			Disk       string    `json:"disk,omitempty"`
			State      string    `json:"state,omitempty"`
			LobbyID    uint16    `json:"lobby_id,omitempty"`
			UpdatedAt  time.Time `json:"updated_at,omitempty"`
		}

		type statusResponse struct {
			LobbyUsers  []*onlineUser `json:"lobby_users"`
			BattleUsers []*onlineUser `json:"battle_users"`
			ActiveGames []*activeGame `json:"active_games"`
		}

		userAdded := map[string]bool{}

		resp, err, _ := httpRequestGroup.Do("/lbs/status", func() (interface{}, error) {
			resp := new(statusResponse)

			lbs.Locked(func(lbs *Lbs) {
				for _, u := range lbs.userPeers {
					if !userAdded[u.UserID] {
						userAdded[u.UserID] = true
						lobbyID := uint16(0)
						if u.Lobby != nil {
							lobbyID = u.Lobby.ID
						}
						user := &onlineUser{
							UserID:     u.UserID,
							Name:       u.Name,
							Team:       teamName(int(u.Team)),
							LobbyID:    lobbyID,
							BattleCode: "",
							Platform:   u.Platform,
							Disk:       u.GameDisk,
							Flycast:    u.PlatformInfo["flycast"],
						}

						if u.logout && u.Battle != nil {
							user.BattleCode = u.Battle.BattleCode
							user.BattlePos = u.Battle.GetPosition(u.UserID)
							resp.BattleUsers = append(resp.BattleUsers, user)
						} else {
							resp.LobbyUsers = append(resp.LobbyUsers, user)
						}
					}
				}
			})

			for _, u := range sharedData.GetMcsUsers() {
				if !userAdded[u.UserID] {
					userAdded[u.UserID] = true
					resp.BattleUsers = append(resp.BattleUsers, &onlineUser{
						UserID:     u.UserID,
						Name:       u.Name,
						Team:       teamName(int(u.Team)),
						BattleCode: u.BattleCode,
						BattlePos:  uint8(u.Pos),
						Platform:   u.Platform,
						Disk:       u.GameDisk,
						Flycast:    "unknown",
					})
				}
			}

			for _, g := range sharedData.GetMcsGames() {
				resp.ActiveGames = append(resp.ActiveGames, &activeGame{
					BattleCode: g.BattleCode,
					Disk:       g.GameDisk,
					State:      gameStateName(g.State),
					LobbyID:    g.LobbyID,
					UpdatedAt:  g.UpdatedAt,
				})

				for _, u := range resp.BattleUsers {
					if u.BattleCode == g.BattleCode {
						u.LobbyID = g.LobbyID
					}
				}
			}

			return resp, nil
		})

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			logger.Error("JSON encode failed", zap.Error(err))
		}
	})

	http.HandleFunc("/lbs/replay", func(w http.ResponseWriter, r *http.Request) {
		// Public API: find replays

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var err error
		q := NewFindReplayQuery()
		q.BattleCode = r.FormValue("battle_code")
		q.Disk = r.FormValue("disk")
		q.UserID = r.FormValue("user_id")
		q.UserName = r.FormValue("user_name")
		q.PilotName = r.FormValue("pilot_name")
		if r.FormValue("lobby_id") != "" {
			if q.LobbyID, err = strconv.Atoi(r.FormValue("lobby_id")); err != nil {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			}
		}
		if r.FormValue("players") != "" {
			if q.Players, err = strconv.Atoi(r.FormValue("players")); err != nil {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			}
		}
		if r.FormValue("aggregate") != "" {
			if q.Aggregate, err = strconv.Atoi(r.FormValue("aggregate")); err != nil {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			}
		}
		if r.FormValue("reverse") != "" {
			if reverse, err := strconv.Atoi(r.FormValue("reverse")); err != nil {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			} else {
				q.Reverse = reverse == 1
			}
		}
		if r.FormValue("page") != "" {
			if q.Page, err = strconv.Atoi(r.FormValue("page")); err != nil {
				http.Error(w, "invalid query", http.StatusBadRequest)
				return
			}
		}

		replays, err := getDB().FindReplay(q)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if len(replays) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(replays)
		if err != nil {
			logger.Error("JSON encode failed", zap.Error(err))
		}
	})

	http.HandleFunc("/lbs/user", func(w http.ResponseWriter, r *http.Request) {
		// Public API: find user

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// find user by login_key
		loginKey := r.FormValue("login_key")
		if loginKey != "" {
			userList, err := getDB().GetUserList(loginKey)
			if err != nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			for _, u := range userList {
				// remove internal information
				u.SessionID = ""
				u.LoginKey = ""
				u.System = 0
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(userList)
			if err != nil {
				logger.Error("JSON encode failed", zap.Error(err))
			}
			return
		}

		// find user by machine_id
		machineID := r.FormValue("machine_id")
		if machineID != "" {
			userList, err := getDB().GetUserListByMachineID(machineID)
			if err != nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			for _, u := range userList {
				// remove internal information
				u.SessionID = ""
				u.LoginKey = ""
				u.System = 0
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(userList)
			if err != nil {
				logger.Error("JSON encode failed", zap.Error(err))
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	http.HandleFunc("/ops/replay_uploaded", func(w http.ResponseWriter, r *http.Request) {
		// Private API: Called when a replay is uploaded

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		battleCode := r.FormValue("battle_code")
		url := r.FormValue("url")
		if battleCode == "" || url == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if !strings.Contains(url, "https://storage.googleapis.com/gdxsv/") {
			logger.Warn("replay_uploaded invalid url", zap.String("url", url))
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		resp, err := http.Head(url)
		if err != nil {
			logger.Warn("replay_uploaded: Head failure", zap.Error(err))
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			logger.Warn("replay_uploaded: Head Invalid status code", zap.Int("status", resp.StatusCode))
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if err := getDB().SetReplayURL(battleCode, url); err != nil {
			logger.Warn("SetReplayURL failure", zap.Error(err))
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("OK"))
		if err != nil {
			logger.Error("Write response failed", zap.Error(err))
		}
	})

	http.HandleFunc("/ops/reload", func(w http.ResponseWriter, r *http.Request) {
		// Private API: Reloads settings from database

		lbs.Locked(func(lbs *Lbs) {
			lbs.reload = true
		})
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/plain")
		_, err := w.Write([]byte("OK"))
		if err != nil {
			logger.Error("Write response failed", zap.Error(err))
		}
	})
}
