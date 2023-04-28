package function

import (
	"compress/gzip"
	"context"
	"fmt"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/storage"
)

func init() {
	functions.HTTP("FunctionEntryPoint", uploadHandler)
}

var (
	bucketName     = "gdxsv"
	uploadBasePath = "replays/"
	ErrExist       = os.ErrExist
)

func fileExists(ctx context.Context, bucketName, objectName string) (bool, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return false, fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	_, err = bucket.Object(objectName).Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("Object(%q).Attrs: %v", objectName, err)
	}
	return true, nil
}

func uploadFileToGCS(ctx context.Context, bucketName, objectName string, r io.Reader) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	obj := bucket.Object(objectName)

	// すでに同じファイルが存在する場合はアップロードしない
	exists, err := fileExists(ctx, bucketName, objectName)
	if err != nil {
		return fmt.Errorf("fileExists: %v", err)
	}
	if exists {
		return ErrExist
	}

	w := obj.NewWriter(ctx)
	w.ContentEncoding = "gzip"
	w.CacheControl = "no-transform"
	gw := gzip.NewWriter(w)

	if _, err = io.Copy(gw, r); err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzipWriter.Close: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %v", err)
	}

	return nil
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 2 * 1024 * 1024 // 2 megabytes.
	ctx := context.Background()

	// ParseMultipartForm parses a request body as multipart/form-data.
	// The whole request body is parsed and up to a total of maxMemory bytes of
	// its file parts are stored in memory, with the remainder stored on
	// disk in temporary files.

	// Note that any files saved during a particular invocation may not
	// persist after the current invocation completes; persistent files
	// should be stored elsewhere, such as in a Cloud Storage bucket.
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		log.Printf("Error parsing form: %v", err)
		return
	}

	// Be sure to remove all temporary files after your function is finished.
	defer func() {
		if err := r.MultipartForm.RemoveAll(); err != nil {
			http.Error(w, "Error cleaning up form files", http.StatusInternalServerError)
			log.Printf("Error cleaning up form files: %v", err)
		}
	}()

	// r.MultipartForm.File contains *multipart.FileHeader values for every
	// file in the form. You can access the file contents using
	// *multipart.FileHeader's Open method.
	for _, headers := range r.MultipartForm.File {
		for _, h := range headers {
			if !strings.HasSuffix(h.Filename, ".pb") {
				http.Error(w, "Invalid file name", http.StatusBadRequest)
				return
			}

			file, err := h.Open()
			if err != nil {
				http.Error(w, "Unable to open file", http.StatusBadRequest)
				return
			}

			err = uploadFileToGCS(ctx, bucketName, uploadBasePath+h.Filename, file)
			file.Close()
			if err == ErrExist {
				http.Error(w, "Already uploaded", http.StatusConflict)
			} else if err != nil {
				http.Error(w, "Failed to upload file", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "File uploaded: %q (%v bytes)\n", h.Filename, h.Size)
			return
		}
	}

	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "No file")
}
