package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveAPI_NoStore(t *testing.T) {
	savedMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	t.Cleanup(func() { http.DefaultServeMux = savedMux })

	lbs := NewLbs()
	done := make(chan struct{})
	go func() {
		defer close(done)
		lbs.eventLoop()
	}()
	t.Cleanup(func() {
		lbs.Quit()
		<-done
	})
	lbs.RegisterHTTPHandlers()

	for _, tt := range []struct {
		url    string
		status int
	}{
		{"/lbs/status", http.StatusOK},
		{"/lbs/spectators?battle_code=unknown", http.StatusOK},
		{"/lbs/spectators", http.StatusBadRequest},
	} {
		t.Run(tt.url, func(t *testing.T) {
			rec := httptest.NewRecorder()
			http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			assertEq(t, tt.status, rec.Code)
			assertEq(t, "no-store", rec.Result().Header.Get("Cache-Control"))
		})
	}
}
