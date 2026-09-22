package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
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
			RecoveryProfile: pt.RecoveryProfile, Endless: pt.Endless, RandomSeed: pt.RandomSeed,
			QueuedAt: timeFromUnix(pt.QueuedAt), EndedAt: timeFromUnix(pt.EndedAt),
			Attempts: pt.Attempts, ErrorAttempts: pt.ErrorAttempts, LossRecoveries: pt.LossRecoveries,
			RecoveryAttempts: pt.RecoveryAttempts, RecoveryBadges: pt.RecoveryBadges, RecoveryEvents: pt.RecoveryEvents, RecoveryMaps: pt.RecoveryMaps,
			Activity: copyRunActivity(pt.Activity),
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

type controlPlanePersistPayload struct {
	stateRaw   []byte
	stateHash  [sha256.Size]byte
	links      map[string]IssueLink
	linksHash  [sha256.Size]byte
	outbox     map[string]outboxEntry
	outboxHash [sha256.Size]byte
}

type controlPlaneWriteCoordinator struct {
	mu sync.Mutex

	haveState  bool
	stateHash  [sha256.Size]byte
	haveLinks  bool
	linksHash  [sha256.Size]byte
	haveOutbox bool
	outboxHash [sha256.Size]byte
}

var controlPlaneWriteCoordinators sync.Map // *controlPlane -> *controlPlaneWriteCoordinator

func controlPlaneWritesFor(cp *controlPlane) *controlPlaneWriteCoordinator {
	if existing, ok := controlPlaneWriteCoordinators.Load(cp); ok {
		return existing.(*controlPlaneWriteCoordinator)
	}
	created := &controlPlaneWriteCoordinator{}
	actual, _ := controlPlaneWriteCoordinators.LoadOrStore(cp, created)
	return actual.(*controlPlaneWriteCoordinator)
}

// controlPlanePersistedStateLocked keeps only scheduler/recovery state in the
// singleton snapshot. Heartbeat telemetry is live UI data and is reconstructed
// by the next heartbeat after a wall restart; retaining it here turns every
// frame/stats update into a new multi-megabyte TOAST value.
func controlPlanePersistedStateLocked(w *Wall) persistedState {
	ps := w.persistedStateLocked()
	ps.IssueLinks = nil
	ps.Outbox = nil
	for id, pt := range ps.Tiles {
		pt.Frame = 0
		pt.Map = 0
		pt.X = 0
		pt.Y = 0
		pt.Trace = ""
		pt.Question = ""
		pt.Decision = ""
		pt.StopSoFar = ""
		pt.Stats = nil
		pt.Player = nil
		pt.WorkerAddrs = nil
		pt.Activity = durableRunActivity(pt.Activity)
		ps.Tiles[id] = pt
	}
	return ps
}

func captureControlPlanePersistPayload(w *Wall) (controlPlanePersistPayload, error) {
	w.mu.Lock()
	ps := controlPlanePersistedStateLocked(w)
	links := copyIssueLink(w.issueLinks)
	outbox := copyOutbox(w.outbox)
	w.mu.Unlock()

	stateRaw, err := json.Marshal(ps)
	if err != nil {
		return controlPlanePersistPayload{}, err
	}
	linksRaw, err := json.Marshal(links)
	if err != nil {
		return controlPlanePersistPayload{}, err
	}
	outboxRaw, err := json.Marshal(outbox)
	if err != nil {
		return controlPlanePersistPayload{}, err
	}
	return controlPlanePersistPayload{
		stateRaw:   stateRaw,
		stateHash:  sha256.Sum256(stateRaw),
		links:      links,
		linksHash:  sha256.Sum256(linksRaw),
		outbox:     outbox,
		outboxHash: sha256.Sum256(outboxRaw),
	}, nil
}

func (cp *controlPlane) persistWall(w *Wall) error {
	writes := controlPlaneWritesFor(cp)
	writes.mu.Lock()
	defer writes.mu.Unlock()

	payload, err := captureControlPlanePersistPayload(w)
	if err != nil {
		return err
	}
	stateChanged := !writes.haveState || writes.stateHash != payload.stateHash
	linksChanged := !writes.haveLinks || writes.linksHash != payload.linksHash
	outboxChanged := !writes.haveOutbox || writes.outboxHash != payload.outboxHash
	if !stateChanged && !linksChanged && !outboxChanged {
		return nil
	}

	tx, err := cp.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if stateChanged {
		if _, err := tx.Exec(`INSERT INTO control_plane_state(id,state_json,updated_at) VALUES(1,$1::jsonb,NOW()) ON CONFLICT(id) DO UPDATE SET state_json=EXCLUDED.state_json,updated_at=NOW() WHERE control_plane_state.state_json IS DISTINCT FROM EXCLUDED.state_json`, string(payload.stateRaw)); err != nil {
			return fmt.Errorf("persist wall state: %w", err)
		}
	}
	if linksChanged {
		if _, err := tx.Exec(`DELETE FROM issue_links`); err != nil {
			return err
		}
		keys := make([]string, 0, len(payload.links))
		for key := range payload.links {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			link := payload.links[key]
			raw, _ := json.Marshal(link)
			if _, err := tx.Exec(`INSERT INTO issue_links(failure_key,issue_id,status,fingerprint,payload_json,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,NOW())`, key, link.IssueID, link.Status, link.Fingerprint, string(raw)); err != nil {
				return fmt.Errorf("persist issue link %s: %w", key, err)
			}
			if link.Fingerprint != "" {
				if _, err := tx.Exec(`INSERT INTO issue_fingerprints(failure_key,fingerprint,payload_json,updated_at) VALUES($1,$2,$3::jsonb,NOW()) ON CONFLICT(failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,payload_json=EXCLUDED.payload_json,updated_at=NOW() WHERE issue_fingerprints.fingerprint IS DISTINCT FROM EXCLUDED.fingerprint OR issue_fingerprints.payload_json IS DISTINCT FROM EXCLUDED.payload_json`, key, link.Fingerprint, string(raw)); err != nil {
					return err
				}
			}
		}
	}
	if outboxChanged {
		if _, err := tx.Exec(`DELETE FROM issue_outbox`); err != nil {
			return err
		}
		ids := make([]string, 0, len(payload.outbox))
		for id := range payload.outbox {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			e := payload.outbox[id]
			raw, _ := json.Marshal(e)
			if _, err := tx.Exec(`INSERT INTO issue_outbox(external_id,run_id,attempt,failure_key,status,next_attempt,payload_json,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NOW())`, e.ExternalID, e.RunID, e.Attempt, e.Key, e.Status, e.NextAttempt, string(raw)); err != nil {
				return fmt.Errorf("persist issue outbox %s: %w", id, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if stateChanged {
		writes.stateHash = payload.stateHash
		writes.haveState = true
	}
	if linksChanged {
		writes.linksHash = payload.linksHash
		writes.haveLinks = true
	}
	if outboxChanged {
		writes.outboxHash = payload.outboxHash
		writes.haveOutbox = true
	}
	return nil
}

// RunControlPlaneSweep is a safety net for background scheduler/issue changes
// that do not pass through the HTTP mutation middleware (for example stale-run
// reaping and remote issue status refreshes). persistWall hashes the compact
// recovery snapshot, so unchanged sweeps perform no PostgreSQL writes. Active
// run telemetry is deliberately not copied to runs or control_plane_state here.
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
	}
}
