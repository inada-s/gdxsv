package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func replayFilterTestServer(t *testing.T) (SQLiteDB, *http.ServeMux) {
	t.Helper()
	db := newFreshTestDB(t)
	must(t, db.Init())
	savedDB, savedMux := defaultdb, http.DefaultServeMux
	defaultdb = db
	mux := http.NewServeMux()
	http.DefaultServeMux = mux
	t.Cleanup(func() {
		defaultdb, http.DefaultServeMux = savedDB, savedMux
	})
	NewLbs().RegisterHTTPHandlers()
	return db, mux
}

func insertReplayFilterBattle(t *testing.T, db SQLiteDB, code, disk string, lobby int, created int64, users ...ReplayUser) {
	t.Helper()
	for i, user := range users {
		_, err := db.Exec(`INSERT INTO battle_record
  (battle_code, disk, lobby_id, players, aggregate, user_id, user_name,
   pilot_name, pos, team, created, replay_url, used_ms_mask)
VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)`,
			code, disk, lobby, len(users), user.UserID, user.UserName,
			user.PilotName, i+1, i%2+1, time.Unix(created, 0), code+".pb", 1<<i)
		must(t, err)
	}
}

func requestReplayFilters(t *testing.T, mux *http.ServeMux, query url.Values, status int) []*FoundReplay {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lbs/replay?"+query.Encode(), nil))
	if rec.Code != status {
		t.Fatalf("GET %s: status %d, want %d; body: %s", query.Encode(), rec.Code, status, rec.Body.String())
	}
	if status != http.StatusOK {
		return nil
	}
	var replays []*FoundReplay
	must(t, json.Unmarshal(rec.Body.Bytes(), &replays))
	return replays
}

func TestReplayPlayerFilters(t *testing.T) {
	db, mux := replayFilterTestServer(t)
	a := ReplayUser{UserID: "AAA111", UserName: "Alice", PilotName: "Ace"}
	b := ReplayUser{UserID: "BBB222", UserName: "Bob", PilotName: "Bravo"}
	c := ReplayUser{UserID: "CCC333", UserName: "Carol", PilotName: "パイロット + A&B, One"}
	d := ReplayUser{UserID: "DDD444", UserName: "Dave", PilotName: "Delta"}
	insertReplayFilterBattle(t, db, "ab", "dc2", 2, 1, a, b)
	insertReplayFilterBattle(t, db, "ac", "dc2", 2, 2, a, c)
	insertReplayFilterBattle(t, db, "bc", "dc1", 4, 3, b, c)
	insertReplayFilterBattle(t, db, "abcd", "dc2", 2, 4, a, b, c, d)
	_, err := db.Exec("UPDATE battle_record SET aggregate = 0 WHERE battle_code = ?", "ac")
	must(t, err)
	allUsers := map[string][]string{
		"ab":   {a.UserID, b.UserID},
		"ac":   {a.UserID, c.UserID},
		"bc":   {b.UserID, c.UserID},
		"abcd": {a.UserID, b.UserID, c.UserID, d.UserID},
	}

	for _, tt := range []struct {
		name  string
		query url.Values
		want  []string
	}{
		{"no_filters", nil, []string{"abcd", "bc", "ac", "ab"}},
		{"empty_filters", url.Values{"user_id": {""}, "user_name": {""}, "pilot_name": {""}}, []string{"abcd", "bc", "ac", "ab"}},
		{"legacy_single_id", url.Values{"user_id": {a.UserID}}, []string{"abcd", "ac", "ab"}},
		{"legacy_single_hn_pattern", url.Values{"user_name": {"%lic%"}}, []string{"abcd", "ac", "ab"}},
		{"legacy_single_pn_pattern", url.Values{"pilot_name": {"%Bra%"}}, []string{"abcd", "bc", "ab"}},
		{"two_ids", url.Values{"user_id": {a.UserID, b.UserID}}, []string{"abcd", "ab"}},
		{"three_ids", url.Values{"user_id": {a.UserID, b.UserID, c.UserID}}, []string{"abcd"}},
		{"four_ids", url.Values{"user_id": {a.UserID, b.UserID, c.UserID, d.UserID}}, []string{"abcd"}},
		{"two_handle_names", url.Values{"user_name": {"%Ali%", "%Bo%"}}, []string{"abcd", "ab"}},
		{"two_pilot_names", url.Values{"pilot_name": {"%Ace%", "%Bravo%"}}, []string{"abcd", "ab"}},
		{"id_and_other_players_pn", url.Values{"user_id": {a.UserID}, "pilot_name": {b.PilotName}}, []string{"abcd", "ab"}},
		{"all_fields_different_players", url.Values{"user_id": {a.UserID}, "user_name": {b.UserName}, "pilot_name": {c.PilotName}}, []string{"abcd"}},
		{"all_fields_same_player", url.Values{"user_id": {a.UserID}, "user_name": {a.UserName}, "pilot_name": {a.PilotName}}, []string{"abcd", "ac", "ab"}},
		{"overlapping_name_patterns", url.Values{"user_name": {"A%", "%ice"}}, []string{"abcd", "ac", "ab"}},
		{"url_encoded_pn", url.Values{"pilot_name": {c.PilotName}}, []string{"abcd", "bc", "ac"}},
		{"duplicates_and_empty", url.Values{"user_id": {a.UserID, "", a.UserID, a.UserID, a.UserID, a.UserID}}, []string{"abcd", "ac", "ab"}},
		{"duplicates_at_limit", url.Values{"user_id": {a.UserID, b.UserID, c.UserID, d.UserID, a.UserID}}, []string{"abcd"}},
		{"missing_player", url.Values{"user_id": {a.UserID, "ZZZ999"}}, nil},
		{"id_is_exact", url.Values{"user_id": {"AAA%"}}, nil},
		{"comma_is_not_a_separator", url.Values{"user_id": {a.UserID + "," + b.UserID}}, nil},
		{"sql_is_bound", url.Values{"user_name": {"' OR 1=1 --"}}, nil},
		{"disk", url.Values{"user_id": {b.UserID, c.UserID}, "disk": {"dc1"}}, []string{"bc"}},
		{"disk_mismatch", url.Values{"user_id": {a.UserID, b.UserID}, "disk": {"dc1"}}, nil},
		{"lobby", url.Values{"user_id": {b.UserID, c.UserID}, "lobby_id": {"4"}}, []string{"bc"}},
		{"lobby_mismatch", url.Values{"user_id": {a.UserID, b.UserID}, "lobby_id": {"4"}}, nil},
		{"player_count", url.Values{"user_id": {a.UserID, b.UserID}, "players": {"2"}}, []string{"ab"}},
		{"player_count_four", url.Values{"user_id": {a.UserID, b.UserID}, "players": {"4"}}, []string{"abcd"}},
		{"player_count_mismatch", url.Values{"user_id": {a.UserID, b.UserID}, "players": {"3"}}, nil},
		{"battle_code", url.Values{"user_id": {a.UserID, b.UserID}, "battle_code": {"ab"}}, []string{"ab"}},
		{"battle_code_mismatch", url.Values{"user_id": {a.UserID, b.UserID}, "battle_code": {"ac"}}, nil},
		{"aggregate", url.Values{"user_id": {a.UserID, b.UserID}, "aggregate": {"0"}}, nil},
		{"ranked", url.Values{"user_id": {a.UserID, b.UserID}, "aggregate": {"1"}}, []string{"abcd", "ab"}},
		{"unranked", url.Values{"user_id": {a.UserID, c.UserID}, "aggregate": {"0"}}, []string{"ac"}},
		{"used_ms", url.Values{"user_id": {a.UserID, b.UserID}, "used_ms": {"7"}}, nil},
		{"reverse", url.Values{"user_id": {a.UserID, b.UserID}, "reverse": {"1"}}, []string{"ab", "abcd"}},
		{"empty_page", url.Values{"user_id": {a.UserID, b.UserID}, "page": {"1"}}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := http.StatusOK
			if len(tt.want) == 0 {
				status = http.StatusNoContent
			}
			replays := requestReplayFilters(t, mux, tt.query, status)
			var got []string
			for _, replay := range replays {
				code := strings.TrimSuffix(replay.ReplayURL, ".pb")
				got = append(got, code)
				var ids []string
				for _, user := range replay.Users {
					ids = append(ids, user.UserID)
				}
				// Matching must not remove the other participants from a replay.
				assertEq(t, allUsers[code], ids)
			}
			assertEq(t, tt.want, got)
		})
	}

	for _, field := range []string{"user_id", "user_name", "pilot_name"} {
		t.Run(field+"_limits", func(t *testing.T) {
			requestReplayFilters(t, mux, url.Values{field: {"1", "2", "3", "4", "5"}}, http.StatusBadRequest)
			requestReplayFilters(t, mux, url.Values{field: {strings.Repeat("x", maxReplayFilterBytes+1)}}, http.StatusBadRequest)
			requestReplayFilters(t, mux, url.Values{field: {strings.Repeat("x", maxReplayFilterBytes)}}, http.StatusNoContent)
		})
	}
	t.Run("malformed_query", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lbs/replay?user_name=%", nil))
		assertEq(t, http.StatusBadRequest, rec.Code)
	})
}

func TestReplayPlayerFiltersUsedMS(t *testing.T) {
	db, mux := replayFilterTestServer(t)
	a := ReplayUser{UserID: "AAA111", UserName: "Alice", PilotName: "Ace"}
	b := ReplayUser{UserID: "BBB222", UserName: "Bob", PilotName: "Bravo"}
	c := ReplayUser{UserID: "CCC333", UserName: "Carol", PilotName: "Charlie"}
	d := ReplayUser{UserID: "DDD444", UserName: "Dave", PilotName: "Delta"}
	insertReplayFilterBattle(t, db, "abcd", "dc2", 2, 1, a, b, c, d)
	// Different masks expose accidentally matching an unrelated participant.
	for i, user := range []ReplayUser{a, b, c, d} {
		ms := []int{3, 7, 9, 10}[i]
		must(t, db.SaveUserUsedMs("abcd", user.UserID, 1<<ms, fmt.Sprint(ms)))
	}

	for _, tt := range []struct {
		name  string
		query url.Values
		ms    int
		match bool
	}{
		{"any_player", nil, 7, true},
		{"unused_ms", nil, 30, false},
		{"single_id_own_ms", url.Values{"user_id": {a.UserID}}, 3, true},
		{"single_id_other_ms", url.Values{"user_id": {a.UserID}}, 7, false},
		{"single_hn_own_ms", url.Values{"user_name": {"%lic%"}}, 3, true},
		{"single_hn_other_ms", url.Values{"user_name": {"%lic%"}}, 7, false},
		{"single_pn_own_ms", url.Values{"pilot_name": {"%rav%"}}, 7, true},
		{"single_pn_other_ms", url.Values{"pilot_name": {"%rav%"}}, 3, false},
		{"two_ids_first_ms", url.Values{"user_id": {a.UserID, b.UserID}}, 3, true},
		{"two_ids_second_ms", url.Values{"user_id": {a.UserID, b.UserID}}, 7, true},
		{"two_ids_unrelated_ms", url.Values{"user_id": {a.UserID, b.UserID}}, 9, false},
		{"reversed_ids_first_ms", url.Values{"user_id": {b.UserID, a.UserID}}, 3, true},
		{"reversed_ids_second_ms", url.Values{"user_id": {b.UserID, a.UserID}}, 7, true},
		{"reversed_ids_unrelated_ms", url.Values{"user_id": {b.UserID, a.UserID}}, 9, false},
		{"two_hns_second_ms", url.Values{"user_name": {"%lic%", "%ob%"}}, 7, true},
		{"two_hns_unrelated_ms", url.Values{"user_name": {"%lic%", "%ob%"}}, 9, false},
		{"two_pns_second_ms", url.Values{"pilot_name": {"%Ace%", "%Bravo%"}}, 7, true},
		{"two_pns_unrelated_ms", url.Values{"pilot_name": {"%Ace%", "%Bravo%"}}, 9, false},
		{"id_and_hn_ms", url.Values{"user_id": {a.UserID}, "user_name": {"%ob%"}}, 7, true},
		{"id_and_hn_unrelated_ms", url.Values{"user_id": {a.UserID}, "user_name": {"%ob%"}}, 9, false},
		{"id_and_pn_ms", url.Values{"user_id": {a.UserID}, "pilot_name": {b.PilotName}}, 7, true},
		{"id_and_pn_unrelated_ms", url.Values{"user_id": {a.UserID}, "pilot_name": {b.PilotName}}, 9, false},
		{"hn_and_pn_ms", url.Values{"user_name": {a.UserName}, "pilot_name": {b.PilotName}}, 7, true},
		{"three_fields_ms", url.Values{"user_id": {a.UserID}, "user_name": {b.UserName}, "pilot_name": {c.PilotName}}, 9, true},
		{"three_fields_unrelated_ms", url.Values{"user_id": {a.UserID}, "user_name": {b.UserName}, "pilot_name": {c.PilotName}}, 10, false},
		{"four_ids_last_ms", url.Values{"user_id": {a.UserID, b.UserID, c.UserID, d.UserID}}, 10, true},
		{"overlapping_patterns_ms", url.Values{"user_name": {"A%", "%ice"}}, 3, true},
		{"overlapping_patterns_unrelated_ms", url.Values{"user_name": {"A%", "%ice"}}, 7, false},
		{"missing_required_player", url.Values{"user_id": {a.UserID, "ZZZ999"}}, 3, false},
		{"all_battle_filters", url.Values{"user_id": {a.UserID, b.UserID}, "lobby_id": {"2"}, "players": {"4"}, "battle_code": {"abcd"}, "aggregate": {"1"}, "disk": {"dc2"}}, 7, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query := tt.query
			if query == nil {
				query = make(url.Values)
			}
			query.Set("used_ms", fmt.Sprint(tt.ms))
			status := http.StatusNoContent
			if tt.match {
				status = http.StatusOK
			}
			replays := requestReplayFilters(t, mux, query, status)
			if tt.match {
				assertEq(t, 1, len(replays))
				assertEq(t, "abcd.pb", replays[0].ReplayURL)
				// The result must still contain everyone, not just MS users.
				var ids []string
				for _, user := range replays[0].Users {
					ids = append(ids, user.UserID)
				}
				assertEq(t, []string{a.UserID, b.UserID, c.UserID, d.UserID}, ids)
			}
		})
	}
}

func TestReplayPlayerFilterPagination(t *testing.T) {
	db, mux := replayFilterTestServer(t)
	a := ReplayUser{UserID: "AAA111", UserName: "Alice", PilotName: "Ace"}
	b := ReplayUser{UserID: "BBB222", UserName: "Bob", PilotName: "Bravo"}
	c := ReplayUser{UserID: "CCC333", UserName: "Carol", PilotName: "Charlie"}
	for i := 0; i <= 100; i++ {
		insertReplayFilterBattle(t, db, fmt.Sprintf("match-%03d", i), "dc2", 2, int64(i*2), a, b)
		insertReplayFilterBattle(t, db, fmt.Sprintf("other-%03d", i), "dc2", 2, int64(i*2+1), a, c)
	}
	for _, tt := range []struct {
		page, reverse, count, first, last int
	}{
		{0, 0, 100, 100, 1},
		{1, 0, 1, 0, 0},
		{0, 1, 100, 0, 99},
		{1, 1, 1, 100, 100},
	} {
		t.Run(fmt.Sprintf("page_%d_reverse_%d", tt.page, tt.reverse), func(t *testing.T) {
			replays := requestReplayFilters(t, mux, url.Values{
				"user_id": {a.UserID, b.UserID},
				"page":    {fmt.Sprint(tt.page)},
				"reverse": {fmt.Sprint(tt.reverse)},
			}, http.StatusOK)
			assertEq(t, tt.count, len(replays))
			assertEq(t, fmt.Sprintf("match-%03d.pb", tt.first), replays[0].ReplayURL)
			assertEq(t, fmt.Sprintf("match-%03d.pb", tt.last), replays[len(replays)-1].ReplayURL)
			for _, replay := range replays {
				if !strings.HasPrefix(replay.ReplayURL, "match-") {
					t.Fatalf("unexpected replay %s", replay.ReplayURL)
				}
				assertEq(t, 2, len(replay.Users))
			}
		})
	}
}
