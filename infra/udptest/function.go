package function

import (
	"fmt"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"log"
	"net"
	"net/http"
)

func init() {
	functions.HTTP("FunctionEntryPoint", udpTestHandler)
}

func udpTestHandler(w http.ResponseWriter, r *http.Request) {
	addr := ""

	if r.Method == "GET" {
		if r.URL.Query().Get("addr") != "" {
			addr = r.URL.Query().Get("addr")
		}
	}

	if r.Method == "POST" {
		const maxMemory = 1024 * 1024

		if err := r.ParseMultipartForm(maxMemory); err != nil {
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			log.Printf("Error parsing form: %v", err)
			return
		}

		defer func() {
			if err := r.MultipartForm.RemoveAll(); err != nil {
				http.Error(w, "Error cleaning up form files", http.StatusInternalServerError)
				log.Printf("Error cleaning up form files: %v", err)
			}
		}()

		if r.FormValue("addr") != "" {
			addr = r.FormValue("addr")
		}
	}

	if addr != "" {
		conn, err := net.Dial("udp", addr)
		if err != nil {
			fmt.Printf("Some error %v", err)
			return
		}
		conn.Write([]byte("Hello"))
		conn.Close()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Sent"))
		return
	}

	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("No addr"))
}
