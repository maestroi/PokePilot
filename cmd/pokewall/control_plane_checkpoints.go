package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

var (
	errStoredCheckpointNotFound     = errors.New("stored checkpoint not found")
	errCheckpointStale              = errors.New("stale checkpoint")
	errCheckpointRunFinished        = errors.New("checkpoint run already finished")
	errCheckpointArtifactInvalid    = errors.New("invalid checkpoint artifact")
	errCheckpointStorageUnavailable = errors.New("checkpoint artifact storage unavailable")
)

// Migration 2 adds the only artifact payload column PostgreSQL is allowed to
// carry: small structured JSON. Emulator state/RAM/video/replay bytes are
// always object-storage references.
func (cp *controlPlane) migrateCheckpointArtifacts() error {
	tx, err := cp.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var applied bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=2)`).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		if _, err := tx.Exec(`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS inline_data BYTEA`); err != nil {
			return fmt.Errorf("control-plane migration 2: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(2) ON CONFLICT DO NOTHING`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func checkpointRequestRunID(path string) (string, bool) {
	const prefix = "/v1/runs/"
	const suffix = "/checkpoint"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func checkpointHTTPStatus(err error) int {
	switch {
	case errors.Is(err, errStoredCheckpointNotFound):
		return http.StatusNotFound
	case errors.Is(err, errCheckpointStale), errors.Is(err, errCheckpointRunFinished):
		return http.StatusConflict
	case errors.Is(err, errCheckpointStorageUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, errCheckpointArtifactInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// controlPlaneCheckpointHTTPHandler replaces PokéWall's file-backed checkpoint
// route in PostgreSQL mode. Small JSON sidecars are stored inline in Postgres;
// binary/large payloads are written to S3 and only their references are stored.
func (w *Wall) controlPlaneCheckpointHTTPHandler(next http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil {
		return next
	}
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		id, ok := checkpointRequestRunID(req.URL.Path)
		if !ok || req.Method != http.MethodPost {
			next.ServeHTTP(res, req)
			return
		}
		req.Body = http.MaxBytesReader(res, req.Body, maxFinishBody)
		var incoming struct {
			farm.CheckpointReport
			Resume bool `json:"resume,omitempty"`
		}
		if err := json.NewDecoder(req.Body).Decode(&incoming); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "bad checkpoint: " + err.Error()})
			return
		}
		if incoming.RunID != "" && incoming.RunID != id {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "run_id mismatch: path " + id + " body " + incoming.RunID})
			return
		}
		incoming.RunID = id
		if incoming.Resume {
			w.handleControlPlaneCheckpointResume(res, incoming.CheckpointReport)
			return
		}
		if err := w.storeControlPlaneCheckpoint(incoming.CheckpointReport); err != nil {
			writeJSON(res, checkpointHTTPStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (w *Wall) storeControlPlaneCheckpoint(report farm.CheckpointReport) error {
	cp := controlPlaneFor(w)
	if cp == nil {
		return errors.New("PostgreSQL control plane is not configured")
	}
	if err := farm.ValidateCheckpointReport(report); err != nil {
		return fmt.Errorf("%w: %v", errCheckpointArtifactInvalid, err)
	}

	w.mu.Lock()
	t := w.tiles[report.RunID]
	if t == nil {
		w.mu.Unlock()
		return fmt.Errorf("%w: unknown run %s", errStoredCheckpointNotFound, report.RunID)
	}
	if t.Finished {
		w.mu.Unlock()
		return fmt.Errorf("%w: %s", errCheckpointRunFinished, report.RunID)
	}
	attempt := t.Attempts + 1
	if report.Attempt != 0 && report.Attempt != attempt {
		w.mu.Unlock()
		return fmt.Errorf("%w: run is on attempt %d, report claims %d", errCheckpointStale, attempt, report.Attempt)
	}
	w.mu.Unlock()

	store, configured, storeErr := artifactstore.S3FromEnv()
	if storeErr != nil {
		return fmt.Errorf("%w: checkpoint S3: %v", errCheckpointStorageUnavailable, storeErr)
	}
	type persisted struct {
		meta   farm.Artifact
		inline []byte
	}
	persistedArtifacts := make([]persisted, 0, len(report.Artifacts))
	uploaded := make([]string, 0, len(report.Artifacts))
	cleanupUploads := func() {
		if store == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, key := range uploaded {
			_ = store.DeleteObject(ctx, key)
		}
	}
	for _, art := range report.Artifacts {
		meta := art
		inline := []byte(nil)
		if art.Store == farm.ArtifactStoreS3 {
			meta.Data = nil
		} else if isSmallStructuredArtifact(art) {
			inline = append([]byte(nil), art.Data...)
			meta.Data = nil
		} else {
			if !configured || store == nil {
				cleanupUploads()
				return fmt.Errorf("%w: S3 is required for checkpoint artifact %s", errCheckpointStorageUnavailable, art.Name)
			}
			key := checkpointObjectKey(report.RunID, attempt, art.Name)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			obj, err := store.PutObject(ctx, key, art.MediaType, art.Data)
			cancel()
			if err != nil {
				cleanupUploads()
				return fmt.Errorf("upload checkpoint artifact %s: %w", art.Name, err)
			}
			uploaded = append(uploaded, key)
			meta = farm.Artifact{
				Name: art.Name, MediaType: art.MediaType, SHA256: obj.SHA256,
				Store: farm.ArtifactStoreS3, Bucket: obj.Bucket, ObjectKey: obj.Key, Size: obj.Size,
			}
		}
		persistedArtifacts = append(persistedArtifacts, persisted{meta: meta, inline: inline})
	}

	// Recheck the generation after potentially slow object uploads.
	w.mu.Lock()
	t = w.tiles[report.RunID]
	stillCurrent := t != nil && !t.Finished && t.Attempts+1 == attempt
	w.mu.Unlock()
	if !stillCurrent {
		cleanupUploads()
		return fmt.Errorf("%w: run is no longer on attempt %d", errCheckpointStale, attempt)
	}

	tx, err := cp.db.Begin()
	if err != nil {
		cleanupUploads()
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, p := range persistedArtifacts {
		metaRaw, _ := json.Marshal(p.meta)
		var inline any
		if p.inline != nil {
			inline = p.inline
		}
		if _, err := tx.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,media_type,sha256,store,bucket,object_key,size,metadata_json,inline_data) VALUES($1,$2,'checkpoint',$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11) ON CONFLICT(run_id,attempt,kind,name) DO UPDATE SET media_type=EXCLUDED.media_type,sha256=EXCLUDED.sha256,store=EXCLUDED.store,bucket=EXCLUDED.bucket,object_key=EXCLUDED.object_key,size=EXCLUDED.size,metadata_json=EXCLUDED.metadata_json,inline_data=EXCLUDED.inline_data`, report.RunID, attempt, p.meta.Name, p.meta.MediaType, p.meta.SHA256, p.meta.Store, p.meta.Bucket, p.meta.ObjectKey, checkpointArtifactSize(p.meta, p.inline), string(metaRaw), inline); err != nil {
			cleanupUploads()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		cleanupUploads()
		return err
	}
	cp.retainStoredCheckpointWindow(report.RunID, attempt, store)
	return nil
}

func isSmallStructuredArtifact(art farm.Artifact) bool {
	media := strings.ToLower(strings.TrimSpace(art.MediaType))
	return len(art.Data) <= maxPostgresInlineEvidence && (strings.Contains(media, "json") || strings.HasSuffix(strings.ToLower(art.Name), ".json"))
}

func checkpointArtifactSize(meta farm.Artifact, inline []byte) int64 {
	if len(inline) > 0 {
		return int64(len(inline))
	}
	return meta.Size
}

func checkpointObjectKey(runID string, attempt int, name string) string {
	prefix := safeBase(runID)
	if len(prefix) > 64 {
		prefix = prefix[:64]
	}
	sum := sha256.Sum256([]byte(runID))
	return fmt.Sprintf("runs/%s-%s/attempt-%d/checkpoints/%s", prefix, hex.EncodeToString(sum[:6]), attempt, name)
}

type storedCheckpointArtifact struct {
	meta      farm.Artifact
	inline    []byte
	hasInline bool
}

func (cp *controlPlane) storedCheckpointArtifacts(runID string, attempt int) (map[string]storedCheckpointArtifact, error) {
	rows, err := cp.db.Query(`SELECT metadata_json,inline_data,inline_data IS NOT NULL FROM artifacts WHERE run_id=$1 AND attempt=$2 AND kind='checkpoint'`, runID, attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]storedCheckpointArtifact{}
	for rows.Next() {
		var raw, inline []byte
		var hasInline bool
		if err := rows.Scan(&raw, &inline, &hasInline); err != nil {
			return nil, err
		}
		var meta farm.Artifact
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, err
		}
		out[meta.Name] = storedCheckpointArtifact{meta: meta, inline: append([]byte(nil), inline...), hasInline: hasInline}
	}
	return out, rows.Err()
}

func (cp *controlPlane) materializeStoredArtifact(stored storedCheckpointArtifact) (farm.Artifact, error) {
	if stored.hasInline {
		art := stored.meta
		art.Store, art.Bucket, art.ObjectKey, art.Size = "", "", "", 0
		art.Data = append([]byte(nil), stored.inline...)
		if err := farm.ValidateCheckpointState(art); err != nil {
			return farm.Artifact{}, err
		}
		return art, nil
	}
	if stored.meta.Store != farm.ArtifactStoreS3 {
		return farm.Artifact{}, fmt.Errorf("checkpoint artifact %s has neither inline data nor S3 reference", stored.meta.Name)
	}
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		return farm.Artifact{}, err
	}
	if !configured || store == nil {
		return farm.Artifact{}, errors.New("checkpoint S3 is not configured")
	}
	if stored.meta.Bucket != store.Bucket() {
		return farm.Artifact{}, fmt.Errorf("checkpoint bucket %q is not configured bucket %q", stored.meta.Bucket, store.Bucket())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	obj, err := store.GetObject(ctx, stored.meta.ObjectKey, "")
	if err != nil {
		cancel()
		return farm.Artifact{}, err
	}
	data, readErr := io.ReadAll(obj.Body)
	closeErr := obj.Body.Close()
	cancel()
	if readErr != nil {
		return farm.Artifact{}, readErr
	}
	if closeErr != nil {
		return farm.Artifact{}, closeErr
	}
	art := farm.Artifact{Name: stored.meta.Name, MediaType: stored.meta.MediaType, SHA256: stored.meta.SHA256, Data: data}
	// A stored state that is no longer a complete save (an older wall, or an
	// object truncated by a mid-write upload) must not be offered as a resume
	// checkpoint: the caller skips this candidate and tries an older pair, so
	// the run keeps its progress instead of being restarted from a cartridge.
	if err := farm.ValidateCheckpointState(art); err != nil {
		return farm.Artifact{}, fmt.Errorf("checkpoint object %s: %w", stored.meta.ObjectKey, err)
	}
	return art, nil
}

func (cp *controlPlane) latestStoredObjective(runID string, attempt int) (farm.ResumeCheckpoint, error) {
	arts, err := cp.storedCheckpointArtifacts(runID, attempt)
	if err != nil {
		return farm.ResumeCheckpoint{}, err
	}
	var states []string
	for name := range arts {
		if strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state") {
			states = append(states, name)
		}
	}
	sortByFrame(states)
	return cp.latestStoredPair(arts, states, attempt)
}

func (cp *controlPlane) latestStoredMajor(runID string, throughAttempt int) (farm.ResumeCheckpoint, error) {
	bestBadge := 0
	var best farm.ResumeCheckpoint
	found := false
	for attempt := throughAttempt; attempt >= 1; attempt-- {
		arts, err := cp.storedCheckpointArtifacts(runID, attempt)
		if err != nil {
			return farm.ResumeCheckpoint{}, err
		}
		var states []string
		for name := range arts {
			if strings.HasPrefix(name, majorCheckpointPrefix) && strings.HasSuffix(name, ".state") {
				states = append(states, name)
			}
		}
		sort.Strings(states)
		candidate, err := cp.latestStoredPair(arts, states, attempt)
		if err != nil {
			if errors.Is(err, errStoredCheckpointNotFound) {
				continue
			}
			return farm.ResumeCheckpoint{}, err
		}
		badge, ok := majorCheckpointBadge(candidate.State.Name)
		if ok && (!found || badge > bestBadge) {
			best, bestBadge, found = candidate, badge, true
		}
	}
	if !found {
		return farm.ResumeCheckpoint{}, errStoredCheckpointNotFound
	}
	return best, nil
}

func (cp *controlPlane) latestStoredPair(arts map[string]storedCheckpointArtifact, states []string, attempt int) (farm.ResumeCheckpoint, error) {
	for i := len(states) - 1; i >= 0; i-- {
		stateName := states[i]
		base := strings.TrimSuffix(stateName, ".state")
		knowledgeName := ""
		for name := range arts {
			if strings.HasPrefix(name, base+".knowledge-v") && strings.HasSuffix(name, ".json") && name > knowledgeName {
				knowledgeName = name
			}
		}
		if knowledgeName == "" {
			continue
		}
		stateArt, err := cp.materializeStoredArtifact(arts[stateName])
		if err != nil {
			continue
		}
		knowledgeArt, err := cp.materializeStoredArtifact(arts[knowledgeName])
		if err != nil {
			continue
		}
		return farm.ResumeCheckpoint{Attempt: attempt, State: stateArt, Knowledge: &knowledgeArt}, nil
	}
	return farm.ResumeCheckpoint{}, errStoredCheckpointNotFound
}

// latestStoredLineageObjective returns the deepest usable ordinary checkpoint
// in a run's lineage. The frame embedded in the checkpoint name is cumulative
// emulator progress, so it is the only comparable ordering across attempts:
// picking the newest attempt instead would let an attempt that booted fresh —
// and therefore stored only early checkpoints before dying — outrank the deep
// progress an earlier attempt actually reached. Attempts are still searched
// newest-first so an equally deep candidate prefers the most recent one.
func (w *Wall) latestStoredLineageObjective(startID string) (farm.ResumeCheckpoint, error) {
	cp := controlPlaneFor(w)
	seen := map[string]struct{}{}
	var best farm.ResumeCheckpoint
	bestFrame := uint64(0)
	found := false
	for id := startID; id != ""; {
		if _, duplicate := seen[id]; duplicate {
			break
		}
		seen[id] = struct{}{}
		w.mu.Lock()
		t := w.tiles[id]
		through, parent := 0, ""
		if t != nil {
			through, parent = t.Attempts, t.ResumeFromRunID
		}
		w.mu.Unlock()
		for attempt := through; attempt >= 1; attempt-- {
			candidate, err := cp.latestStoredObjective(id, attempt)
			if err == nil {
				if frame := objectiveFrame(candidate.State.Name); !found || frame > bestFrame {
					best, bestFrame, found = candidate, frame, true
				}
				continue
			}
			if !errors.Is(err, errStoredCheckpointNotFound) {
				return farm.ResumeCheckpoint{}, err
			}
		}
		id = parent
	}
	if !found {
		return farm.ResumeCheckpoint{}, errStoredCheckpointNotFound
	}
	return best, nil
}

func (w *Wall) latestStoredLineageMajor(startID string) (farm.ResumeCheckpoint, error) {
	cp := controlPlaneFor(w)
	seen := map[string]struct{}{}
	bestBadge := 0
	var best farm.ResumeCheckpoint
	found := false
	for id := startID; id != ""; {
		if _, duplicate := seen[id]; duplicate {
			break
		}
		seen[id] = struct{}{}
		w.mu.Lock()
		t := w.tiles[id]
		through, parent := 0, ""
		if t != nil {
			through, parent = t.Attempts, t.ResumeFromRunID
		}
		w.mu.Unlock()
		if through > 0 {
			candidate, err := cp.latestStoredMajor(id, through)
			if err == nil {
				badge, ok := majorCheckpointBadge(candidate.State.Name)
				if ok && (!found || badge > bestBadge) {
					best, bestBadge, found = candidate, badge, true
				}
			} else if !errors.Is(err, errStoredCheckpointNotFound) {
				return farm.ResumeCheckpoint{}, err
			}
		}
		id = parent
	}
	if !found {
		return farm.ResumeCheckpoint{}, errStoredCheckpointNotFound
	}
	return best, nil
}

func (w *Wall) handleControlPlaneCheckpointResume(res http.ResponseWriter, report farm.CheckpointReport) {
	cp := controlPlaneFor(w)
	w.mu.Lock()
	t := w.tiles[report.RunID]
	if t == nil {
		w.mu.Unlock()
		writeJSON(res, http.StatusNotFound, map[string]string{"error": "unknown run " + report.RunID})
		return
	}
	if t.Finished {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": "run already finished: " + report.RunID})
		return
	}
	attempt := t.Attempts + 1
	if report.Attempt != 0 && report.Attempt != attempt {
		w.mu.Unlock()
		writeJSON(res, http.StatusConflict, map[string]string{"error": fmt.Sprintf("stale resume: run is on attempt %d, request claims %d", attempt, report.Attempt)})
		return
	}
	previous := attempt - 1
	retryPrefix := fmt.Sprintf("attempt %d failed: ", previous)
	lostPrefix := retryPrefix + "no heartbeat for "
	planner := t.Planner
	lostRetry := previous > 0 && strings.HasPrefix(t.Detail, lostPrefix)
	endlessRetry := previous > 0 && t.Endless && planner == "llm" && strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	gymRetry := previous > 0 && !t.Endless && planner == "llm" && strings.HasPrefix(t.Detail, retryPrefix) && !lostRetry
	lineageRetry := previous == 0 && t.Endless && planner == "llm" && t.ResumeFromRunID != ""
	resumeParent := t.ResumeFromRunID
	w.mu.Unlock()

	var candidate farm.ResumeCheckpoint
	var err error
	switch {
	case lostRetry:
		candidate, err = cp.latestStoredObjective(report.RunID, previous)
		if errors.Is(err, errStoredCheckpointNotFound) && planner == "llm" {
			candidate, err = w.latestStoredLineageObjective(report.RunID)
			if errors.Is(err, errStoredCheckpointNotFound) {
				candidate, err = w.latestStoredLineageMajor(report.RunID)
			}
		}
	case endlessRetry:
		candidate, err = w.latestStoredLineageObjective(report.RunID)
		if errors.Is(err, errStoredCheckpointNotFound) {
			candidate, err = w.latestStoredLineageMajor(report.RunID)
		}
	case gymRetry:
		candidate, err = w.latestStoredLineageMajor(report.RunID)
	case lineageRetry:
		candidate, err = w.latestStoredLineageObjective(resumeParent)
		if errors.Is(err, errStoredCheckpointNotFound) {
			candidate, err = w.latestStoredLineageMajor(resumeParent)
		}
	default:
		res.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		if !errors.Is(err, errStoredCheckpointNotFound) {
			log.Printf("pokewall: %s attempt %d PostgreSQL/S3 resume checkpoint: %v", report.RunID, previous, err)
		}
		res.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(res, http.StatusOK, candidate)
}

func (cp *controlPlane) retainStoredCheckpointWindow(runID string, attempt int, store *artifactstore.S3) {
	arts, err := cp.storedCheckpointArtifacts(runID, attempt)
	if err != nil {
		log.Printf("pokewall: checkpoint retention list %s/%d: %v", runID, attempt, err)
		return
	}
	files := map[string]struct{}{}
	var periodic, objective, major []string
	for name := range arts {
		files[name] = struct{}{}
		switch {
		case strings.HasPrefix(name, "periodic-") && strings.HasSuffix(name, ".state"):
			periodic = append(periodic, name)
		case strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state"):
			objective = append(objective, name)
		case strings.HasPrefix(name, majorCheckpointPrefix) && strings.HasSuffix(name, ".state"):
			major = append(major, name)
		}
	}
	sort.Strings(periodic)
	sortByFrame(objective)
	sort.Strings(major)
	dropNames := map[string]struct{}{}
	knowledgeSidecar := func(name string) string {
		base := strings.TrimSuffix(name, ".state")
		for file := range files {
			if strings.HasPrefix(file, base+".knowledge-v") && strings.HasSuffix(file, ".json") {
				return file
			}
		}
		return ""
	}
	markDrop := func(names []string, keep int, sidecar func(string) string) {
		n := len(names) - keep
		if n <= 0 {
			return
		}
		for _, name := range names[:n] {
			dropNames[name] = struct{}{}
			if side := sidecar(name); side != "" {
				dropNames[side] = struct{}{}
			}
		}
	}
	markDrop(periodic, checkpointPeriodicKeep, func(name string) string { return strings.TrimSuffix(name, ".state") + ".json" })
	markDrop(objective, checkpointObjectiveKeep, knowledgeSidecar)
	markDrop(major, checkpointMajorKeep, knowledgeSidecar)
	for name := range dropNames {
		stored, ok := arts[name]
		if !ok {
			continue
		}
		if _, err := cp.db.Exec(`DELETE FROM artifacts WHERE run_id=$1 AND attempt=$2 AND kind='checkpoint' AND name=$3`, runID, attempt, name); err != nil {
			log.Printf("pokewall: checkpoint retention delete %s/%d/%s: %v", runID, attempt, name, err)
			continue
		}
		if store != nil && stored.meta.Store == farm.ArtifactStoreS3 && stored.meta.ObjectKey != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := store.DeleteObject(ctx, stored.meta.ObjectKey)
			cancel()
			if err != nil {
				log.Printf("pokewall: checkpoint S3 retention %s: %v", stored.meta.ObjectKey, err)
			}
		}
	}
}
