package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

// captureFinishFrame snapshots the screen while the runner is still attached
// to the active tile. The finish handler also performs its own final fetch for
// the in-memory history cache; this copy belongs to PostgreSQL persistence and
// must happen before the catalog can evict the completed tile.
func (w *Wall) captureFinishFrame(runID string) []byte {
	w.mu.Lock()
	t := w.tiles[runID]
	var addrs []string
	var cached []byte
	if t != nil && !t.Finished {
		addrs = append([]string(nil), t.workerAddrs...)
		cached = append([]byte(nil), t.lastFrame...)
	}
	w.mu.Unlock()

	if len(addrs) > 0 {
		if data, err := fetchRunnerFrame(addrs); err == nil && len(data) > 0 {
			return data
		}
	}
	return cached
}

// storedFinalFrame returns the newest durable finish screenshot for a run that
// is still present in completed-run history. New screenshots are stored inline
// in PostgreSQL; older rows may point at S3 and are materialized through the
// existing artifact reader so deployments can recover thumbnails already
// captured before the inline-thumbnail change.
func (cp *controlPlane) storedFinalFrame(runID string) ([]byte, bool, error) {
	runID = strings.TrimSpace(runID)
	if cp == nil || runID == "" {
		return nil, false, nil
	}

	var raw, inline []byte
	var hasInline bool
	err := cp.db.QueryRow(`
SELECT a.metadata_json,a.inline_data,a.inline_data IS NOT NULL
FROM artifacts a
JOIN runs r ON r.run_id=a.run_id
WHERE a.run_id=$1 AND r.status=$2 AND a.kind='finish' AND a.name='final-frame.png'
ORDER BY a.attempt DESC
LIMIT 1`, runID, statusDone).Scan(&raw, &inline, &hasInline)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var meta farm.Artifact
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, false, fmt.Errorf("decode final frame metadata: %w", err)
	}
	if meta.Name != "final-frame.png" || !strings.EqualFold(strings.TrimSpace(meta.MediaType), "image/png") {
		return nil, false, fmt.Errorf("invalid final frame metadata for %s", runID)
	}

	if hasInline {
		if len(inline) == 0 || len(inline) > maxRunnerFrameBytes {
			return nil, false, fmt.Errorf("invalid inline final frame size %d for %s", len(inline), runID)
		}
		return append([]byte(nil), inline...), true, nil
	}

	art, err := cp.materializeStoredArtifact(storedCheckpointArtifact{meta: meta})
	if err != nil {
		return nil, false, fmt.Errorf("materialize final frame: %w", err)
	}
	if len(art.Data) == 0 || len(art.Data) > maxRunnerFrameBytes {
		return nil, false, fmt.Errorf("invalid stored final frame size %d for %s", len(art.Data), runID)
	}
	return art.Data, true, nil
}

// controlPlaneFrameHTTPHandler gives /frame a durable-history fallback in
// PostgreSQL mode. Live/requeued runs always stay on the normal runner/RAM
// path; only completed catalog rows with no RAM frame consult durable storage.
func (w *Wall) controlPlaneFrameHTTPHandler(next http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil {
		return next
	}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/frame" {
			next.ServeHTTP(res, req)
			return
		}
		runID := strings.TrimSpace(req.URL.Query().Get("run"))
		if runID == "" {
			next.ServeHTTP(res, req)
			return
		}

		w.mu.Lock()
		t := w.tiles[runID]
		active := t != nil && !t.Finished
		hasRAMFrame := t != nil && len(t.lastFrame) > 0
		w.mu.Unlock()
		if active || hasRAMFrame {
			next.ServeHTTP(res, req)
			return
		}

		data, ok, err := cp.storedFinalFrame(runID)
		if err != nil {
			// A missing/corrupt thumbnail must not turn an otherwise healthy
			// operator page into a 500. Preserve the old endpoint behavior and
			// let the UI's NO FRAME state explain that this optional media is gone.
			log.Printf("pokewall: load final frame %s: %v", runID, err)
			next.ServeHTTP(res, req)
			return
		}
		if !ok {
			next.ServeHTTP(res, req)
			return
		}
		writePNG(res, data)
	})
}
