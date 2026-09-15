package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultWorkerControlPort = "8100"
	workerForceEndTimeout    = 2 * time.Second
	maxWorkerControlError    = 4 << 10
)

type workerTerminator func(context.Context, string) error

// workerControlHTTPHandler adds destructive worker lifecycle actions around the
// normal operator/runtime handler. Runner protocol compatibility remains in the
// fallback; only the private operator relay exposes this route to a browser.
func workerControlHTTPHandler(w *Wall, fallback http.Handler) http.Handler {
	return workerControlHTTPHandlerWithTerminator(w, fallback, requestWorkerForceEnd)
}

func workerControlHTTPHandlerWithTerminator(w *Wall, fallback http.Handler, terminate workerTerminator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/workers/{addr}/force-end", func(res http.ResponseWriter, req *http.Request) {
		w.handleForceEndWorker(res, req, terminate)
	})
	mux.Handle("/", fallback)
	return mux
}

func (w *Wall) handleForceEndWorker(res http.ResponseWriter, req *http.Request, terminate workerTerminator) {
	addr := strings.TrimSpace(req.PathValue("addr"))
	if addr == "" {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "worker address is required"})
		return
	}

	w.mu.Lock()
	worker, ok := w.workers[addr]
	if !ok || worker == nil {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown worker " + addr})
		return
	}
	runID := worker.RunID
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(req.Context(), workerForceEndTimeout)
	err := terminate(ctx, addr)
	cancel()
	if err != nil {
		writeJSON(res, http.StatusBadGateway, map[string]string{"error": "force end worker: " + err.Error()})
		return
	}

	now := time.Now()
	settledRunID := ""
	w.mu.Lock()
	// A final heartbeat may have refreshed this record while the kill request
	// crossed the overlay. Prefer that latest run identity if one exists.
	if current := w.workers[addr]; current != nil && current.RunID != "" {
		runID = current.RunID
	}
	delete(w.workers, addr)
	if runID != "" {
		if tile := w.tiles[runID]; tile != nil && !tile.Finished {
			// Mark the run cancelled before settlement. settleRun treats a user
			// cancellation as terminal, does not retry it, and does not enqueue
			// an endless successor. That is intentionally different from the
			// stale-worker "lost" path.
			w.cancel[runID] = true
			w.settleRun(tile, "cancelled", "worker force-ended: "+addr, now)
			settledRunID = runID
		}
	}
	w.mu.Unlock()

	if settledRunID != "" {
		w.dropFrameCache(settledRunID)
		w.saveState()
	}
	writeJSON(res, http.StatusOK, map[string]any{
		"status": "force-ended",
		"worker": addr,
		"run_id": settledRunID,
	})
}

func requestWorkerForceEnd(ctx context.Context, workerAddr string) error {
	host, _, err := net.SplitHostPort(workerAddr)
	if err != nil {
		return fmt.Errorf("invalid worker address %q: %w", workerAddr, err)
	}
	port := strings.TrimSpace(os.Getenv("POKEPILOT_WORKER_CONTROL_PORT"))
	if port == "" {
		port = defaultWorkerControlPort
	}
	target := "http://" + net.JoinHostPort(host, port) + "/v1/worker/force-end"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: workerForceEndTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxWorkerControlError))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = resp.Status
	}
	return fmt.Errorf("worker control returned %s: %s", resp.Status, detail)
}
