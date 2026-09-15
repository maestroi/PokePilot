package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/maestroi/pokepilot/farm"
)

const controlPlaneDriverName = "pokepilot-postgres"

// The existing runCatalog deliberately uses the database/sql question-mark
// dialect so its SQLite test backend stays tiny. Production PostgreSQL uses
// this wrapper to rebind those placeholders without duplicating the catalog
// query layer. LIMIT -1 is the other SQLite-ism used by the history pager.
type controlPlaneDriver struct{}
type controlPlaneConn struct{ driver.Conn }

func init() { sql.Register(controlPlaneDriverName, controlPlaneDriver{}) }

func (controlPlaneDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&pq.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &controlPlaneConn{Conn: conn}, nil
}

func (c *controlPlaneConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(rebindPostgresQuery(query))
}

func (c *controlPlaneConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	query = rebindPostgresQuery(query)
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func rebindPostgresQuery(query string) string {
	query = strings.ReplaceAll(query, "LIMIT -1", "LIMIT ALL")
	var out strings.Builder
	out.Grow(len(query) + 16)
	arg := 1
	for _, r := range query {
		if r == '?' {
			fmt.Fprintf(&out, "$%d", arg)
			arg++
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// Migration 1 is intentionally a clean-cutover schema. There is no importer
// for the old SQLite/state.json/model-experiments.json stores: production data
// was deliberately wiped before this migration.
const controlPlaneMigration001 = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS model_deployments (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL,
    revision TEXT NOT NULL DEFAULT '',
    artifact TEXT NOT NULL DEFAULT '',
    quantization TEXT NOT NULL DEFAULT '',
    compute TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    api_model TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    control_url TEXT NOT NULL DEFAULT '',
    token_env TEXT NOT NULL DEFAULT '',
    engine TEXT NOT NULL DEFAULT '',
    engine_version TEXT NOT NULL DEFAULT '',
    engine_config TEXT NOT NULL DEFAULT '',
    legacy_profile TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx
    ON model_deployments(enabled, compute, label, id);

CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    planner TEXT NOT NULL DEFAULT '',
    starter TEXT NOT NULL DEFAULT '',
    goal TEXT NOT NULL DEFAULT '',
    llm_profile TEXT NOT NULL DEFAULT '',
    queued_at BIGINT NOT NULL DEFAULT 0,
    ended_at BIGINT NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL DEFAULT '',
    how TEXT NOT NULL DEFAULT '',
    starter_facet TEXT NOT NULL DEFAULT '',
    row_json BYTEA NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_status_ended_idx ON runs(status, ended_at DESC, queued_at DESC);
CREATE INDEX IF NOT EXISTS runs_history_filter_idx ON runs(status, outcome, how, starter_facet);
CREATE INDEX IF NOT EXISTS runs_planner_time_idx ON runs(planner, ended_at DESC);

CREATE TABLE IF NOT EXISTS control_plane_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    state_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS run_attempts (
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    runner_version TEXT NOT NULL DEFAULT '',
    seed_burn INTEGER NOT NULL DEFAULT 0,
    report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    finished_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt)
);
CREATE INDEX IF NOT EXISTS run_attempts_finished_idx ON run_attempts(finished_at DESC);
CREATE INDEX IF NOT EXISTS run_attempts_reason_idx ON run_attempts(reason, finished_at DESC);

CREATE TABLE IF NOT EXISTS experiments (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    request_json JSONB NOT NULL,
    record_json JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS experiments_created_idx ON experiments(created_at DESC);

CREATE TABLE IF NOT EXISTS experiment_runs (
    run_id TEXT PRIMARY KEY,
    experiment_id TEXT NOT NULL DEFAULT '',
    experiment_arm TEXT NOT NULL DEFAULT '',
    experiment_case TEXT NOT NULL DEFAULT '',
    deployment_id TEXT NOT NULL DEFAULT '',
    comparable_hash TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS experiment_runs_pair_idx ON experiment_runs(experiment_id, experiment_case, experiment_arm);
CREATE INDEX IF NOT EXISTS experiment_runs_deployment_idx ON experiment_runs(deployment_id, experiment_id);

CREATE TABLE IF NOT EXISTS model_hosts (
    host_id TEXT PRIMARY KEY,
    compute TEXT NOT NULL DEFAULT '',
    control_url TEXT NOT NULL DEFAULT '',
    deployment_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    health TEXT NOT NULL DEFAULT '',
    active_leases INTEGER NOT NULL DEFAULT 0,
    max_parallel_workers INTEGER NOT NULL DEFAULT 0,
    status_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS model_hosts_state_idx ON model_hosts(state, compute);

CREATE TABLE IF NOT EXISTS llm_exchanges (
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    exchange_index INTEGER NOT NULL,
    observation JSONB,
    offered JSONB NOT NULL DEFAULT '[]'::jsonb,
    replan_reason TEXT NOT NULL DEFAULT '',
    plan_goal TEXT NOT NULL DEFAULT '',
    plan_steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    rejected BOOLEAN NOT NULL DEFAULT FALSE,
    error TEXT NOT NULL DEFAULT '',
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    backend TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    prefill_tps DOUBLE PRECISION NOT NULL DEFAULT 0,
    decode_tps DOUBLE PRECISION NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, exchange_index)
);
CREATE INDEX IF NOT EXISTS llm_exchanges_model_idx ON llm_exchanges(model, recorded_at DESC);
CREATE INDEX IF NOT EXISTS llm_exchanges_run_idx ON llm_exchanges(run_id, attempt);

CREATE TABLE IF NOT EXISTS objective_failures (
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    failure_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    blocking BOOLEAN NOT NULL DEFAULT FALSE,
    terminal_count INTEGER NOT NULL DEFAULT 0,
    failure_json JSONB NOT NULL,
    report_json JSONB NOT NULL,
    delivery_status TEXT NOT NULL DEFAULT 'pending',
    delivery_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, failure_key)
);
CREATE INDEX IF NOT EXISTS objective_failures_fingerprint_idx ON objective_failures(fingerprint, updated_at DESC);
CREATE INDEX IF NOT EXISTS objective_failures_delivery_idx ON objective_failures(delivery_status, updated_at);

CREATE TABLE IF NOT EXISTS issue_fingerprints (
    failure_key TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_fingerprints_fingerprint_idx ON issue_fingerprints(fingerprint);

CREATE TABLE IF NOT EXISTS issue_occurrences (
    external_id TEXT PRIMARY KEY,
    failure_key TEXT NOT NULL,
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_occurrences_key_idx ON issue_occurrences(failure_key, created_at DESC);
CREATE INDEX IF NOT EXISTS issue_occurrences_run_idx ON issue_occurrences(run_id, attempt);

CREATE TABLE IF NOT EXISTS issue_links (
    failure_key TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_links_status_idx ON issue_links(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS issue_outbox (
    external_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    failure_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    next_attempt BIGINT NOT NULL DEFAULT 0,
    payload_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_outbox_pending_idx ON issue_outbox(status, next_attempt, updated_at);

CREATE TABLE IF NOT EXISTS artifacts (
    run_id TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    store TEXT NOT NULL DEFAULT '',
    bucket TEXT NOT NULL DEFAULT '',
    object_key TEXT NOT NULL DEFAULT '',
    size BIGINT NOT NULL DEFAULT 0,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, kind, name)
);
CREATE INDEX IF NOT EXISTS artifacts_run_idx ON artifacts(run_id, attempt, kind);
CREATE INDEX IF NOT EXISTS artifacts_object_idx ON artifacts(store, bucket, object_key);

CREATE TABLE IF NOT EXISTS dataset_manifests (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL DEFAULT '',
    query_json JSONB NOT NULL,
    manifest_json JSONB NOT NULL,
    object_key TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS dataset_manifests_created_idx ON dataset_manifests(created_at DESC);
`

type controlPlane struct {
	db *sql.DB
}

var wallControlPlanes sync.Map // *Wall -> *controlPlane
var wallExperimentControllers sync.Map // *Wall -> *modelExperimentController

func controlPlaneFor(w *Wall) *controlPlane {
	if v, ok := wallControlPlanes.Load(w); ok {
		return v.(*controlPlane)
	}
	return nil
}

func (w *Wall) SetControlPlaneDatabase(dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return errors.New("empty PostgreSQL DSN")
	}
	if old := controlPlaneFor(w); old != nil {
		_ = w.CloseControlPlane()
	}
	db, err := sql.Open(controlPlaneDriverName, dsn)
	if err != nil {
		return fmt.Errorf("open control plane: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	var pingErr error
	for attempt := 0; attempt < 20; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		pingErr = db.PingContext(ctx)
		cancel()
		if pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		_ = db.Close()
		return fmt.Errorf("connect control plane: %w", pingErr)
	}
	cp := &controlPlane{db: db}
	if err := cp.migrate(); err != nil {
		_ = db.Close()
		return err
	}
	wallControlPlanes.Store(w, cp)
	wallCatalogs.Store(w, &runCatalog{db: db})
	if err := cp.restoreWall(w); err != nil {
		wallControlPlanes.Delete(w)
		wallCatalogs.Delete(w)
		_ = db.Close()
		return err
	}
	if err := w.syncCatalogFromRAM(false); err != nil {
		return fmt.Errorf("sync active runs to control plane: %w", err)
	}
	w.evictCatalogFinished()
	return nil
}

func (w *Wall) CloseControlPlane() error {
	cp := controlPlaneFor(w)
	if cp == nil {
		return nil
	}
	wallExperimentControllers.Delete(w)
	wallControlPlanes.Delete(w)
	wallCatalogs.Delete(w)
	return cp.db.Close()
}

func (cp *controlPlane) migrate() error {
	tx, err := cp.db.Begin()
	if err != nil {
		return fmt.Errorf("begin control-plane migration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var applied bool
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=1)`).Scan(&applied); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if !applied {
		if _, err := tx.Exec(controlPlaneMigration001); err != nil {
			return fmt.Errorf("apply control-plane migration 1: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(1) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 1: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit control-plane migration: %w", err)
	}
	return nil
}

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
	w.cancel = make(map[string]struct{})
	for _, id := range ps.Order {
		pt, ok := ps.Tiles[id]
		if !ok {
			continue
		}
		w.order = append(w.order, id)
		w.tiles[id] = &Tile{
			RunID: pt.RunID, Status: pt.Status, Planner: pt.Planner, Starter: pt.Starter,
			Dest: pt.Dest, Goal: pt.Goal, LLMProfile: pt.LLMProfile, ReasoningEffort: pt.ReasoningEffort,
			Seed: pt.Seed, FPS: pt.FPS, MaxRounds: pt.MaxRounds, MaxFrames: pt.MaxFrames,
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
	// Issue protocol state is normalized below; do not duplicate it inside the
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
	for key := range links { keys = append(keys, key) }
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
	for id := range outbox { ids = append(ids, id) }
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

// RunControlPlaneSweep is the durability bridge for high-frequency mutations
// that intentionally stay off the synchronous HTTP path (heartbeats and issue
// status refreshes). Lifecycle requests are persisted synchronously by the
// middleware below; this sweep is only the safety net for live telemetry.
func (w *Wall) RunControlPlaneSweep(interval time.Duration) {
	cp := controlPlaneFor(w)
	if cp == nil { return }
	if interval <= 0 { interval = 2 * time.Second }
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

func sanitizeFinishReport(report farm.FinishReport) farm.FinishReport {
	// PostgreSQL keeps searchable metadata and small structured evidence, never
	// emulator states/frames or other large payload bytes.
	report.SaveState = nil
	report.FramePNG = nil
	for i := range report.Artifacts {
		report.Artifacts[i].Data = nil
	}
	return report
}

func (cp *controlPlane) persistFinish(w *Wall, report farm.FinishReport) error {
	attempt := report.Attempt
	if attempt <= 0 {
		w.mu.Lock()
		if t := w.tiles[report.RunID]; t != nil { attempt = t.Attempts }
		w.mu.Unlock()
		if attempt <= 0 { attempt = 1 }
	}
	safe := sanitizeFinishReport(report)
	raw, err := json.Marshal(safe)
	if err != nil { return err }
	tx, err := cp.db.Begin()
	if err != nil { return err }
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`INSERT INTO run_attempts(run_id,attempt,reason,detail,runner_version,seed_burn,report_json,finished_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NOW()) ON CONFLICT(run_id,attempt) DO UPDATE SET reason=EXCLUDED.reason,detail=EXCLUDED.detail,runner_version=EXCLUDED.runner_version,seed_burn=EXCLUDED.seed_burn,report_json=EXCLUDED.report_json,finished_at=NOW()`, report.RunID, attempt, report.Reason, report.Detail, report.RunnerVersion, report.SeedBurn, string(raw)); err != nil {
		return fmt.Errorf("persist run attempt: %w", err)
	}
	for _, art := range report.Artifacts {
		meta := art
		meta.Data = nil
		metaRaw, _ := json.Marshal(meta)
		if _, err := tx.Exec(`INSERT INTO artifacts(run_id,attempt,kind,name,media_type,sha256,store,bucket,object_key,size,metadata_json) VALUES($1,$2,'finish',$3,$4,$5,$6,$7,$8,$9,$10::jsonb) ON CONFLICT(run_id,attempt,kind,name) DO UPDATE SET media_type=EXCLUDED.media_type,sha256=EXCLUDED.sha256,store=EXCLUDED.store,bucket=EXCLUDED.bucket,object_key=EXCLUDED.object_key,size=EXCLUDED.size,metadata_json=EXCLUDED.metadata_json`, report.RunID, attempt, art.Name, art.MediaType, art.SHA256, art.Store, art.Bucket, art.ObjectKey, art.Size, string(metaRaw)); err != nil {
			return fmt.Errorf("persist artifact %s: %w", art.Name, err)
		}
	}
	if err := cp.persistObjectiveFailuresTx(tx, report, attempt); err != nil { return err }
	if err := cp.persistStrategicRecordsTx(tx, w, report.RunID, attempt); err != nil { return err }
	return tx.Commit()
}

func (cp *controlPlane) persistStrategicRecordsTx(tx *sql.Tx, w *Wall, runID string, attempt int) error {
	row, ok := w.ramRow(runID)
	if !ok {
		if catalog := catalogFor(w); catalog != nil {
			var err error
			row, ok, err = catalog.get(runID)
			if err != nil { return err }
		}
	}
	if !ok || row.Stats == nil { return nil }
	for i, rec := range row.Stats.StrategicRecords {
		offered, _ := json.Marshal(rec.Offered)
		steps, _ := json.Marshal(rec.PlanSteps)
		var observation any = nil
		if len(rec.Observation) > 0 && json.Valid(rec.Observation) { observation = string(rec.Observation) }
		q := `INSERT INTO llm_exchanges(run_id,attempt,exchange_index,observation,offered,replan_reason,plan_goal,plan_steps,rejected,error,duration_seconds,backend,model,prompt_tokens,completion_tokens,prefill_tps,decode_tps) VALUES($1,$2,$3,CASE WHEN $4::text='' THEN NULL ELSE $4::jsonb END,$5::jsonb,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(run_id,attempt,exchange_index) DO UPDATE SET observation=EXCLUDED.observation,offered=EXCLUDED.offered,replan_reason=EXCLUDED.replan_reason,plan_goal=EXCLUDED.plan_goal,plan_steps=EXCLUDED.plan_steps,rejected=EXCLUDED.rejected,error=EXCLUDED.error,duration_seconds=EXCLUDED.duration_seconds,backend=EXCLUDED.backend,model=EXCLUDED.model,prompt_tokens=EXCLUDED.prompt_tokens,completion_tokens=EXCLUDED.completion_tokens,prefill_tps=EXCLUDED.prefill_tps,decode_tps=EXCLUDED.decode_tps`
		obs := ""
		if observation != nil { obs = observation.(string) }
		if _, err := tx.Exec(q, runID, attempt, i, obs, string(offered), rec.ReplanReason, rec.PlanGoal, string(steps), rec.Rejected, rec.Error, rec.DurationSeconds, rec.Backend, rec.Model, rec.PromptTokens, rec.CompletionTokens, rec.PrefillTPS, rec.DecodeTPS); err != nil {
			return fmt.Errorf("persist LLM exchange %d: %w", i, err)
		}
	}
	return nil
}

func (cp *controlPlane) persistObjectiveFailuresTx(tx *sql.Tx, report farm.FinishReport, attempt int) error {
	failures, err := farm.DecodeObjectiveFailures(report)
	if err != nil { return err }
	if synthetic, ok := terminalRunFailure(report, failures); ok { failures = append(failures, synthetic) }
	safeReport := sanitizeFinishReport(report)
	reportRaw, _ := json.Marshal(safeReport)
	for _, failure := range failures {
		key, fp, _, err := objectiveFailureFingerprint(failure)
		if err != nil { return err }
		failureRaw, _ := json.Marshal(failure)
		ext := objectiveFailureExternalID(report.RunID, attempt, key)
		if _, err := tx.Exec(`INSERT INTO objective_failures(run_id,attempt,failure_key,fingerprint,blocking,terminal_count,failure_json,report_json,delivery_status,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,'pending',NOW()) ON CONFLICT(run_id,attempt,failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,blocking=EXCLUDED.blocking,terminal_count=EXCLUDED.terminal_count,failure_json=EXCLUDED.failure_json,report_json=EXCLUDED.report_json,updated_at=NOW()`, report.RunID, attempt, key, fp, failure.Blocking, failure.TerminalCount, string(failureRaw), string(reportRaw)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO issue_fingerprints(failure_key,fingerprint,payload_json,updated_at) VALUES($1,$2,$3::jsonb,NOW()) ON CONFLICT(failure_key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,payload_json=EXCLUDED.payload_json,updated_at=NOW()`, key, fp, string(failureRaw)); err != nil { return err }
		occ := map[string]any{"failure": failure, "fingerprint": fp}
		occRaw, _ := json.Marshal(occ)
		if _, err := tx.Exec(`INSERT INTO issue_occurrences(external_id,failure_key,run_id,attempt,payload_json) VALUES($1,$2,$3,$4,$5::jsonb) ON CONFLICT(external_id) DO UPDATE SET payload_json=EXCLUDED.payload_json`, ext, key, report.RunID, attempt, string(occRaw)); err != nil { return err }
	}
	return nil
}

// controlPlaneHTTPHandler makes successful lifecycle mutations durable before
// their HTTP acknowledgement is released to the caller. Heartbeats deliberately
// stay on the periodic sweep so the database is not in the frame/telemetry hot
// path.
func (w *Wall) controlPlaneHTTPHandler(next http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil { return next }
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet || req.URL.Path == "/v1/workers" || strings.HasSuffix(req.URL.Path, "/heartbeat") {
			next.ServeHTTP(res, req)
			return
		}
		var finish *farm.FinishReport
		if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/finish") {
			data, err := io.ReadAll(io.LimitReader(req.Body, maxFinishBody+1))
			if err != nil { writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()}); return }
			if len(data) > maxFinishBody { writeJSON(res, http.StatusRequestEntityTooLarge, map[string]string{"error": "finish report too large"}); return }
			var parsed farm.FinishReport
			if json.Unmarshal(data, &parsed) == nil { finish = &parsed }
			req.Body = io.NopCloser(bytes.NewReader(data))
			req.ContentLength = int64(len(data))
		}
		buffered := newCatalogBufferedWriter()
		next.ServeHTTP(buffered, req)
		if buffered.status >= 200 && buffered.status < 300 {
			if finish != nil {
				if err := cp.persistFinish(w, *finish); err != nil {
					writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist finish metadata: " + err.Error()})
					return
				}
			}
			if err := cp.persistWall(w); err != nil {
				writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist control plane: " + err.Error()})
				return
			}
			if err := cp.persistExperimentController(w); err != nil {
				writeJSON(res, http.StatusInternalServerError, map[string]string{"error": "persist experiments: " + err.Error()})
				return
			}
		}
		buffered.flush(res)
	})
}

func controlPlaneModelExperimentHTTPHandler(w *Wall, fallback http.Handler) http.Handler {
	cp := controlPlaneFor(w)
	if cp == nil { return modelExperimentHTTPHandler(w, fallback) }
	controller := &modelExperimentController{
		wall: w, fallback: fallback,
		state: modelExperimentState{Runs: map[string]runExperimentMeta{}, Experiments: map[string]experimentRecord{}},
		client: &http.Client{Timeout: 2 * time.Second},
	}
	if err := cp.loadExperimentState(controller); err != nil {
		log.Printf("pokewall: load experiments from PostgreSQL: %v", err)
	}
	if source := strings.TrimSpace(os.Getenv("POKEPILOT_MODEL_REGISTRY")); source != "" {
		registry, err := farm.LoadModelRegistry(source)
		if err != nil { logModelExperiment("model registry %s: %v", source, err) } else { controller.registry = registry }
	}
	wallExperimentControllers.Store(w, controller)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", controller.handleModels)
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
	if err != nil { return err }
	for runs.Next() {
		var id string
		var raw []byte
		if err := runs.Scan(&id, &raw); err != nil { runs.Close(); return err }
		var meta runExperimentMeta
		if err := json.Unmarshal(raw, &meta); err != nil { runs.Close(); return err }
		c.state.Runs[id] = meta
	}
	if err := runs.Close(); err != nil { return err }
	exps, err := cp.db.Query(`SELECT id,record_json FROM experiments`)
	if err != nil { return err }
	defer exps.Close()
	for exps.Next() {
		var id string
		var raw []byte
		if err := exps.Scan(&id, &raw); err != nil { return err }
		var record experimentRecord
		if err := json.Unmarshal(raw, &record); err != nil { return err }
		c.state.Experiments[id] = record
	}
	return exps.Err()
}

func (cp *controlPlane) persistExperimentController(w *Wall) error {
	v, ok := wallExperimentControllers.Load(w)
	if !ok { return nil }
	c := v.(*modelExperimentController)
	c.mu.Lock()
	state := modelExperimentState{Runs: make(map[string]runExperimentMeta, len(c.state.Runs)), Experiments: make(map[string]experimentRecord, len(c.state.Experiments))}
	for id, meta := range c.state.Runs { state.Runs[id] = meta }
	for id, rec := range c.state.Experiments { state.Experiments[id] = rec }
	c.mu.Unlock()
	tx, err := cp.db.Begin()
	if err != nil { return err }
	defer tx.Rollback() //nolint:errcheck
	for id, meta := range state.Runs {
		raw, _ := json.Marshal(meta)
		if _, err := tx.Exec(`INSERT INTO experiment_runs(run_id,experiment_id,experiment_arm,experiment_case,deployment_id,comparable_hash,metadata_json) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(run_id) DO UPDATE SET experiment_id=EXCLUDED.experiment_id,experiment_arm=EXCLUDED.experiment_arm,experiment_case=EXCLUDED.experiment_case,deployment_id=EXCLUDED.deployment_id,comparable_hash=EXCLUDED.comparable_hash,metadata_json=EXCLUDED.metadata_json`, id, meta.ExperimentID, meta.ExperimentArm, meta.ExperimentCase, meta.Deployment, meta.ComparableHash, string(raw)); err != nil { return err }
	}
	for id, record := range state.Experiments {
		recordRaw, _ := json.Marshal(record)
		requestRaw, _ := json.Marshal(record.Request)
		if _, err := tx.Exec(`INSERT INTO experiments(id,name,created_at,request_json,record_json) VALUES($1,$2,$3,$4::jsonb,$5::jsonb) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,request_json=EXCLUDED.request_json,record_json=EXCLUDED.record_json`, id, record.Name, record.CreatedAt, string(requestRaw), string(recordRaw)); err != nil { return err }
	}
	return tx.Commit()
}

// RunControlPlaneObjectiveFailures retries durable structured failure reports
// from PostgreSQL. It deliberately performs no dump-directory discovery.
func (w *Wall) RunControlPlaneObjectiveFailures(every time.Duration) {
	cp := controlPlaneFor(w)
	if cp == nil || w.issueClient() == nil { return }
	if every <= 0 { every = defaultObjectiveFailureReportEvery }
	for {
		rows, err := cp.pendingObjectiveFailures(32)
		if err != nil {
			log.Printf("pokewall: load objective failure outbox: %v", err)
		} else {
			for _, item := range rows { cp.deliverObjectiveFailure(w, item) }
		}
		time.Sleep(every)
	}
}

type pendingObjectiveFailure struct {
	runID, key string
	attempt int
	failure farm.ObjectiveFailure
	report farm.FinishReport
}

func (cp *controlPlane) pendingObjectiveFailures(limit int) ([]pendingObjectiveFailure, error) {
	rows, err := cp.db.Query(`SELECT run_id,attempt,failure_key,failure_json,report_json FROM objective_failures WHERE delivery_status='pending' ORDER BY updated_at LIMIT $1`, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []pendingObjectiveFailure
	for rows.Next() {
		var item pendingObjectiveFailure
		var failureRaw, reportRaw []byte
		if err := rows.Scan(&item.runID, &item.attempt, &item.key, &failureRaw, &reportRaw); err != nil { return nil, err }
		if err := json.Unmarshal(failureRaw, &item.failure); err != nil { return nil, err }
		if err := json.Unmarshal(reportRaw, &item.report); err != nil { return nil, err }
		out = append(out, item)
	}
	return out, rows.Err()
}

func (cp *controlPlane) deliverObjectiveFailure(w *Wall, item pendingObjectiveFailure) {
	err := w.reportObjectiveFailure(item.report, item.failure)
	status := "complete"
	errText := ""
	if err != nil {
		errText = err.Error()
		if isRetryableIssueError(err) { status = "pending" } else { status = "error" }
	}
	if _, dbErr := cp.db.Exec(`UPDATE objective_failures SET delivery_status=$1,delivery_error=$2,updated_at=NOW() WHERE run_id=$3 AND attempt=$4 AND failure_key=$5`, status, errText, item.runID, item.attempt, item.key); dbErr != nil {
		log.Printf("pokewall: update objective failure delivery: %v", dbErr)
	}
	if err != nil { log.Printf("pokewall: objective failure %s/%d/%s: %v", item.runID, item.attempt, item.key, err) }
}
