package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

const controlPlaneMigration002 = `
CREATE TABLE IF NOT EXISTS spectator_run_settings (
    run_id TEXT PRIMARY KEY REFERENCES runs(run_id) ON DELETE CASCADE,
    visible BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS spectator_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    featured_run_id TEXT REFERENCES runs(run_id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO spectator_state(id, featured_run_id) VALUES(1, NULL)
ON CONFLICT (id) DO NOTHING;
`

type spectatorRunControl struct {
	Visible bool `json:"visible"`
}

type spectatorControlSnapshot struct {
	FeaturedRunID string                         `json:"featured_run_id,omitempty"`
	Runs          map[string]spectatorRunControl `json:"runs"`
}

type spectatorRunControlPatch struct {
	Visible  *bool `json:"visible,omitempty"`
	Featured *bool `json:"featured,omitempty"`
}

type spectatorRunControlResult struct {
	RunID         string `json:"run_id"`
	Visible       bool   `json:"visible"`
	Featured      bool   `json:"featured"`
	FeaturedRunID string `json:"featured_run_id,omitempty"`
}

type spectatorControlMemory struct {
	mu       sync.Mutex
	visible  map[string]bool
	featured string
}

var wallSpectatorControls sync.Map // *Wall -> *spectatorControlMemory

func spectatorControlMemoryFor(w *Wall) *spectatorControlMemory {
	if value, ok := wallSpectatorControls.Load(w); ok {
		return value.(*spectatorControlMemory)
	}
	created := &spectatorControlMemory{visible: make(map[string]bool)}
	value, _ := wallSpectatorControls.LoadOrStore(w, created)
	return value.(*spectatorControlMemory)
}

func (cp *controlPlane) migrateSpectatorControl() error {
	if cp == nil || cp.db == nil {
		return nil
	}
	tx, err := cp.db.Begin()
	if err != nil {
		return fmt.Errorf("begin spectator-control migration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var applied bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=2)`).Scan(&applied); err != nil {
		return fmt.Errorf("read spectator-control migration version: %w", err)
	}
	if !applied {
		if _, err := tx.Exec(controlPlaneMigration002); err != nil {
			return fmt.Errorf("apply control-plane migration 2: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(2) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 2: %w", err)
		}
	}
	return tx.Commit()
}

func spectatorControlHTTPHandler(w *Wall, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/spectator/control", w.handleSpectatorControl)
	mux.HandleFunc("PATCH /v1/runs/{id}/spectator", w.handleSpectatorRunControl)
	mux.Handle("/", next)
	return mux
}

func (w *Wall) handleSpectatorControl(res http.ResponseWriter, req *http.Request) {
	snapshot, err := w.spectatorControlSnapshot(req.Context())
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	res.Header().Set("Cache-Control", "no-store")
	writeJSON(res, http.StatusOK, snapshot)
}

func (w *Wall) handleSpectatorRunControl(res http.ResponseWriter, req *http.Request) {
	runID := strings.TrimSpace(req.PathValue("id"))
	if runID == "" || len(runID) > 256 {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	req.Body = http.MaxBytesReader(res, req.Body, maxSmallControlBody)
	var patch spectatorRunControlPatch
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad spectator control: " + err.Error()})
		return
	}
	if patch.Visible == nil && patch.Featured == nil {
		writeJSON(res, http.StatusBadRequest, map[string]string{"error": "visible or featured is required"})
		return
	}

	result, err := w.patchSpectatorRunControl(req.Context(), runID, patch)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "run not found: " + runID})
		return
	}
	if err != nil {
		writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(res, http.StatusOK, result)
}

func (w *Wall) spectatorControlSnapshot(ctx context.Context) (spectatorControlSnapshot, error) {
	if cp := controlPlaneFor(w); cp != nil {
		return cp.spectatorControlSnapshot(ctx)
	}
	memory := spectatorControlMemoryFor(w)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	out := spectatorControlSnapshot{
		FeaturedRunID: memory.featured,
		Runs:          make(map[string]spectatorRunControl, len(memory.visible)),
	}
	for runID, visible := range memory.visible {
		out.Runs[runID] = spectatorRunControl{Visible: visible}
	}
	return out, nil
}

func (cp *controlPlane) spectatorControlSnapshot(ctx context.Context) (spectatorControlSnapshot, error) {
	out := spectatorControlSnapshot{Runs: make(map[string]spectatorRunControl)}
	rows, err := cp.db.QueryContext(ctx, `SELECT run_id, visible FROM spectator_run_settings`)
	if err != nil {
		return out, fmt.Errorf("read spectator run settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var runID string
		var visible bool
		if err := rows.Scan(&runID, &visible); err != nil {
			return out, fmt.Errorf("scan spectator run setting: %w", err)
		}
		out.Runs[runID] = spectatorRunControl{Visible: visible}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iterate spectator run settings: %w", err)
	}
	var featured sql.NullString
	if err := cp.db.QueryRowContext(ctx, `SELECT featured_run_id FROM spectator_state WHERE id=1`).Scan(&featured); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("read featured spectator run: %w", err)
	}
	if featured.Valid {
		out.FeaturedRunID = featured.String
	}
	return out, nil
}

func (w *Wall) patchSpectatorRunControl(ctx context.Context, runID string, patch spectatorRunControlPatch) (spectatorRunControlResult, error) {
	if cp := controlPlaneFor(w); cp != nil {
		return cp.patchSpectatorRunControl(ctx, runID, patch)
	}

	w.mu.Lock()
	_, exists := w.tiles[runID]
	w.mu.Unlock()
	if !exists {
		return spectatorRunControlResult{}, sql.ErrNoRows
	}

	memory := spectatorControlMemoryFor(w)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	visible := true
	if current, ok := memory.visible[runID]; ok {
		visible = current
	}
	if patch.Visible != nil {
		visible = *patch.Visible
		memory.visible[runID] = visible
	}
	if patch.Featured != nil {
		if *patch.Featured {
			visible = true
			memory.visible[runID] = true
			memory.featured = runID
		} else if memory.featured == runID {
			memory.featured = ""
		}
	}
	if !visible && memory.featured == runID {
		memory.featured = ""
	}
	return spectatorRunControlResult{
		RunID:         runID,
		Visible:       visible,
		Featured:      memory.featured == runID,
		FeaturedRunID: memory.featured,
	}, nil
}

func (cp *controlPlane) patchSpectatorRunControl(ctx context.Context, runID string, patch spectatorRunControlPatch) (spectatorRunControlResult, error) {
	tx, err := cp.db.BeginTx(ctx, nil)
	if err != nil {
		return spectatorRunControlResult{}, fmt.Errorf("begin spectator control update: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE run_id=?)`, runID).Scan(&exists); err != nil {
		return spectatorRunControlResult{}, fmt.Errorf("find spectator run: %w", err)
	}
	if !exists {
		return spectatorRunControlResult{}, sql.ErrNoRows
	}

	visible := true
	if err := tx.QueryRowContext(ctx, `SELECT visible FROM spectator_run_settings WHERE run_id=?`, runID).Scan(&visible); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return spectatorRunControlResult{}, fmt.Errorf("read spectator run setting: %w", err)
	}
	var featured sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT featured_run_id FROM spectator_state WHERE id=1`).Scan(&featured); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return spectatorRunControlResult{}, fmt.Errorf("read featured spectator run: %w", err)
	}
	featuredRunID := ""
	if featured.Valid {
		featuredRunID = featured.String
	}

	if patch.Visible != nil {
		visible = *patch.Visible
		if _, err := tx.ExecContext(ctx, `
INSERT INTO spectator_run_settings(run_id, visible, updated_at) VALUES(?, ?, NOW())
ON CONFLICT(run_id) DO UPDATE SET visible=excluded.visible, updated_at=NOW()`, runID, visible); err != nil {
			return spectatorRunControlResult{}, fmt.Errorf("update spectator visibility: %w", err)
		}
	}

	if patch.Featured != nil {
		if *patch.Featured {
			visible = true
			if _, err := tx.ExecContext(ctx, `
INSERT INTO spectator_run_settings(run_id, visible, updated_at) VALUES(?, TRUE, NOW())
ON CONFLICT(run_id) DO UPDATE SET visible=TRUE, updated_at=NOW()`, runID); err != nil {
				return spectatorRunControlResult{}, fmt.Errorf("make featured run visible: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE spectator_state SET featured_run_id=?, updated_at=NOW() WHERE id=1`, runID); err != nil {
				return spectatorRunControlResult{}, fmt.Errorf("feature spectator run: %w", err)
			}
			featuredRunID = runID
		} else if featuredRunID == runID {
			if _, err := tx.ExecContext(ctx, `UPDATE spectator_state SET featured_run_id=NULL, updated_at=NOW() WHERE id=1`); err != nil {
				return spectatorRunControlResult{}, fmt.Errorf("clear featured spectator run: %w", err)
			}
			featuredRunID = ""
		}
	}

	if !visible && featuredRunID == runID {
		if _, err := tx.ExecContext(ctx, `UPDATE spectator_state SET featured_run_id=NULL, updated_at=NOW() WHERE id=1`); err != nil {
			return spectatorRunControlResult{}, fmt.Errorf("hide featured spectator run: %w", err)
		}
		featuredRunID = ""
	}

	if err := tx.Commit(); err != nil {
		return spectatorRunControlResult{}, fmt.Errorf("commit spectator control update: %w", err)
	}
	return spectatorRunControlResult{
		RunID:         runID,
		Visible:       visible,
		Featured:      featuredRunID == runID,
		FeaturedRunID: featuredRunID,
	}, nil
}
