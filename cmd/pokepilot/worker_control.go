package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultWorkerControlPort = "8100"
	workerExitDelay          = 25 * time.Millisecond
	workerControlHeaderWait  = 5 * time.Second
)

// The worker control listener is intentionally private to the farm overlay.
// It is never published by the stack or proxied by the spectator surface.
// Pokewall uses it only for the explicit operator "force end" action.
func init() {
	if strings.TrimSpace(os.Getenv("POKEPILOT_ORCH_URL")) == "" {
		return
	}
	port := strings.TrimSpace(os.Getenv("POKEPILOT_WORKER_CONTROL_PORT"))
	if port == "" {
		port = defaultWorkerControlPort
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", port))
	if err != nil {
		log.Fatalf("farm: worker control listen: %v", err)
	}
	server := &http.Server{
		Handler:           newWorkerControlHandler(os.Exit),
		ReadHeaderTimeout: workerControlHeaderWait,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("farm: worker control server stopped: %v", err)
		}
	}()
	log.Printf("farm: worker control listening on %s", listener.Addr())
}

func newWorkerControlHandler(exit func(int)) http.Handler {
	mux := http.NewServeMux()
	var once sync.Once
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeWorkerControlJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/worker/force-end", func(w http.ResponseWriter, _ *http.Request) {
		writeWorkerControlJSON(w, http.StatusAccepted, map[string]string{"status": "force-ending"})
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		once.Do(func() {
			go func() {
				// Give the 202 response a chance to leave the socket before the
				// process disappears. We deliberately do not wait for gameplay,
				// checkpoint uploads, model calls, or any other worker goroutine.
				time.Sleep(workerExitDelay)
				exit(0)
			}()
		})
	})
	return mux
}

func writeWorkerControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
