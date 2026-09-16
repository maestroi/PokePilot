package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

func controlPlaneModelExperimentHTTPHandler(w *Wall, fallback http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil {
		return modelExperimentHTTPHandler(w, fallback)
	}
	controller := &modelExperimentController{
		wall:     w,
		fallback: fallback,
		state: modelExperimentState{
			Runs:        map[string]runExperimentMeta{},
			Experiments: map[string]experimentRecord{},
		},
		client: &http.Client{Timeout: 2 * time.Second},
	}
	if source := strings.TrimSpace(os.Getenv("POKEPILOT_MODEL_REGISTRY")); source != "" {
		controller.registrySource = source
		registry, err := farm.LoadModelRegistry(source)
		if err != nil {
			logModelExperiment("model registry %s: %v", source, err)
		} else {
			controller.registry = registry
		}
	}
	if err := cp.loadExperimentState(controller); err != nil {
		log.Printf("pokewall: load experiments from PostgreSQL: %v", err)
	}
	controller.backfillRunsFromExperiments()
	controller.attachDeploymentsToTiles()
	wallExperimentControllers.Store(w, controller)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", controller.handleModels)
	mux.HandleFunc("PATCH /v1/models/{id}", controller.handlePatchModel)
	mux.HandleFunc("POST /v1/experiments", controller.handleCreateExperiment)
	mux.HandleFunc("GET /v1/experiments", controller.handleExperiments)
	mux.HandleFunc("GET /v1/experiments/{id}", controller.handleExperiment)
	mux.HandleFunc("POST /v1/specs", controller.handleSpec)
	mux.HandleFunc("POST /v1/lease", controller.handleLease)
	mux.HandleFunc("POST /v1/runs/{id}/finish", controller.handleFinish)
	mux.HandleFunc("GET /v1/dashboard", controller.handleDashboard)
	mux.Handle("/", fallback)
	return mux
}

func (cp *controlPlane) loadExperimentState(c *modelExperimentController) error {
	runs, err := cp.db.Query(`SELECT run_id,metadata_json FROM experiment_runs`)
	if err != nil {
		return err
	}
	for runs.Next() {
		var id string
		var raw []byte
		if err := runs.Scan(&id, &raw); err != nil {
			runs.Close()
			return err
		}
		var meta runExperimentMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			runs.Close()
			return err
		}
		c.state.Runs[id] = meta
	}
	if err := runs.Close(); err != nil {
		return err
	}

	exps, err := cp.db.Query(`SELECT id,record_json FROM experiments`)
	if err != nil {
		return err
	}
	defer exps.Close()
	for exps.Next() {
		var id string
		var raw []byte
		if err := exps.Scan(&id, &raw); err != nil {
			return err
		}
		var record experimentRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return err
		}
		c.state.Experiments[id] = record
	}
	return exps.Err()
}

func experimentPersistWriteOrder(state modelExperimentState) (runIDs, experimentIDs []string) {
	return sortedMapKeys(state.Runs), sortedMapKeys(state.Experiments)
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (cp *controlPlane) persistExperimentController(w *Wall) error {
	v, ok := wallExperimentControllers.Load(w)
	if !ok {
		return nil
	}
	c := v.(*modelExperimentController)
	c.mu.Lock()
	state := modelExperimentState{
		Runs:        make(map[string]runExperimentMeta, len(c.state.Runs)),
		Experiments: make(map[string]experimentRecord, len(c.state.Experiments)),
	}
	for id, meta := range c.state.Runs {
		state.Runs[id] = meta
	}
	for id, record := range c.state.Experiments {
		state.Experiments[id] = record
	}
	c.mu.Unlock()

	// HTTP mutations can persist the same controller state repeatedly. Stable
	// lock ordering avoids deadlocks, while the DISTINCT predicates below keep
	// unchanged rows from creating new MVCC/TOAST versions.
	cp.experimentPersist.Lock()
	defer cp.experimentPersist.Unlock()
	tx, err := cp.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	runIDs, experimentIDs := experimentPersistWriteOrder(state)
	for _, id := range runIDs {
		meta := state.Runs[id]
		raw, _ := json.Marshal(meta)
		if _, err := tx.Exec(`INSERT INTO experiment_runs(run_id,experiment_id,experiment_arm,experiment_case,deployment_id,comparable_hash,metadata_json) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(run_id) DO UPDATE SET experiment_id=EXCLUDED.experiment_id,experiment_arm=EXCLUDED.experiment_arm,experiment_case=EXCLUDED.experiment_case,deployment_id=EXCLUDED.deployment_id,comparable_hash=EXCLUDED.comparable_hash,metadata_json=EXCLUDED.metadata_json WHERE experiment_runs.experiment_id IS DISTINCT FROM EXCLUDED.experiment_id OR experiment_runs.experiment_arm IS DISTINCT FROM EXCLUDED.experiment_arm OR experiment_runs.experiment_case IS DISTINCT FROM EXCLUDED.experiment_case OR experiment_runs.deployment_id IS DISTINCT FROM EXCLUDED.deployment_id OR experiment_runs.comparable_hash IS DISTINCT FROM EXCLUDED.comparable_hash OR experiment_runs.metadata_json IS DISTINCT FROM EXCLUDED.metadata_json`, id, meta.ExperimentID, meta.ExperimentArm, meta.ExperimentCase, meta.Deployment, meta.ComparableHash, string(raw)); err != nil {
			return err
		}
	}
	for _, id := range experimentIDs {
		record := state.Experiments[id]
		recordRaw, _ := json.Marshal(record)
		requestRaw, _ := json.Marshal(record.Request)
		if _, err := tx.Exec(`INSERT INTO experiments(id,name,created_at,request_json,record_json) VALUES($1,$2,$3,$4::jsonb,$5::jsonb) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,request_json=EXCLUDED.request_json,record_json=EXCLUDED.record_json WHERE experiments.name IS DISTINCT FROM EXCLUDED.name OR experiments.request_json IS DISTINCT FROM EXCLUDED.request_json OR experiments.record_json IS DISTINCT FROM EXCLUDED.record_json`, id, record.Name, record.CreatedAt, string(requestRaw), string(recordRaw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
