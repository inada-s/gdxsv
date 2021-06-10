package main

import (
	"flag"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func must(tb testing.TB, err error) {
	if err != nil {
		pc, file, line, _ := runtime.Caller(1)
		name := runtime.FuncForPC(pc).Name()
		tb.Errorf("In %s:%d %s\nerr:%vn", file, line, name, err)
	}
}

func assertEq(tb testing.TB, expected, actual interface{}) {
	ok := reflect.DeepEqual(expected, actual)
	if !ok {
		pc, file, line, _ := runtime.Caller(1)
		name := runtime.FuncForPC(pc).Name()
		tb.Errorf("In %s:%d %s\nexpected: %#v \nactual: %#v\n", file, line, name, expected, actual)
	}
}

func TestMain(m *testing.M) {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	prepareLogger()
	prepareTestDB()

	mustInsertDBAccount(DBAccount{LoginKey: "0000000000"})
	mustInsertDBUser(DBUser{LoginKey: "0000000000", UserID: "DUMMY0", Name: "DUMMY0"})
	mustInsertBattleRecord(BattleRecord{
		BattleCode: "dummy",
		UserID:     "DUMMY0",
		UserName:   "DUMMY0",
		PilotName:  "DUMMY0",
		LobbyID:    1,
		Players:    1,
		Aggregate:  0,
	})
	mustInsertMBan("DUMMY0",
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	mustInsertMLobbySetting(MLobbySetting{
		Platform: "dummy",
		Disk:     "dummy",
		No:       1,
		Name:     "dummy",
		Comment:  "dummy",
		RuleID:   "dummy",
	})
	mustInsertMRule(MRule{ID: "dummy"})
	mustInsertMString("dummy", "dummy string")

	os.Exit(m.Run())
}
