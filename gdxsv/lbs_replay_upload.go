package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

// POST /lbs/replay_upload: flycast posts a battle's replay as a multipart file named
// <battle_code>.pb after the battle (GdxUploadReplay). The file goes to the replay bucket
// (GDXSV_REPLAY_BUCKET) as replays/<battle_code>.pb, gzip encoded, and its public URL is
// recorded on the battle (served by /lbs/replay). 409 when that replay is already there.

const (
	replayUploadPrefix  = "replays/"
	replayUploadMaxSize = 32 << 20
	replayUploadMemory  = 10 << 20
)

var errReplayExists = errors.New("replay already uploaded")

// replayStorer keeps uploaded replays; replayStore is nil when uploads are off.
type replayStorer interface {
	// Put stores the object unless it exists (errReplayExists) and returns its public URL.
	Put(ctx context.Context, name string, r io.Reader) (string, error)
}

var replayStore replayStorer

// gcsReplayStore writes gzip encoded objects to a public GCS bucket.
type gcsReplayStore struct {
	bucketName string
	bucket     *storage.BucketHandle
}

func newGCSReplayStore(ctx context.Context, bucketName, keyPath string) (*gcsReplayStore, error) {
	var opts []option.ClientOption
	if keyPath != "" {
		opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, keyPath))
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &gcsReplayStore{bucketName: bucketName, bucket: client.Bucket(bucketName)}, nil
}

func (s *gcsReplayStore) Put(ctx context.Context, name string, r io.Reader) (string, error) {
	obj := s.bucket.Object(name)
	url := "https://storage.googleapis.com/" + s.bucketName + "/" + name
	if _, err := obj.Attrs(ctx); err == nil {
		return url, errReplayExists
	} else if !errors.Is(err, storage.ErrObjectNotExist) {
		return "", fmt.Errorf("attrs: %w", err)
	}

	// DoesNotExist: a second upload racing this one fails instead of overwriting.
	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	w.ContentEncoding = "gzip"
	w.CacheControl = "no-transform"
	gw := gzip.NewWriter(w)
	if _, err := io.Copy(gw, r); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("copy: %w", err)
	}
	if err := gw.Close(); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("gzip close: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("writer close: %w", err)
	}
	return url, nil
}

type replayUploadHandler struct {
	store  replayStorer
	setURL func(battleCode, url string) error
}

func (h *replayUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if h.store == nil {
		http.Error(w, "replay upload is disabled", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, replayUploadMaxSize)
	if err := r.ParseMultipartForm(replayUploadMemory); err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	// the first file of the form, whatever its field name
	var fh *multipart.FileHeader
	for _, headers := range r.MultipartForm.File {
		if len(headers) > 0 {
			fh = headers[0]
			break
		}
	}
	if fh == nil {
		http.Error(w, "No file", http.StatusBadRequest)
		return
	}
	battleCode := strings.TrimSuffix(fh.Filename, ".pb")
	if battleCode == "" || battleCode == fh.Filename || strings.ContainsAny(battleCode, "/\\") {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}
	file, err := fh.Open()
	if err != nil {
		http.Error(w, "Unable to open file", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	url, err := h.store.Put(ctx, replayUploadPrefix+fh.Filename, file)
	cancel()
	_ = file.Close()
	if errors.Is(err, errReplayExists) {
		http.Error(w, "Already uploaded", http.StatusConflict)
		return
	}
	if err != nil {
		logger.Warn("replay_upload: store failure", zap.String("battle_code", battleCode), zap.Error(err))
		http.Error(w, "Failed to upload file", http.StatusInternalServerError)
		return
	}
	if err := h.setURL(battleCode, url); err != nil {
		logger.Warn("replay_upload: SetReplayURL failure", zap.String("battle_code", battleCode), zap.Error(err))
		http.Error(w, "Failed to record replay", http.StatusInternalServerError)
		return
	}
	logger.Info("replay uploaded", zap.String("battle_code", battleCode), zap.Int64("size", fh.Size))
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "File uploaded: %q (%v bytes)\n", fh.Filename, fh.Size)
}
