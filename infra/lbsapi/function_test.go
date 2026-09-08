package function

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestLbsAPIHandlerCacheControl(t *testing.T) {
	savedClient := http.DefaultClient
	savedCache := cache
	t.Cleanup(func() {
		http.DefaultClient = savedClient
		cache = savedCache
	})

	for _, tt := range []struct {
		url          string
		cacheControl string
	}{
		{"/status", "no-store"},
		{"/spectators?battle_code=battle-a", "no-store"},
		{"/spectators?battle_code=battle-b", "no-store"},
		{"/replay", ""},
		{"/user", ""},
	} {
		t.Run(tt.url, func(t *testing.T) {
			cache = make(map[string]*ResponseCache)
			calls := 0
			http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			})}

			// Both an origin response and a cache hit must carry the policy.
			for _, fromCache := range []string{"", "yes"} {
				rec := httptest.NewRecorder()
				lbsApiHandler(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
				header := rec.Result().Header
				if rec.Code != http.StatusOK || rec.Body.String() != `{"ok":true}` {
					t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
				}
				if got := header.Get("Cache-Control"); got != tt.cacheControl {
					t.Errorf("Cache-Control = %q, want %q", got, tt.cacheControl)
				}
				if got := header.Get("FromCache"); got != fromCache {
					t.Errorf("FromCache = %q, want %q", got, fromCache)
				}
			}
			if calls != 1 {
				t.Fatalf("origin requests = %d, want 1", calls)
			}

			// The bounded server-side cache still expires normally.
			cache[tt.url].Time = time.Now().Add(-4 * time.Second)
			rec := httptest.NewRecorder()
			lbsApiHandler(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if calls != 2 || rec.Result().Header.Get("Cache-Control") != tt.cacheControl {
				t.Fatal("expired response was not refreshed with the expected cache policy")
			}
		})
	}
}

func TestLbsAPIHandlerErrorsNoStore(t *testing.T) {
	savedClient := http.DefaultClient
	savedCache := cache
	t.Cleanup(func() {
		http.DefaultClient = savedClient
		cache = savedCache
	})

	for _, target := range []string{"/status", "/spectators?battle_code=battle-a"} {
		for _, networkError := range []bool{false, true} {
			cache = make(map[string]*ResponseCache)
			http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if networkError {
					return nil, errors.New("test connection failure")
				}
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("unavailable")),
					Header:     make(http.Header),
				}, nil
			})}
			rec := httptest.NewRecorder()
			lbsApiHandler(rec, httptest.NewRequest(http.MethodGet, target, nil))
			wantStatus := http.StatusServiceUnavailable
			if networkError {
				wantStatus = http.StatusBadRequest
			}
			if rec.Code != wantStatus || rec.Result().Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s (networkError=%t): status=%d, Cache-Control=%q", target, networkError,
					rec.Code, rec.Result().Header.Get("Cache-Control"))
			}
		}
	}
}
