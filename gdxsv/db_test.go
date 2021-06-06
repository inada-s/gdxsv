package main

import (
	"flag"
	"log"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/jmoiron/sqlx"
)

var testDB DB
var testLoginKey string
var testUserID string

func must(tb testing.TB, err error) {
	if err != nil {
		pc, file, line, _ := runtime.Caller(1)
		name := runtime.FuncForPC(pc).Name()
		tb.Fatalf("In %s:%d %s\nerr:%vn", file, line, name, err)
	}
}

func assertEq(tb testing.TB, expected, actual interface{}) {
	ok := reflect.DeepEqual(expected, actual)
	if !ok {
		pc, file, line, _ := runtime.Caller(1)
		name := runtime.FuncForPC(pc).Name()
		tb.Fatalf("In %s:%d %s\nexpected: %#v \nactual: %#v\n", file, line, name, expected, actual)
	}
}

func Test001RegisterAccount(t *testing.T) {
	a, err := testDB.RegisterAccount("1.2.3.4")
	must(t, err)
	assertEq(t, "1.2.3.4", a.CreatedIP)
	assertEq(t, 10, len(a.LoginKey))
	testLoginKey = a.LoginKey
}

func Test002GetAccount(t *testing.T) {
	a, err := testDB.GetAccountByLoginKey(testLoginKey)
	must(t, err)
	assertEq(t, testLoginKey, a.LoginKey)
}

func Test002GetInvalidAccount(t *testing.T) {
	a, err := testDB.GetAccountByLoginKey("hogehoge01")
	if err == nil {
		t.FailNow()
	}
	if a != nil {
		t.FailNow()
	}
}

func Test101RegisterUser(t *testing.T) {
	u, err := testDB.RegisterUser(testLoginKey)
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
	u, err := testDB.GetUser(testUserID)
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
	_, err := testDB.GetUser("HOGE01")
	if err == nil {
		t.FailNow()
	}
}

func Test104UpdateUser(t *testing.T) {
	u, err := testDB.GetUser(testUserID)
	must(t, err)
	u.Name = "テストユーザ"
	u.Team = "テストチーム"
	u.BattleCount = 100
	u.WinCount = 99
	u.LoseCount = 1
	u.DailyBattleCount = 10
	u.DailyWinCount = 9
	u.DailyLoseCount = 1
	err = testDB.UpdateUser(u)
	must(t, err)
	v, err := testDB.GetUser(testUserID)
	must(t, err)
	assertEq(t, u, v)
}

func Test105GetUserList(t *testing.T) {
	users, err := testDB.GetUserList(testLoginKey)
	must(t, err)
	assertEq(t, 1, len(users))
}

func Test106GetUserListNone(t *testing.T) {
	users, err := testDB.GetUserList("hogehoge01")
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
	err := testDB.AddBattleRecord(br)
	must(t, err)

	actual, err := testDB.GetBattleRecordUser(br.BattleCode, "123456")
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
	err := testDB.AddBattleRecord(br)
	must(t, err)
	err = testDB.UpdateBattleRecord(br)
	must(t, err)

	actual, err := testDB.GetBattleRecordUser(br.BattleCode, "23456")
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

	err := testDB.AddBattleRecord(br)
	must(t, err)
	err = testDB.UpdateBattleRecord(br)
	must(t, err)

	rec, err := testDB.CalculateUserTotalBattleCount("11111", 0)
	must(t, err)

	assertEq(t, br.Round, rec.Battle)
	assertEq(t, br.Win, rec.Win)
	assertEq(t, br.Lose, rec.Lose)
	assertEq(t, br.Kill, rec.Kill)
	assertEq(t, br.Death, rec.Death)

	rec, err = testDB.CalculateUserDailyBattleCount("11111")
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
	_, err := testDB.(SQLiteDB).Exec("DELETE FROM user")
	must(t, err)

	var users []*DBUser
	for i := 0; i < 3; i++ {
		ac, err := testDB.RegisterAccount("12.34.56.78")
		must(t, err)
		u, err := testDB.RegisterUser(ac.LoginKey)
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
		err := testDB.AddBattleRecord(br)
		must(t, err)
		err = testDB.UpdateBattleRecord(br)
		must(t, err)
	}

	for _, u := range users {
		rec, err := testDB.CalculateUserTotalBattleCount(u.UserID, 0)
		must(t, err)
		u.BattleCount = rec.Battle
		u.WinCount = rec.Win
		u.LoseCount = rec.Lose
		u.KillCount = rec.Kill
		u.DeathCount = rec.Death

		rec, err = testDB.CalculateUserTotalBattleCount(u.UserID, 1)
		must(t, err)
		u.RenpoBattleCount = rec.Battle
		u.RenpoWinCount = rec.Win
		u.RenpoLoseCount = rec.Lose
		u.RenpoKillCount = rec.Kill
		u.RenpoDeathCount = rec.Death

		rec, err = testDB.CalculateUserTotalBattleCount(u.UserID, 2)
		must(t, err)
		u.ZeonBattleCount = rec.Battle
		u.ZeonWinCount = rec.Win
		u.ZeonLoseCount = rec.Lose
		u.ZeonKillCount = rec.Kill
		u.ZeonDeathCount = rec.Death

		err = testDB.UpdateUser(u)
		t.Log(*u)
		must(t, err)
	}

	totalRanking, err := testDB.GetWinCountRanking(0)
	must(t, err)

	assertEq(t, 1000, totalRanking[0].WinCount)
	assertEq(t, 1000, totalRanking[0].BattleCount)
	assertEq(t, 1, totalRanking[0].Rank)
	assertEq(t, 1, totalRanking[1].Rank)
	assertEq(t, 3, totalRanking[2].Rank)

	aeugRanking, err := testDB.GetWinCountRanking(1)
	must(t, err)

	assertEq(t, 1000, aeugRanking[0].RenpoWinCount)
	assertEq(t, 1000, aeugRanking[0].BattleCount)
	assertEq(t, 0, aeugRanking[1].RenpoWinCount)

	assertEq(t, 1, aeugRanking[0].Rank)
	assertEq(t, 2, aeugRanking[1].Rank)
	assertEq(t, 2, aeugRanking[2].Rank)

	assertEq(t, 1000, aeugRanking[0].WinCount)

	titansRanking, err := testDB.GetWinCountRanking(2)
	must(t, err)

	assertEq(t, 1000, titansRanking[0].ZeonWinCount)
	assertEq(t, 1000, titansRanking[0].BattleCount)
	assertEq(t, 5, titansRanking[1].ZeonWinCount)

	assertEq(t, 1, titansRanking[0].Rank)
	assertEq(t, 2, titansRanking[1].Rank)
	assertEq(t, 3, titansRanking[2].Rank)
}

func TestMain(m *testing.M) {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	conn, err := sqlx.Open("sqlite3", "file::memory:")
	if err != nil {
		log.Fatalln("Cannot open test db. err:", err)
	}

	testDB = SQLiteDB{
		DB:          conn,
		SQLiteCache: NewSQLiteCache(),
	}
	err = testDB.Init()
	if err != nil {
		log.Fatalln("Failed to prepare DB. err:", err)
	}
	os.Exit(m.Run())
}
