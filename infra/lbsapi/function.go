package function

import (
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"io"
	"net/http"
	"sync"
	"time"
)

func init() {
	functions.HTTP("FunctionEntryPoint", lbsApiHandler)
}

type ResponseCache struct {
	Time time.Time
	Body []byte
}

var (
	mtx   sync.Mutex
	cache = make(map[string]*ResponseCache)
)

func clearOldCacheLocked() {
	for k, v := range cache {
		if 3.0 < time.Since(v.Time).Seconds() {
			delete(cache, k)
		}
	}
}

func lbsApiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusBadRequest)
	}

	key := r.URL.RequestURI()
	mtx.Lock()
	clearOldCacheLocked()
	v, ok := cache[key]
	mtx.Unlock()

	if ok {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("FromCache", "yes")
		w.WriteHeader(http.StatusOK)
		w.Write(v.Body)
		return
	}

	resp, err := http.Get("http://zdxsv.net:9880/lbs/" + r.URL.RequestURI())
	if err != nil {
		http.Error(w, "Unable to request lobby", http.StatusBadRequest)
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		mtx.Lock()
		cache[key] = &ResponseCache{time.Now(), body}
		mtx.Unlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}
