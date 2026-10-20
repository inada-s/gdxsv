package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeReplayStore struct {
	objects map[string][]byte
	fail    error
}

func (s *fakeReplayStore) Put(_ context.Context, name string, r io.Reader) (string, error) {
	if s.fail != nil {
		return "", s.fail
	}
	if _, ok := s.objects[name]; ok {
		return "https://storage.googleapis.com/gdxsv/" + name, errReplayExists
	}
	b, _ := io.ReadAll(r)
	s.objects[name] = b
	return "https://storage.googleapis.com/gdxsv/" + name, nil
}

func replayUploadRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	mw.Close()
	req := httptest.NewRequest("POST", "/lbs/replay_upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestReplayUpload(t *testing.T) {
	store := &fakeReplayStore{objects: map[string][]byte{}}
	recorded := map[string]string{}
	h := &replayUploadHandler{store: store, setURL: func(code, url string) error {
		recorded[code] = url
		return nil
	}}
	do := func(req *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	w := do(replayUploadRequest(t, "C1234.pb", []byte("replay-bytes")))
	if w.Code != 200 || string(store.objects["replays/C1234.pb"]) != "replay-bytes" {
		t.Fatalf("code %d body %s objects %v", w.Code, w.Body.String(), store.objects)
	}
	if recorded["C1234"] != "https://storage.googleapis.com/gdxsv/replays/C1234.pb" {
		t.Errorf("recorded %v", recorded)
	}

	// the same replay again: 409, as the Cloud Function answered (flycast treats it as success)
	if w := do(replayUploadRequest(t, "C1234.pb", []byte("replay-bytes"))); w.Code != http.StatusConflict {
		t.Errorf("second upload code %d", w.Code)
	}

	for _, name := range []string{"C1234.txt", ".pb"} {
		if w := do(replayUploadRequest(t, name, []byte("x"))); w.Code != http.StatusBadRequest {
			t.Errorf("%q: code %d", name, w.Code)
		}
	}
	// a path in the file name: Go keeps only its base name
	if w := do(replayUploadRequest(t, "../a/b.pb", []byte("y"))); w.Code != 200 || string(store.objects["replays/b.pb"]) != "y" || recorded["b"] == "" {
		t.Errorf("path name: code %d objects %v recorded %v", w.Code, store.objects, recorded)
	}
	if w := do(httptest.NewRequest("GET", "/lbs/replay_upload", nil)); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET code %d", w.Code)
	}
	if w := do(httptest.NewRequest("POST", "/lbs/replay_upload", bytes.NewReader(nil))); w.Code != http.StatusBadRequest {
		t.Errorf("no form code %d", w.Code)
	}

	store.fail = errors.New("gcs down")
	if w := do(replayUploadRequest(t, "C5678.pb", []byte("x"))); w.Code != http.StatusInternalServerError || recorded["C5678"] != "" {
		t.Errorf("store failure code %d recorded %v", w.Code, recorded)
	}

	off := &replayUploadHandler{}
	if w := httptest.NewRecorder(); true {
		off.ServeHTTP(w, replayUploadRequest(t, "C1.pb", []byte("x")))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("disabled code %d", w.Code)
		}
	}
}
