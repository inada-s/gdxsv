package main

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// newFreshTestDB opens an isolated in-memory SQLite database that is not
// shared with the package-level defaultdb, for tests that mutate the schema.
func newFreshTestDB(t *testing.T) SQLiteDB {
	t.Helper()
	// A unique name keeps each shared-cache in-memory database independent.
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	conn, err := sqlx.Open("sqlite3", dsn)
	must(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return SQLiteDB{DB: conn, SQLiteCache: NewSQLiteCache()}
}

func TestRankingKillCount(t *testing.T) {
	cleanTables(t, "user")
	db := getDB().(SQLiteDB)
	db.deleteRankingCache()

	// total kill order: A > B > C, renpo order: C > B > A, zeon order: B > A = C
	mustInsertDBUser(DBUser{LoginKey: "rankA", UserID: "RANK_A", Name: "A", Team: "team-a",
		KillCount: 300, RenpoKillCount: 10, ZeonKillCount: 50})
	mustInsertDBUser(DBUser{LoginKey: "rankB", UserID: "RANK_B", Name: "B", Team: "team-b",
		KillCount: 200, RenpoKillCount: 20, ZeonKillCount: 90})
	mustInsertDBUser(DBUser{LoginKey: "rankC", UserID: "RANK_C", Name: "C", Team: "team-c",
		KillCount: 100, RenpoKillCount: 30, ZeonKillCount: 50})

	userIDs := func(r []*RankingRecord) []string {
		ids := make([]string, 0, len(r))
		for _, x := range r {
			ids = append(ids, x.UserID)
		}
		return ids
	}
	ranks := func(r []*RankingRecord) []int {
		rs := make([]int, 0, len(r))
		for _, x := range r {
			rs = append(rs, x.Rank)
		}
		return rs
	}

	t.Run("Total", func(t *testing.T) {
		r, err := getDB().GetKillCountRanking(0)
		must(t, err)
		assertEq(t, 3, len(r))
		assertEq(t, []string{"RANK_A", "RANK_B", "RANK_C"}, userIDs(r))
		assertEq(t, []int{1, 2, 3}, ranks(r))
		assertEq(t, 300, r[0].KillCount)
		assertEq(t, "A", r[0].Name)
		assertEq(t, "team-a", r[0].Team)
	})

	t.Run("Renpo", func(t *testing.T) {
		r, err := getDB().GetKillCountRanking(1)
		must(t, err)
		assertEq(t, 3, len(r))
		assertEq(t, []string{"RANK_C", "RANK_B", "RANK_A"}, userIDs(r))
		assertEq(t, []int{1, 2, 3}, ranks(r))
		assertEq(t, 30, r[0].RenpoKillCount)
	})

	t.Run("Zeon", func(t *testing.T) {
		r, err := getDB().GetKillCountRanking(2)
		must(t, err)
		assertEq(t, 3, len(r))
		assertEq(t, "RANK_B", r[0].UserID)
		assertEq(t, 90, r[0].ZeonKillCount)
		// A and C tie at 50 and share rank 2 (RANK(), not ROW_NUMBER()).
		assertEq(t, []int{1, 2, 2}, ranks(r))
		assertEq(t, 50, r[1].ZeonKillCount)
		assertEq(t, 50, r[2].ZeonKillCount)
	})

	t.Run("CacheKeysDoNotCollide", func(t *testing.T) {
		db.mtx.Lock()
		defer db.mtx.Unlock()
		for _, key := range []string{"kill0", "kill1", "kill2"} {
			if _, ok := db.rankingCache[key]; !ok {
				t.Fatalf("expected cache key %q to be populated", key)
			}
		}
		assertEq(t, "RANK_A", db.rankingCache["kill0"][0].UserID)
		assertEq(t, "RANK_C", db.rankingCache["kill1"][0].UserID)
		assertEq(t, "RANK_B", db.rankingCache["kill2"][0].UserID)
		// Kill and win rankings must not share cache entries either.
		if _, ok := db.rankingCache["win0"]; ok {
			t.Fatal("win ranking cache should not have been populated by kill ranking")
		}
	})

	t.Run("CacheIsServedUntilInvalidated", func(t *testing.T) {
		mustInsertDBUser(DBUser{LoginKey: "rankD", UserID: "RANK_D", Name: "D", KillCount: 9999})

		r, err := getDB().GetKillCountRanking(0)
		must(t, err)
		assertEq(t, 3, len(r))
		assertEq(t, "RANK_A", r[0].UserID)

		db.deleteRankingCache()

		r, err = getDB().GetKillCountRanking(0)
		must(t, err)
		assertEq(t, 4, len(r))
		assertEq(t, "RANK_D", r[0].UserID)
		assertEq(t, 1, r[0].Rank)
		assertEq(t, 2, r[1].Rank)
	})

	t.Run("InvalidUTF8IsReplaced", func(t *testing.T) {
		mustInsertDBUser(DBUser{LoginKey: "rankE", UserID: "RANK_E",
			Name: "bad\xff\xfename", Team: "bad\xc0team", KillCount: 100000})
		db.deleteRankingCache()

		r, err := getDB().GetKillCountRanking(0)
		must(t, err)
		assertEq(t, "RANK_E", r[0].UserID)
		assertEq(t, "？", r[0].Name)
		assertEq(t, "？", r[0].Team)
		// Valid names are untouched.
		assertEq(t, "D", r[1].Name)
	})

	t.Run("EmptyTable", func(t *testing.T) {
		cleanTables(t, "user")
		db.deleteRankingCache()
		for _, team := range []byte{0, 1, 2} {
			r, err := getDB().GetKillCountRanking(team)
			must(t, err)
			assertEq(t, 0, len(r))
		}
	})
}

func TestDBResetDailyBattleCount(t *testing.T) {
	cleanTables(t, "user")

	mustInsertDBUser(DBUser{LoginKey: "daily1", UserID: "DAILY1",
		BattleCount: 100, WinCount: 60, LoseCount: 40, KillCount: 500, DeathCount: 300,
		RenpoBattleCount: 50, RenpoWinCount: 30, RenpoLoseCount: 20, RenpoKillCount: 250, RenpoDeathCount: 150,
		ZeonBattleCount: 50, ZeonWinCount: 30, ZeonLoseCount: 20, ZeonKillCount: 250, ZeonDeathCount: 150,
		DailyBattleCount: 10, DailyWinCount: 6, DailyLoseCount: 4})
	mustInsertDBUser(DBUser{LoginKey: "daily2", UserID: "DAILY2",
		BattleCount: 7, WinCount: 3, LoseCount: 4,
		DailyBattleCount: 7, DailyWinCount: 3, DailyLoseCount: 4})

	must(t, getDB().ResetDailyBattleCount())

	u1, err := getDB().GetUser("DAILY1")
	must(t, err)
	assertEq(t, 0, u1.DailyBattleCount)
	assertEq(t, 0, u1.DailyWinCount)
	assertEq(t, 0, u1.DailyLoseCount)
	assertEq(t, 100, u1.BattleCount)
	assertEq(t, 60, u1.WinCount)
	assertEq(t, 40, u1.LoseCount)
	assertEq(t, 500, u1.KillCount)
	assertEq(t, 300, u1.DeathCount)
	assertEq(t, 50, u1.RenpoBattleCount)
	assertEq(t, 30, u1.RenpoWinCount)
	assertEq(t, 20, u1.RenpoLoseCount)
	assertEq(t, 250, u1.RenpoKillCount)
	assertEq(t, 150, u1.RenpoDeathCount)
	assertEq(t, 50, u1.ZeonBattleCount)
	assertEq(t, 30, u1.ZeonWinCount)
	assertEq(t, 20, u1.ZeonLoseCount)
	assertEq(t, 250, u1.ZeonKillCount)
	assertEq(t, 150, u1.ZeonDeathCount)

	u2, err := getDB().GetUser("DAILY2")
	must(t, err)
	assertEq(t, 0, u2.DailyBattleCount)
	assertEq(t, 0, u2.DailyWinCount)
	assertEq(t, 0, u2.DailyLoseCount)
	assertEq(t, 7, u2.BattleCount)
	assertEq(t, 3, u2.WinCount)
	assertEq(t, 4, u2.LoseCount)

	// Resetting an already-reset table is a no-op, not an error.
	must(t, getDB().ResetDailyBattleCount())
}

func TestDBGetRule(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		rule, err := getDB().GetRule("dummy")
		must(t, err)
		assertEq(t, &MRule{ID: "dummy"}, rule)
	})

	t.Run("FoundAllColumns", func(t *testing.T) {
		want := MRule{
			ID: "rule-full", Difficulty: 1, DamageLevel: 2, Timer: 3, TeamFlag: 4, StageFlag: 5,
			MsFlag: 6, RenpoVital: 7, ZeonVital: 8, MaFlag: 9, ReloadFlag: 10, BoostKeep: 11,
			RedarFlag: 12, LockonFlag: 13, Onematch: 14, RenpoMaskPS2: 15, ZeonMaskPS2: 16,
			AutoRebattle: 17, NoRanking: 18, CPUFlag: 19, SelectLook: 20, RenpoMaskDC: 21,
			ZeonMaskDC: 22, StageNo: 23,
		}
		mustInsertMRule(want)
		rule, err := getDB().GetRule("rule-full")
		must(t, err)
		assertEq(t, &want, rule)
	})

	t.Run("NotFound", func(t *testing.T) {
		rule, err := getDB().GetRule("no-such-rule")
		if err == nil {
			t.Fatal("expected error for missing rule")
		}
		assertEq(t, sql.ErrNoRows, err)
		if rule != nil {
			t.Fatalf("expected nil rule, got %#v", rule)
		}
	})
}

func TestDBGetPatch(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		patch, err := getDB().GetPatch("emu-x86/64", "dc2", "dummy-patch")
		must(t, err)
		assertEq(t, &MPatch{
			Platform:  "emu-x86/64",
			Disk:      "dc2",
			Name:      "dummy-patch",
			WriteOnce: true,
			Codes:     "1, 8, 0c391d97, 1, 0",
		}, patch)
	})

	t.Run("NotFound", func(t *testing.T) {
		for _, c := range []struct{ platform, disk, name string }{
			{"emu-x86/64", "dc2", "no-such-patch"},
			{"emu-x86/64", "dc1", "dummy-patch"},
			{"other", "dc2", "dummy-patch"},
			{"", "", ""},
		} {
			patch, err := getDB().GetPatch(c.platform, c.disk, c.name)
			if err == nil {
				t.Fatalf("expected error for %+v", c)
			}
			assertEq(t, sql.ErrNoRows, err)
			if patch != nil {
				t.Fatalf("expected nil patch for %+v, got %#v", c, patch)
			}
		}
	})
}

func TestDBGetUserListByMachineID(t *testing.T) {
	cleanTables(t, "user", "account")

	const machineA = "machine-A"
	const machineB = "machine-B"

	// Account 1 -> machineA, two users.
	ac1, err := getDB().RegisterAccount("10.0.0.1")
	must(t, err)
	must(t, getDB().LoginAccount(ac1, "sess1", "10.0.0.1", machineA))
	u1a, err := getDB().RegisterUser(ac1.LoginKey)
	must(t, err)
	u1b, err := getDB().RegisterUser(ac1.LoginKey)
	must(t, err)

	// Account 2 -> machineA, one user.
	ac2, err := getDB().RegisterAccount("10.0.0.2")
	must(t, err)
	must(t, getDB().LoginAccount(ac2, "sess2", "10.0.0.2", machineA))
	u2, err := getDB().RegisterUser(ac2.LoginKey)
	must(t, err)

	// Account 3 -> machineB, one user.
	ac3, err := getDB().RegisterAccount("10.0.0.3")
	must(t, err)
	must(t, getDB().LoginAccount(ac3, "sess3", "10.0.0.3", machineB))
	u3, err := getDB().RegisterUser(ac3.LoginKey)
	must(t, err)

	// Account 4 -> never logged in (empty machine id), one user.
	ac4, err := getDB().RegisterAccount("10.0.0.4")
	must(t, err)
	_, err = getDB().RegisterUser(ac4.LoginKey)
	must(t, err)

	idSet := func(users []*DBUser) map[string]bool {
		m := map[string]bool{}
		for _, u := range users {
			m[u.UserID] = true
		}
		return m
	}

	t.Run("Zero", func(t *testing.T) {
		users, err := getDB().GetUserListByMachineID("machine-unknown")
		must(t, err)
		assertEq(t, 0, len(users))
	})

	t.Run("EmptyMachineIDMatchesNeverLoggedIn", func(t *testing.T) {
		users, err := getDB().GetUserListByMachineID("")
		must(t, err)
		// account 4 has the default '' machine id, so it is the only match.
		assertEq(t, 1, len(users))
		assertEq(t, ac4.LoginKey, users[0].LoginKey)
	})

	t.Run("One", func(t *testing.T) {
		users, err := getDB().GetUserListByMachineID(machineB)
		must(t, err)
		assertEq(t, 1, len(users))
		assertEq(t, u3.UserID, users[0].UserID)
		assertEq(t, ac3.LoginKey, users[0].LoginKey)
	})

	t.Run("ManyAcrossAccounts", func(t *testing.T) {
		users, err := getDB().GetUserListByMachineID(machineA)
		must(t, err)
		assertEq(t, 3, len(users))
		assertEq(t, map[string]bool{u1a.UserID: true, u1b.UserID: true, u2.UserID: true}, idSet(users))
	})

	t.Run("FollowsLastLogin", func(t *testing.T) {
		// Account 3 moves to machineA; its user now shows up there and not on machineB.
		must(t, getDB().LoginAccount(ac3, "sess3b", "10.0.0.3", machineA))

		users, err := getDB().GetUserListByMachineID(machineA)
		must(t, err)
		assertEq(t, 4, len(users))

		users, err = getDB().GetUserListByMachineID(machineB)
		must(t, err)
		assertEq(t, 0, len(users))
	})
}

func TestDBIncrementReplayPlayCount(t *testing.T) {
	const code = "TestDBIncrementReplayPlayCount"
	must(t, getDB().AddBattleRecord(&BattleRecord{BattleCode: code, UserID: "INC1", Players: 2}))
	must(t, getDB().AddBattleRecord(&BattleRecord{BattleCode: code, UserID: "INC2", Players: 2}))
	must(t, getDB().AddBattleRecord(&BattleRecord{BattleCode: code + "-other", UserID: "INC1", Players: 1}))

	t.Run("Monotonic", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			must(t, getDB().IncrementReplayPlayCount(code))
			br, err := getDB().GetBattleRecordUser(code, "INC1")
			must(t, err)
			assertEq(t, i, br.PlayCount)
		}
		// Every row of the battle is incremented together.
		br, err := getDB().GetBattleRecordUser(code, "INC2")
		must(t, err)
		assertEq(t, 3, br.PlayCount)
		// Unrelated battles are untouched.
		br, err = getDB().GetBattleRecordUser(code+"-other", "INC1")
		must(t, err)
		assertEq(t, 0, br.PlayCount)
	})

	t.Run("Concurrent", func(t *testing.T) {
		const goroutines = 8
		const perGoroutine = 5
		var wg sync.WaitGroup
		errs := make(chan error, goroutines*perGoroutine)
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perGoroutine; i++ {
					if err := getDB().IncrementReplayPlayCount(code); err != nil {
						errs <- err
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			must(t, err)
		}

		br, err := getDB().GetBattleRecordUser(code, "INC1")
		must(t, err)
		assertEq(t, 3+goroutines*perGoroutine, br.PlayCount)
	})

	t.Run("UnknownCodeIsNoop", func(t *testing.T) {
		must(t, getDB().IncrementReplayPlayCount("no-such-battle"))
	})
}

// schemaSnapshot returns every table (and its DDL) in sqlite_master, so two
// migrations can be compared for a stable schema.
func schemaSnapshot(t *testing.T, db SQLiteDB) map[string]string {
	t.Helper()
	rows, err := db.Queryx(`SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	must(t, err)
	defer rows.Close()
	snap := map[string]string{}
	for rows.Next() {
		var name, ddl string
		must(t, rows.Scan(&name, &ddl))
		snap[name] = ddl
	}
	must(t, rows.Err())
	return snap
}

func TestMigrateIdempotent(t *testing.T) {
	db := newFreshTestDB(t)
	must(t, db.Init())

	expectedTables := []string{"account", "user", "battle_record", "m_string", "m_ban", "m_lobby_setting", "m_rule", "m_patch"}
	initial := schemaSnapshot(t, db)
	assertEq(t, len(expectedTables), len(initial))
	for _, name := range expectedTables {
		if _, ok := initial[name]; !ok {
			t.Fatalf("table %q missing after Init", name)
		}
	}

	// Seed one row in every migrated table so we can check data survives.
	ac, err := db.RegisterAccount("1.1.1.1")
	must(t, err)
	must(t, db.LoginAccount(ac, "sess", "1.1.1.1", "machine"))
	u, err := db.RegisterUser(ac.LoginKey)
	must(t, err)
	u.Name = "migrated"
	u.KillCount = 42
	must(t, db.UpdateUser(u))
	must(t, db.AddBattleRecord(&BattleRecord{BattleCode: "mig", UserID: u.UserID, Players: 4, Team: 1}))
	_, err = db.Exec(`INSERT INTO m_string (key, value) VALUES ('k', 'v')`)
	must(t, err)
	_, err = db.Exec(`INSERT INTO m_ban (key, until, created) VALUES ('b', ?, ?)`, time.Now(), time.Now())
	must(t, err)
	_, err = db.Exec(`INSERT INTO m_lobby_setting (platform, disk, no, name, enable_force_start, team_shuffle, ping_limit) VALUES ('p', 'd', 1, 'n', 0, 0, 0)`)
	must(t, err)
	_, err = db.Exec(`INSERT INTO m_rule VALUES ('r', 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0)`)
	must(t, err)

	for i := 1; i <= 2; i++ {
		t.Run(fmt.Sprintf("Run%d", i), func(t *testing.T) {
			must(t, db.Migrate())

			after := schemaSnapshot(t, db)
			assertEq(t, initial, after)

			// No *_tmp leftovers of any kind.
			var tmpCount int
			must(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name LIKE '%_tmp'`).Scan(&tmpCount))
			assertEq(t, 0, tmpCount)

			gotAc, err := db.GetAccountByLoginKey(ac.LoginKey)
			must(t, err)
			assertEq(t, "machine", gotAc.LastLoginMachineID)
			assertEq(t, "sess", gotAc.SessionID)

			gotU, err := db.GetUser(u.UserID)
			must(t, err)
			assertEq(t, "migrated", gotU.Name)
			assertEq(t, 42, gotU.KillCount)

			gotBr, err := db.GetBattleRecordUser("mig", u.UserID)
			must(t, err)
			assertEq(t, 1, gotBr.Team)
			assertEq(t, 4, gotBr.Players)

			v, err := db.GetString("k")
			must(t, err)
			assertEq(t, "v", v)

			ls, err := db.GetLobbySetting("p", "d", 1)
			must(t, err)
			assertEq(t, "n", ls.Name)

			r, err := db.GetRule("r")
			must(t, err)
			assertEq(t, "r", r.ID)

			var banCount int
			must(t, db.QueryRow(`SELECT COUNT(*) FROM m_ban`).Scan(&banCount))
			assertEq(t, 1, banCount)
		})
	}

	// Migrate is transactional: the connection is usable afterwards.
	_, err = db.RegisterAccount("2.2.2.2")
	must(t, err)
}

// legacyMigrateDB creates the seven migrated tables with a minimal old-era
// layout, overriding battle_record's DDL with the given column list.
func legacyMigrateDB(t *testing.T, accountMachineCol, battleRecordCols string) SQLiteDB {
	t.Helper()
	db := newFreshTestDB(t)
	_, err := db.Exec(`
CREATE TABLE account (
    login_key text, session_id text default '', last_user_id text default '',
    created_ip text default '', last_login_ip text default '', ` + accountMachineCol + ` text default '',
    created timestamp, last_login timestamp, system integer default 0, PRIMARY KEY (login_key));
CREATE TABLE user (user_id text, login_key text, PRIMARY KEY (user_id, login_key));
CREATE TABLE battle_record (battle_code text, user_id text, ` + battleRecordCols + `, PRIMARY KEY (battle_code, user_id));
CREATE TABLE m_string (key text, value text, PRIMARY KEY (key));
CREATE TABLE m_ban (key text, until timestamp, created timestamp, PRIMARY KEY (key));
CREATE TABLE m_lobby_setting (platform text, disk text, no integer, PRIMARY KEY (platform, disk, no));
CREATE TABLE m_rule (id text, PRIMARY KEY (id));
INSERT INTO user (user_id, login_key) VALUES ('OLDUSR', 'oldkey');
`)
	must(t, err)
	return db
}

func TestMigrateRenamedColumns(t *testing.T) {
	t.Run("SideAndCpuid", func(t *testing.T) {
		// 2021-02: battle_record.side -> team, 2021-06: account.last_login_cpuid -> last_login_machine_id.
		db := legacyMigrateDB(t, "last_login_cpuid", "side integer default 0")
		_, err := db.Exec(`
INSERT INTO account (login_key, last_login_cpuid) VALUES ('oldkey', 'oldcpu');
INSERT INTO battle_record (battle_code, user_id, side) VALUES ('oldbattle', 'OLDUSR', 2);`)
		must(t, err)

		must(t, db.Migrate())

		// Legacy rows carry NULLs in columns the old schema never had, so
		// read the migrated values column-by-column instead of StructScan.
		var machineID, loginKey, name string
		var team int
		must(t, db.QueryRow(`SELECT last_login_machine_id FROM account WHERE login_key = 'oldkey'`).Scan(&machineID))
		assertEq(t, "oldcpu", machineID)
		must(t, db.QueryRow(`SELECT login_key, name FROM user WHERE user_id = 'OLDUSR'`).Scan(&loginKey, &name))
		assertEq(t, "oldkey", loginKey)
		assertEq(t, "default", name)
		must(t, db.QueryRow(`SELECT team FROM battle_record WHERE battle_code = 'oldbattle'`).Scan(&team))
		assertEq(t, 2, team)

		// Running again on the now-current schema is still fine.
		must(t, db.Migrate())
		must(t, db.QueryRow(`SELECT team FROM battle_record WHERE battle_code = 'oldbattle'`).Scan(&team))
		assertEq(t, 2, team)
	})

	t.Run("ResultRemoved", func(t *testing.T) {
		// 2026-02: battle_record.result dropped.
		db := legacyMigrateDB(t, "last_login_machine_id", "team integer default 0, result text default ''")
		_, err := db.Exec(`
INSERT INTO account (login_key, last_login_machine_id) VALUES ('oldkey', 'machine');
INSERT INTO battle_record (battle_code, user_id, team, result) VALUES ('oldbattle', 'OLDUSR', 1, 'win');`)
		must(t, err)

		must(t, db.Migrate())

		var team int
		must(t, db.QueryRow(`SELECT team FROM battle_record WHERE battle_code = 'oldbattle'`).Scan(&team))
		assertEq(t, 1, team)

		var hasResult int
		must(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('battle_record') WHERE name = 'result'`).Scan(&hasResult))
		assertEq(t, 0, hasResult)

		must(t, db.Migrate())
	})
}

func TestDBLoginUser(t *testing.T) {
	cleanTables(t, "user", "account")

	ac, err := getDB().RegisterAccount("9.9.9.9")
	must(t, err)
	u, err := getDB().RegisterUser(ac.LoginKey)
	must(t, err)

	t.Run("Success", func(t *testing.T) {
		u.SessionID = "user-session-1"
		must(t, getDB().LoginUser(u))

		gotU, err := getDB().GetUser(u.UserID)
		must(t, err)
		assertEq(t, "user-session-1", gotU.SessionID)

		gotAc, err := getDB().GetAccountByLoginKey(ac.LoginKey)
		must(t, err)
		assertEq(t, u.UserID, gotAc.LastUserID)
	})

	t.Run("SwitchUserOnSameAccount", func(t *testing.T) {
		u2, err := getDB().RegisterUser(ac.LoginKey)
		must(t, err)
		u2.SessionID = "user-session-2"
		must(t, getDB().LoginUser(u2))

		gotAc, err := getDB().GetAccountByLoginKey(ac.LoginKey)
		must(t, err)
		assertEq(t, u2.UserID, gotAc.LastUserID)

		// The first user's session is left as is.
		gotU, err := getDB().GetUser(u.UserID)
		must(t, err)
		assertEq(t, "user-session-1", gotU.SessionID)
	})

	t.Run("MissingAccount", func(t *testing.T) {
		orphan := &DBUser{LoginKey: "nosuchkey0", UserID: "ORPHAN", SessionID: "orphan-session"}
		err := getDB().LoginUser(orphan)
		if err == nil {
			t.Fatal("expected error for missing account")
		}
		assertEq(t, sql.ErrNoRows, err)
	})

	t.Run("MissingUserRowIsNotAnError", func(t *testing.T) {
		// The account exists but the user row does not; UPDATE affects 0 rows.
		ghost := &DBUser{LoginKey: ac.LoginKey, UserID: "GHOST", SessionID: "ghost-session"}
		must(t, getDB().LoginUser(ghost))

		gotAc, err := getDB().GetAccountByLoginKey(ac.LoginKey)
		must(t, err)
		assertEq(t, "GHOST", gotAc.LastUserID)
	})
}

func TestDBSetReplayURLBulk(t *testing.T) {
	const code1 = "TestDBSetReplayURLBulk1"
	const code2 = "TestDBSetReplayURLBulk2"
	must(t, getDB().AddBattleRecord(&BattleRecord{Disk: "dc2", BattleCode: code1, UserID: "BULK"}))
	must(t, getDB().AddBattleRecord(&BattleRecord{Disk: "dc2", BattleCode: code2, UserID: "BULK"}))

	t.Run("LengthMismatch", func(t *testing.T) {
		err := getDB().SetReplayURLBulk([]string{code1, code2}, []string{"http://example.com/1"}, nil)
		if err == nil {
			t.Fatal("expected error for mismatched lengths")
		}
		err = getDB().SetReplayURLBulk([]string{code1}, []string{"http://example.com/1", "http://example.com/2"}, nil)
		if err == nil {
			t.Fatal("expected error for mismatched lengths")
		}

		// Nothing was written.
		br, err := getDB().GetBattleRecordUser(code1, "BULK")
		must(t, err)
		assertEq(t, "", br.ReplayURL)
	})

	t.Run("Empty", func(t *testing.T) {
		must(t, getDB().SetReplayURLBulk(nil, nil, nil))
		must(t, getDB().SetReplayURLBulk([]string{}, []string{}, []string{}))
	})

	t.Run("DisksShorterThanCodes", func(t *testing.T) {
		// Only the first entry carries a disk; the second keeps its existing disk.
		must(t, getDB().SetReplayURLBulk(
			[]string{code1, code2},
			[]string{"http://example.com/a", "http://example.com/b"},
			[]string{"dc1"}))

		br, err := getDB().GetBattleRecordUser(code1, "BULK")
		must(t, err)
		assertEq(t, "http://example.com/a", br.ReplayURL)
		assertEq(t, "dc1", br.Disk)

		br, err = getDB().GetBattleRecordUser(code2, "BULK")
		must(t, err)
		assertEq(t, "http://example.com/b", br.ReplayURL)
		assertEq(t, "dc2", br.Disk)
	})

	t.Run("EmptyDiskKeepsExisting", func(t *testing.T) {
		must(t, getDB().SetReplayURLBulk(
			[]string{code1},
			[]string{"http://example.com/c"},
			[]string{""}))

		br, err := getDB().GetBattleRecordUser(code1, "BULK")
		must(t, err)
		assertEq(t, "http://example.com/c", br.ReplayURL)
		assertEq(t, "dc1", br.Disk)
	})

	t.Run("UnknownCodeIsNoop", func(t *testing.T) {
		must(t, getDB().SetReplayURLBulk([]string{"no-such-battle"}, []string{"http://example.com/x"}, []string{"dc2"}))
	})
}
