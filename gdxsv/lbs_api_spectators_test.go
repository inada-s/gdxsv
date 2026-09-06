package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"gdxsv/gdxsv/proto"
)

func TestSpectatorsHandler_ConcurrentBattles(t *testing.T) {
	saved := spectatorRegistry
	r := newTestSpectatorRegistry()
	spectatorRegistry = r
	t.Cleanup(func() { spectatorRegistry = saved })

	for i, code := range []string{"battle-a", "battle-b"} {
		r.Open(&proto.P2PMatching{BattleCode: code}, "dc2", nil)
		s, _ := r.GetAny(code)
		s.PushInputs(0, []uint64{1})
		for j := 0; j <= i; j++ {
			subscribeTestSpectator(s, testAddr(30000+i*10+j), 0)
		}
	}

	type response struct {
		BattleCode string `json:"battle_code"`
		Spectators int    `json:"spectators"`
		Live       bool   `json:"live"`
	}
	want := []response{{"battle-a", 1, true}, {"battle-b", 2, true}, {"unknown", 0, false}}
	const requests = 96
	recorders := make([]*httptest.ResponseRecorder, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range recorders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodGet, "/lbs/spectators?battle_code="+want[i%len(want)].BattleCode, nil)
			rec := httptest.NewRecorder()
			spectatorsHandler(rec, req)
			recorders[i] = rec
		}(i)
	}
	close(start)
	wg.Wait()

	for i, rec := range recorders {
		assertEq(t, http.StatusOK, rec.Code)
		assertEq(t, "application/json", rec.Header().Get("Content-Type"))
		var got response
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		assertEq(t, want[i%len(want)], got)
	}
}

func TestSpectatorsHandler_DoesNotShareStatusRequest(t *testing.T) {
	// Even a battle_code equal to another endpoint's key must not share it.
	release := make(chan struct{})
	flight := httpRequestGroup.DoChan("/lbs/status", func() (interface{}, error) {
		<-release
		return "lobby status, not a spectator count", nil
	})
	var wg sync.WaitGroup
	defer func() {
		close(release)
		<-flight
		wg.Wait()
	}()

	done := make(chan *httptest.ResponseRecorder, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/lbs/spectators?battle_code="+url.QueryEscape("/lbs/status"), nil)
		rec := httptest.NewRecorder()
		spectatorsHandler(rec, req)
		done <- rec
	}()

	select {
	case rec := <-done:
		assertEq(t, http.StatusOK, rec.Code)
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		assertEq(t, "/lbs/status", body["battle_code"])
	case <-time.After(5 * time.Second):
		t.Fatal("spectator request waited for the unrelated /lbs/status flight")
	}
}
