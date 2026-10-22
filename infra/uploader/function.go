package function

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

// Compatibility shim for flycast builds older than gdxsv-1.9.7, which post their
// replays here. The lobby stores replays itself now (POST /lbs/replay_upload, gdxsv
// v1.4.3+), so this function only forwards the multipart request to it and relays
// the answer (200 uploaded, 409 already uploaded, as before).

const uploadURL = "http://lbs.gdxsv.net:9880/lbs/replay_upload"

var client = &http.Client{Timeout: 2 * time.Minute}

func init() {
	functions.HTTP("FunctionEntryPoint", uploadHandler)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, uploadURL, r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	req.ContentLength = r.ContentLength

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("forward to lobby failed: %v", err)
		http.Error(w, "Unable to reach lobby", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
