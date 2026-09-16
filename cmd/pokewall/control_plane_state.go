package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"
)

func (cp *controlPlane) restoreWall(w *Wall) error {
	var raw []byte
	err := cp.db.QueryRow(`SELECT state_json FROM control_plane_state WHERE id=1`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load control-plane state: %w", err)
	}
	if err == nil && len(raw) > 0 {
		var ps persistedState
		if err := json.Unmarshal(raw, &ps); err != nil {
			return fmt.Errorf("decode control-plane state: %w", err)
		}
		restorePersistedState(w, ps)
	}
	links, err := cp.loadIssueLinks()
	if err != nil {
		return err
	}
	outbox, err := cp.loadIssueOutbox()
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.issueLinks = links
	w.outbox = outbox
	w.mu.Unlock()
	return nil
}

func restorePersistedState(w *Wall, ps persistedState) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.order = nil
	w.queue = nil
	w.tiles = make(map[string]*Tile)
	w.cancel = make(map[string]bool)
	for _, id := range ps.Order {
		pt, ok := ps.Tiles[id]
		if !ok {
			continue
		}
		w.order = append(w.order, id)
		w.tiles[id] = &Tile{
			RunID: pt.RunID, Status: pt.Status, Planner: pt.Planner, Starter: pt.Starter,
			Dest: pt.Dest, Goal: pt.Goal, LLMProfile: pt.LLMProfile, LLMDeployment: pt.LLMDeployment,
			ExperimentID: pt.ExperimentID, ExperimentArm: pt.ExperimentArm, ExperimentCase: pt.ExperimentCase,
			ReasoningEffort: pt.ReasoningEffort,
			Seed:            pt.Seed, FPS: pt.FPS, MaxRounds: pt.MaxRounds, MaxFrames: pt.MaxFrames,
			Endless: pt.Endless, RandomSeed: pt.RandomSeed,
			QueuedAt: timeFromUnix(pt.QueuedAt), EndedAt: timeFromUnix(pt.EndedAt),
			Attempts: pt.Attempts, ErrorAttempts: pt.ErrorAttempts, LossRecoveries: pt.LossRecoveries,
			Frame: pt.Frame, Map: pt.Map, X: pt.X, Y: pt.Y,
			Trace: pt.Trace, Question: pt.Question, Decision: pt.Decision, StopSoFar: pt.StopSoFar,
			Stats: pt.Stats, Player: pt.Player, Reason: pt.Reason, Detail: pt.Detail,
			Finished: pt.Finished, workerAddrs: append([]string(nil), pt.WorkerAddrs...),
			ReplayAvailable: pt.ReplayAvailable, ResumeFromRunID: pt.ResumeFromRunID, lastUpdate: now,
		}
	}
	w.queue = append(w.queue, ps.Queue...)
}

func (cp *controlPlane) loadIssueLinks() (map[string]IssueLink, error) {
	rows, err := cp.db.Query(`SELECT failure_key,payload_json FROM issue_links`)
	if err != nil {
		return nil, fmt.Errorf("load issue links: %w", err)
	}
	defer rows.Close()
	out := map[string]IssueLink{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var link IssueLink
		if err := json.Unmarshal(raw, &link); err != nil {
			return nil, fmt.Errorf("decode issue link %s: %w", key, err)
		}
		out[key] = link
	}
	return out, rows.Err()
}

func (cp *controlPlane) loadIssueOutbox() (map[string]outboxEntry, error) {
	rows, err := cp.db.Query(`SELECT external_id,payload_json FROM issue_outbox`)
	if err != nil {
		return nil, fmt.Errorf("load issue outbox: %w", err)
	}
	defer rows.Close()
	out := map[string]outboxEntry{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var entry outboxEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("decode issue outbox %s: %w", id, err)
		}
		out[id] = entry
	}
	return out, rows.Err()
}

func (cp *controlPlane) persistWall(w *Wall) error {
	w.mu.Lock()
	ps := w.persistedStateLocked()
	links := copyIssueLink(w.issueLinks)
	outbox := copyOutbox(w.outbox)
	w.mu.Unlock()
	// Protocol state is normalized below rather than duplicated inside the
	// opaque active scheduler snapshot.
	ps.IssueLinks = nil
	ps.Outbox = nil
	stateRaw, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	tx, err := cp.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`INSERT INTO control_plane_state(id,state_json,updated_at) VALUES(1,$1::jsonb,NOW()) ON CONFLICT(id) DO UPDATE SET state_json=EXCLUDED.state_json,updated_at=NOW()`, string(stateRaw)); err != nil {
		return fmt.Errorf("persist wall state: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM issue_links`); err != nil {
		return err
	}
	keys := make([]string, 0, len(links))
	for key := range links {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		link := links[key]
		raw, _ := json.Marshal(link)
		if _, err := tx.Exec(`INSERT INTO issue_links(failure_key,issue_id,status,fingerprint,payload_json,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,NOW())`, key, link.IssueID, link.Status, link.Fingerprint, string(raw)); err != nil {
			return fmt.Errorf("persist issue link %s: %w", key, err)
		}
		if link.Fingerprint != "" {
			if _, err := tx.Exec(`INSERT INTO issue_fingerprints(failure_key,fingerprint,payload_json,updated_at) VALUES($1,$2,$3::jsonb,NOW()) ON CONFLICT(failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,payload_json=EXCLUDED.payload_json,updated_at=NOW()`, key, link.Fingerprint, string(raw)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM issue_outbox`); err != nil {
		return err
	}
	ids := make([]string, 0, len(outbox))
	for id := range outbox {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := outbox[id]
		raw, _ := json.Marshal(e)
		if _, err := tx.Exec(`INSERT INTO issue_outbox(external_id,run_id,attempt,failure_key,status,next_attempt,payload_json,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NOW())`, e.ExternalID, e.RunID, e.Attempt, e.Key, e.Status, e.NextAttempt, string(raw)); err != nil {
			return fmt.Errorf("persist issue outbox %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RunControlPlaneSweep is the safety net for high-frequency mutations that are
// intentionally not synchronous database writes (heartbeats and remote issue
// status refreshes). Lifecycle requests are committed by the HTTP middleware.
func (w *Wall) RunControlPlaneSweep(interval time.Duration) {
	cp := controlPlaneFor(w)
	if cp == nil {
		return
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := cp.persistWall(w); err != nil {
			log.Printf("pokewall: persist control-plane state: %v", err)
		}
		if err := w.syncCatalogFromRAM(false); err != nil {
			log.Printf("pokewall: sync active runs to PostgreSQL: %v", err)
		}
		if err := cp.persistExperimentController(w); err != nil {
			log.Printf("pokewall: persist experiments: %v", err)
		}
	}
}
