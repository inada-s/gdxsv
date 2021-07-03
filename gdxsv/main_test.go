package main

import (
	"flag"
	"os"
	"reflect"
	"runtime/debug"
	"testing"
	"time"
)

func must(tb testing.TB, err error) {
	if err != nil {
		tb.Fatal("err:", err, string(debug.Stack()))
	}
}

func assertEq(tb testing.TB, expected, actual interface{}) {
	ok := reflect.DeepEqual(expected, actual)
	if !ok {
		tb.Fatalf("assertEq failed.\n expected: %#v\n actual:  %#v\n%s", expected, actual, string(debug.Stack()))
	}
}

func TestMain(m *testing.M) {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	*loglevel = 2

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
	mustInsertMPatch(MPatch{
		Platform:  "emu-x86/64",
		Disk:      "dc2",
		Name:      "dummy-patch",
		WriteOnce: true,
		Codes:     "1, 8, 0c391d97, 1, 0",
	})

	os.Exit(m.Run())
}
