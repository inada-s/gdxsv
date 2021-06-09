package main

import (
	"flag"
	"os"
	"reflect"
	"runtime"
	"testing"
)

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

func TestMain(m *testing.M) {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	prepareLogger()
	prepareTestDB()

	os.Exit(m.Run())
}
