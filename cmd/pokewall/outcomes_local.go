package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// handleLocalOutcomes serves the supported catalog-free local wall mode from
// in-memory tiles. Local walls retain completed tiles; SQLite catalog mode and
// the PostgreSQL control plane have their own authoritative outcomes queries.
func (w *Wall) handleLocalOutcomes(res http.ResponseWriter) {
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
