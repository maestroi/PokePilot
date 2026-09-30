package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// localOutcomesHTTPHandler serves the narrow analytics feed from RAM in the
// supported local/dev mode where no catalog is configured. Catalog and
// PostgreSQL deployments keep their durable history handlers authoritative.
func (w *Wall) localOutcomesHTTPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/v1/outcomes" && catalogFor(w) == nil {
			w.handleRAMOutcomes(res)
			return
		}
		next.ServeHTTP(res, req)
	})
}

// handleRAMOutcomes mirrors the catalog-backed narrow projection using the
// in-memory tile set. A catalog-disabled wall keeps finished runs in RAM, so
// this contains the same logical run population without serializing the full
// dashboard payload.
func (w *Wall) handleRAMOutcomes(res http.ResponseWriter) {
	w.mu.Lock()
	rows := make([]tileRow, 0, len(w.order))
	for _, id := range w.order {
		if t := w.tiles[id]; t != nil {
			rows = append(rows, w.tileRowLocked(t))
		}
	}
	w.mu.Unlock()

	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(res, `{"runs":[`)
	first := true
	for _, row := range rows {
		data, err := json.Marshal(outcomeRun(row))
		if err != nil {
			continue
		}
		if !first {
			_, _ = io.WriteString(res, ",")
		}
		first = false
		_, _ = res.Write(data)
	}
	_, _ = io.WriteString(res, `]}`)
}
