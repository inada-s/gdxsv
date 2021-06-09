package main

import (
	"github.com/jmoiron/sqlx"
	"log"
	"testing"
	"time"
)

var testLoginKey string
var testUserID string

func prepareTestDB() {
	conn, err := sqlx.Open("sqlite3", "file::memory:")
	if err != nil {
		log.Fatalln("Cannot open test db. err:", err)
	}
	db := SQLiteDB{
		DB:          conn,
		SQLiteCache: NewSQLiteCache(),
	}
	err = db.Init()
	if err != nil {
		log.Fatalln("Failed to Init db", err)
	}
	defaultdb = db
}

func Test001RegisterAccount(t *testing.T) {
	a, err := getDB().RegisterAccount("1.2.3.4")
	must(t, err)
	assertEq(t, "1.2.3.4", a.CreatedIP)
	assertEq(t, 10, len(a.LoginKey))
	testLoginKey = a.LoginKey
}

func Test002GetAccount(t *testing.T) {
	a, err := getDB().GetAccountByLoginKey(testLoginKey)
	must(t, err)
	assertEq(t, testLoginKey, a.LoginKey)
}

func Test002GetInvalidAccount(t *testing.T) {
	a, err := getDB().GetAccountByLoginKey("hogehoge01")
	if err == nil {
		t.FailNow()
	}
	if a != nil {
		t.FailNow()
	}
}

func Test101RegisterUser(t *testing.T) {
	u, err := getDB().RegisterUser(testLoginKey)
	must(t, err)
	if u == nil {
		t.FailNow()
	}
	assertEq(t, testLoginKey, u.LoginKey)
	assertEq(t, 6, len(u.UserID))
	assertEq(t, 0, u.BattleCount)
	assertEq(t, 0, u.WinCount)
	assertEq(t, 0, u.LoseCount)
	assertEq(t, 0, u.DailyBattleCount)
	assertEq(t, 0, u.DailyWinCount)
	assertEq(t, 0, u.DailyLoseCount)
	testUserID = u.UserID
}

func Test102GetUser(t *testing.T) {
	u, err := getDB().GetUser(testUserID)
	must(t, err)
	assertEq(t, testUserID, u.UserID)
	assertEq(t, 0, u.BattleCount)
	assertEq(t, 0, u.WinCount)
	assertEq(t, 0, u.LoseCount)
	assertEq(t, 0, u.DailyBattleCount)
	assertEq(t, 0, u.DailyWinCount)
	assertEq(t, 0, u.DailyLoseCount)
}

func Test103GetInvalidUser(t *testing.T) {
	_, err := getDB().GetUser("HOGE01")
	if err == nil {
		t.FailNow()
	}
}

func Test104UpdateUser(t *testing.T) {
	u, err := getDB().GetUser(testUserID)
	must(t, err)
	u.Name = "テストユーザ"
	u.Team = "テストチーム"
	u.BattleCount = 100
	u.WinCount = 99
	u.LoseCount = 1
	u.DailyBattleCount = 10
	u.DailyWinCount = 9
	u.DailyLoseCount = 1
	err = getDB().UpdateUser(u)
	must(t, err)
	v, err := getDB().GetUser(testUserID)
	must(t, err)
	assertEq(t, u, v)
}

func Test105GetUserList(t *testing.T) {
	users, err := getDB().GetUserList(testLoginKey)
	must(t, err)
	assertEq(t, 1, len(users))
}

func Test106GetUserListNone(t *testing.T) {
	users, err := getDB().GetUserList("hogehoge01")
	must(t, err)
	assertEq(t, 0, len(users))
}

func Test200AddBattleRecord(t *testing.T) {
	br := &BattleRecord{
		BattleCode: "battlecode",
		UserID:     "123456",
		Players:    4,
		Pos:        1,
		Team:       2,
		System:     123,
	}
	err := getDB().AddBattleRecord(br)
	must(t, err)

	actual, err := getDB().GetBattleRecordUser(br.BattleCode, "123456")
	must(t, err)

	// These values are automatically set.
	br.Created = actual.Created
	br.Updated = actual.Updated
	assertEq(t, br, actual)
}

func Test201AddUpdateBattleRecord(t *testing.T) {
	br := &BattleRecord{
		BattleCode: "battlecode",
		UserID:     "23456",
		Players:    4,
		Aggregate:  1,
		Pos:        1,
		Round:      10,
		Win:        7,
		Lose:       3,
		Kill:       123,
		Death:      456,
		Frame:      9999,
		Result:     "result",
		Team:       2,
		System:     123,
	}
	err := getDB().AddBattleRecord(br)
	must(t, err)
	err = getDB().UpdateBattleRecord(br)
	must(t, err)

	actual, err := getDB().GetBattleRecordUser(br.BattleCode, "23456")
	must(t, err)

	// These values are automatically set.
	br.Created = actual.Created
	br.Updated = actual.Updated
	assertEq(t, br, actual)
}

func Test203CalculateUserBattleCount(t *testing.T) {
	br := &BattleRecord{
		BattleCode: "battlecode",
		UserID:     "11111",
		Players:    4,
		Aggregate:  1,
		Pos:        1,
		Round:      10,
		Win:        7,
		Lose:       3,
		Kill:       123,
		Death:      456,
		Frame:      9999,
		Result:     "result",
		Team:       2,
		System:     123,
	}

	err := getDB().AddBattleRecord(br)
	must(t, err)
	err = getDB().UpdateBattleRecord(br)
	must(t, err)

	rec, err := getDB().CalculateUserTotalBattleCount("11111", 0)
	must(t, err)

	assertEq(t, br.Round, rec.Battle)
	assertEq(t, br.Win, rec.Win)
	assertEq(t, br.Lose, rec.Lose)
	assertEq(t, br.Kill, rec.Kill)
	assertEq(t, br.Death, rec.Death)

	rec, err = getDB().CalculateUserDailyBattleCount("11111")
	must(t, err)

	assertEq(t, br.Round, rec.Battle)
	assertEq(t, br.Win, rec.Win)
	assertEq(t, br.Lose, rec.Lose)
	assertEq(t, br.Kill, rec.Kill)
	assertEq(t, br.Death, rec.Death)
	assertEq(t, br.Round, rec.Battle)
}

func Test300Ranking(t *testing.T) {
	// ugly...
	_, err := getDB().(SQLiteDB).Exec("DELETE FROM user")
	must(t, err)

	var users []*DBUser
	for i := 0; i < 3; i++ {
		ac, err := getDB().RegisterAccount("12.34.56.78")
		must(t, err)
		u, err := getDB().RegisterUser(ac.LoginKey)
		must(t, err)
		users = append(users, u)
	}

	for _, br := range []*BattleRecord{
		{
			BattleCode: "rankingtest0",
			UserID:     users[0].UserID,
			Players:    4,
			Aggregate:  1,
			Pos:        1,
			Round:      1000,
			Win:        1000,
			Lose:       0,
			Kill:       1000,
			Death:      456,
			Frame:      9999,
			Result:     "result",
			Team:       1,
			System:     123,
		},
		{
			BattleCode: "rankingtest1",
			UserID:     users[1].UserID,
			Players:    4,
			Aggregate:  1,
			Pos:        2,
			Round:      1000,
			Win:        1000,
			Lose:       0,
			Kill:       1000,
			Death:      456,
			Frame:      9999,
			Result:     "result",
			Team:       2,
			System:     123,
		},
		{
			BattleCode: "rankingtest2",
			UserID:     users[2].UserID,
			Players:    4,
			Aggregate:  1,
			Pos:        2,
			Round:      10,
			Win:        5,
			Lose:       5,
			Kill:       100,
			Death:      100,
			Frame:      9999,
			Result:     "result",
			Team:       2,
			System:     123,
		},
	} {
		err := getDB().AddBattleRecord(br)
		must(t, err)
		err = getDB().UpdateBattleRecord(br)
		must(t, err)
	}

	for _, u := range users {
		rec, err := getDB().CalculateUserTotalBattleCount(u.UserID, 0)
		must(t, err)
		u.BattleCount = rec.Battle
		u.WinCount = rec.Win
		u.LoseCount = rec.Lose
		u.KillCount = rec.Kill
		u.DeathCount = rec.Death

		rec, err = getDB().CalculateUserTotalBattleCount(u.UserID, 1)
		must(t, err)
		u.RenpoBattleCount = rec.Battle
		u.RenpoWinCount = rec.Win
		u.RenpoLoseCount = rec.Lose
		u.RenpoKillCount = rec.Kill
		u.RenpoDeathCount = rec.Death

		rec, err = getDB().CalculateUserTotalBattleCount(u.UserID, 2)
		must(t, err)
		u.ZeonBattleCount = rec.Battle
		u.ZeonWinCount = rec.Win
		u.ZeonLoseCount = rec.Lose
		u.ZeonKillCount = rec.Kill
		u.ZeonDeathCount = rec.Death

		err = getDB().UpdateUser(u)
		t.Log(*u)
		must(t, err)
	}

	totalRanking, err := getDB().GetWinCountRanking(0)
	must(t, err)

	assertEq(t, 1000, totalRanking[0].WinCount)
	assertEq(t, 1000, totalRanking[0].BattleCount)
	assertEq(t, 1, totalRanking[0].Rank)
	assertEq(t, 1, totalRanking[1].Rank)
	assertEq(t, 3, totalRanking[2].Rank)

	aeugRanking, err := getDB().GetWinCountRanking(1)
	must(t, err)

	assertEq(t, 1000, aeugRanking[0].RenpoWinCount)
	assertEq(t, 1000, aeugRanking[0].BattleCount)
	assertEq(t, 0, aeugRanking[1].RenpoWinCount)

	assertEq(t, 1, aeugRanking[0].Rank)
	assertEq(t, 2, aeugRanking[1].Rank)
	assertEq(t, 2, aeugRanking[2].Rank)

	assertEq(t, 1000, aeugRanking[0].WinCount)

	titansRanking, err := getDB().GetWinCountRanking(2)
	must(t, err)

	assertEq(t, 1000, titansRanking[0].ZeonWinCount)
	assertEq(t, 1000, titansRanking[0].BattleCount)
	assertEq(t, 5, titansRanking[1].ZeonWinCount)

	assertEq(t, 1, titansRanking[0].Rank)
	assertEq(t, 2, titansRanking[1].Rank)
	assertEq(t, 3, titansRanking[2].Rank)
}

func mustInsertDBAccount(a DBAccount) {
	db := getDB().(SQLiteDB)
	_, err := db.NamedExec(`
INSERT INTO account (
    login_key     ,
    session_id    ,
    last_user_id  ,
    created_ip    ,
    last_login_ip ,
    last_login_machine_id ,
    created       ,
    last_login    ,
    system
) VALUES (
    :login_key     ,
    :session_id    ,
    :last_user_id  ,
    :created_ip    ,
    :last_login_ip ,
    :last_login_machine_id ,
    :created       ,
    :last_login    ,
    :system
)`, a)
	if err != nil {
		panic(err)
	}
}

func mustInsertDBUser(u DBUser) {
	db := getDB().(SQLiteDB)
	_, err := db.NamedExec(`
INSERT INTO user (
    user_id            ,
    login_key          ,
    session_id         ,
    name               ,
    team               ,
    battle_count       ,
    win_count          ,
    lose_count         ,
    kill_count         ,
    death_count        ,
    renpo_battle_count ,
    renpo_win_count    ,
    renpo_lose_count   ,
    renpo_kill_count   ,
    renpo_death_count  ,
    zeon_battle_count  ,
    zeon_win_count     ,
    zeon_lose_count    ,
    zeon_kill_count    ,
    zeon_death_count   ,
    daily_battle_count ,
    daily_win_count    ,
    daily_lose_count   ,
    created            ,
    system
) VALUES (
    :user_id            ,
    :login_key          ,
    :session_id         ,
    :name               ,
    :team               ,
    :battle_count       ,
    :win_count          ,
    :lose_count         ,
    :kill_count         ,
    :death_count        ,
    :renpo_battle_count ,
    :renpo_win_count    ,
    :renpo_lose_count   ,
    :renpo_kill_count   ,
    :renpo_death_count  ,
    :zeon_battle_count  ,
    :zeon_win_count     ,
    :zeon_lose_count    ,
    :zeon_kill_count    ,
    :zeon_death_count   ,
    :daily_battle_count ,
    :daily_win_count    ,
    :daily_lose_count   ,
    :created            ,
    :system
)`, u)
	if err != nil {
		panic(err)
	}
}

func mustInsertBattleRecord(record BattleRecord) {
	db := getDB().(SQLiteDB)
	_, err := db.NamedExec(`
INSERT INTO battle_record (
    battle_code ,
    user_id     ,
    user_name   ,
    pilot_name  ,
    lobby_id    ,
    players     ,
    aggregate   ,
    pos         ,
    team        ,
    round       ,
    win         ,
    lose        ,
    kill        ,
    death       ,
    frame       ,
    result      ,
    created     ,
    updated     ,
    system      
) VALUES (
    :battle_code ,
    :user_id     ,
    :user_name   ,
    :pilot_name  ,
    :lobby_id    ,
    :players     ,
    :aggregate   ,
    :pos         ,
    :team        ,
    :round       ,
    :win         ,
    :lose        ,
    :kill        ,
    :death       ,
    :frame       ,
    :result      ,
    :created     ,
    :updated     ,
    :system      
)`, record)
	if err != nil {
		panic(err)
	}
}

func mustInsertMString(key, value string) {
	db := getDB().(SQLiteDB)
	_, err := db.Exec(`
INSERT INTO m_string (
  key, value
) VALUES (
  ?, ?
)`, key, value)
	if err != nil {
		panic(err)
	}
}

func mustInsertMBan(key string, until, created *time.Time) {
	db := getDB().(SQLiteDB)
	_, err := db.Exec(`
INSERT INTO m_ban (
  key, until, created
) VALUES (
  ?, ?, ?
)`, key, until, created)
	if err != nil {
		panic(err)
	}
}

func mustInsertMLobbySetting(setting MLobbySetting) {
	db := getDB().(SQLiteDB)
	_, err := db.NamedExec(`
INSERT INTO m_lobby_setting (
    platform           ,
    disk               ,
    no                 ,
    name               ,
    mcs_region         ,
    comment            ,
    rule_id            ,
    enable_force_start ,
    team_shuffle       ,
    ping_limit         ,
    ping_region        
) VALUES (
    :platform           ,
    :disk               ,
    :no                 ,
    :name               ,
    :mcs_region         ,
    :comment            ,
    :rule_id            ,
    :enable_force_start ,
    :team_shuffle       ,
    :ping_limit         ,
    :ping_region        
)`, setting)
	if err != nil {
		panic(err)
	}
}

func mustInsertMRule(rule MRule) {
	db := getDB().(SQLiteDB)
	_, err := db.NamedExec(`
INSERT INTO m_rule (
    id             ,
    difficulty     ,
    damage_level   ,
    timer          ,
    team_flag      ,
    stage_flag     ,
    ms_flag        ,
    renpo_vital    ,
    zeon_vital     ,
    ma_flag        ,
    reload_flag    ,
    boost_keep     ,
    redar_flag     ,
    lockon_flag    ,
    onematch       ,
    renpo_mask_ps2 ,
    zeon_mask_ps2  ,
    auto_rebattle  ,
    no_ranking     ,
    cpu_flag       ,
    select_look    ,
    renpo_mask_dc  ,
    zeon_mask_dc   ,
    stage_no       
) VALUES (
    :id             ,
    :difficulty     ,
    :damage_level   ,
    :timer          ,
    :team_flag      ,
    :stage_flag     ,
    :ms_flag        ,
    :renpo_vital    ,
    :zeon_vital     ,
    :ma_flag        ,
    :reload_flag    ,
    :boost_keep     ,
    :redar_flag     ,
    :lockon_flag    ,
    :onematch       ,
    :renpo_mask_ps2 ,
    :zeon_mask_ps2  ,
    :auto_rebattle  ,
    :no_ranking     ,
    :cpu_flag       ,
    :select_look    ,
    :renpo_mask_dc  ,
    :zeon_mask_dc   ,
    :stage_no       
)`, rule)
	if err != nil {
		panic(err)
	}
}
