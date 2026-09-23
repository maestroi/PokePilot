package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

const controlPlaneDriverName = "pokepilot-postgres"

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

// Migration 1 is a clean-cutover schema. Existing production state was
// intentionally wiped, so there is no SQLite/state.json/experiment importer.
const controlPlaneMigration001 = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS model_deployments (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL DEFAULT '', model_id TEXT NOT NULL,
    revision TEXT NOT NULL DEFAULT '', artifact TEXT NOT NULL DEFAULT '',
    quantization TEXT NOT NULL DEFAULT '', compute TEXT NOT NULL,
    endpoint TEXT NOT NULL, api_model TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE, control_url TEXT NOT NULL DEFAULT '',
    token_env TEXT NOT NULL DEFAULT '', engine TEXT NOT NULL DEFAULT '',
    engine_version TEXT NOT NULL DEFAULT '', engine_config TEXT NOT NULL DEFAULT '',
    legacy_profile TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx ON model_deployments(enabled, compute, label, id);
CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY, status TEXT NOT NULL,
    planner TEXT NOT NULL DEFAULT '', starter TEXT NOT NULL DEFAULT '', goal TEXT NOT NULL DEFAULT '',
    llm_profile TEXT NOT NULL DEFAULT '', queued_at BIGINT NOT NULL DEFAULT 0, ended_at BIGINT NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL DEFAULT '', how TEXT NOT NULL DEFAULT '', starter_facet TEXT NOT NULL DEFAULT '',
    row_json BYTEA NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_status_ended_idx ON runs(status, ended_at DESC, queued_at DESC);
CREATE INDEX IF NOT EXISTS runs_history_filter_idx ON runs(status, outcome, how, starter_facet);
CREATE INDEX IF NOT EXISTS runs_planner_time_idx ON runs(planner, ended_at DESC);
CREATE TABLE IF NOT EXISTS control_plane_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1), state_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS run_attempts (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL,
    reason TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '', runner_version TEXT NOT NULL DEFAULT '',
    seed_burn INTEGER NOT NULL DEFAULT 0, report_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    finished_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY (run_id, attempt)
);
CREATE INDEX IF NOT EXISTS run_attempts_finished_idx ON run_attempts(finished_at DESC);
CREATE INDEX IF NOT EXISTS run_attempts_reason_idx ON run_attempts(reason, finished_at DESC);
CREATE TABLE IF NOT EXISTS experiments (
    id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL,
    request_json JSONB NOT NULL, record_json JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS experiments_created_idx ON experiments(created_at DESC);
CREATE TABLE IF NOT EXISTS experiment_runs (
    run_id TEXT PRIMARY KEY, experiment_id TEXT NOT NULL DEFAULT '', experiment_arm TEXT NOT NULL DEFAULT '',
    experiment_case TEXT NOT NULL DEFAULT '', deployment_id TEXT NOT NULL DEFAULT '', comparable_hash TEXT NOT NULL DEFAULT '',
    metadata_json JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS experiment_runs_pair_idx ON experiment_runs(experiment_id, experiment_case, experiment_arm);
CREATE INDEX IF NOT EXISTS experiment_runs_deployment_idx ON experiment_runs(deployment_id, experiment_id);
CREATE TABLE IF NOT EXISTS model_hosts (
    host_id TEXT PRIMARY KEY, compute TEXT NOT NULL DEFAULT '', control_url TEXT NOT NULL DEFAULT '',
    deployment_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT '', health TEXT NOT NULL DEFAULT '',
    active_leases INTEGER NOT NULL DEFAULT 0, max_parallel_workers INTEGER NOT NULL DEFAULT 0,
    status_json JSONB NOT NULL DEFAULT '{}'::jsonb, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS model_hosts_state_idx ON model_hosts(state, compute);
CREATE TABLE IF NOT EXISTS llm_exchanges (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL, exchange_index INTEGER NOT NULL,
    observation JSONB, offered JSONB NOT NULL DEFAULT '[]'::jsonb, replan_reason TEXT NOT NULL DEFAULT '',
    plan_goal TEXT NOT NULL DEFAULT '', plan_steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    rejected BOOLEAN NOT NULL DEFAULT FALSE, error TEXT NOT NULL DEFAULT '', duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    backend TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0, prefill_tps DOUBLE PRECISION NOT NULL DEFAULT 0,
    decode_tps DOUBLE PRECISION NOT NULL DEFAULT 0, recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, exchange_index)
);
CREATE INDEX IF NOT EXISTS llm_exchanges_model_idx ON llm_exchanges(model, recorded_at DESC);
CREATE INDEX IF NOT EXISTS llm_exchanges_run_idx ON llm_exchanges(run_id, attempt);
CREATE TABLE IF NOT EXISTS decision_exchanges (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL, decision_index INTEGER NOT NULL,
    kind TEXT NOT NULL DEFAULT '', question TEXT NOT NULL DEFAULT '', choice TEXT NOT NULL DEFAULT '',
    probabilities JSONB NOT NULL DEFAULT '{}'::jsonb, confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    fallback BOOLEAN NOT NULL DEFAULT FALSE, error TEXT NOT NULL DEFAULT '',
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0, backend TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '', prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0, input_bytes BIGINT NOT NULL DEFAULT 0,
    output_bytes BIGINT NOT NULL DEFAULT 0, recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, decision_index)
);
CREATE INDEX IF NOT EXISTS decision_exchanges_model_idx ON decision_exchanges(model, recorded_at DESC);
CREATE INDEX IF NOT EXISTS decision_exchanges_run_idx ON decision_exchanges(run_id, attempt);
CREATE INDEX IF NOT EXISTS decision_exchanges_kind_idx ON decision_exchanges(kind, recorded_at DESC);
CREATE TABLE IF NOT EXISTS objective_failures (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL, failure_key TEXT NOT NULL, fingerprint TEXT NOT NULL,
    family_key TEXT NOT NULL DEFAULT '', family_fingerprint TEXT NOT NULL DEFAULT '',
    blocking BOOLEAN NOT NULL DEFAULT FALSE, terminal_count INTEGER NOT NULL DEFAULT 0,
    failure_json JSONB NOT NULL, report_json JSONB NOT NULL,
    delivery_status TEXT NOT NULL DEFAULT 'pending', delivery_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), PRIMARY KEY (run_id, attempt, failure_key)
);
CREATE INDEX IF NOT EXISTS objective_failures_fingerprint_idx ON objective_failures(fingerprint, updated_at DESC);
CREATE INDEX IF NOT EXISTS objective_failures_family_idx ON objective_failures(family_key, updated_at DESC);
CREATE INDEX IF NOT EXISTS objective_failures_delivery_idx ON objective_failures(delivery_status, updated_at);
CREATE TABLE IF NOT EXISTS issue_fingerprints (
    failure_key TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_fingerprints_fingerprint_idx ON issue_fingerprints(fingerprint);
CREATE TABLE IF NOT EXISTS issue_occurrences (
    external_id TEXT PRIMARY KEY, failure_key TEXT NOT NULL, run_id TEXT NOT NULL, attempt INTEGER NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_occurrences_key_idx ON issue_occurrences(failure_key, created_at DESC);
CREATE INDEX IF NOT EXISTS issue_occurrences_run_idx ON issue_occurrences(run_id, attempt);
CREATE TABLE IF NOT EXISTS issue_links (
    failure_key TEXT PRIMARY KEY, issue_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '', payload_json JSONB NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_links_status_idx ON issue_links(status, updated_at DESC);
CREATE TABLE IF NOT EXISTS issue_outbox (
    external_id TEXT PRIMARY KEY, run_id TEXT NOT NULL, attempt INTEGER NOT NULL, failure_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL, next_attempt BIGINT NOT NULL DEFAULT 0, payload_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS issue_outbox_pending_idx ON issue_outbox(status, next_attempt, updated_at);
CREATE TABLE IF NOT EXISTS artifacts (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT '', sha256 TEXT NOT NULL DEFAULT '', store TEXT NOT NULL DEFAULT '',
    bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '', size BIGINT NOT NULL DEFAULT 0,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, kind, name)
);
CREATE INDEX IF NOT EXISTS artifacts_run_idx ON artifacts(run_id, attempt, kind);
CREATE INDEX IF NOT EXISTS artifacts_object_idx ON artifacts(store, bucket, object_key);
CREATE TABLE IF NOT EXISTS dataset_manifests (
    id TEXT PRIMARY KEY, name TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', query_json JSONB NOT NULL,
    manifest_json JSONB NOT NULL, object_key TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS dataset_manifests_created_idx ON dataset_manifests(created_at DESC);
`

const controlPlaneMigration002 = `
ALTER TABLE model_deployments
    ADD COLUMN IF NOT EXISTS discover BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_deployments
    ADD COLUMN IF NOT EXISTS default_for TEXT[] NOT NULL DEFAULT '{}';
`

const controlPlaneMigration004 = `
CREATE TABLE IF NOT EXISTS decision_exchanges (
    run_id TEXT NOT NULL, attempt INTEGER NOT NULL, decision_index INTEGER NOT NULL,
    kind TEXT NOT NULL DEFAULT '', question TEXT NOT NULL DEFAULT '', choice TEXT NOT NULL DEFAULT '',
    probabilities JSONB NOT NULL DEFAULT '{}'::jsonb, confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    fallback BOOLEAN NOT NULL DEFAULT FALSE, error TEXT NOT NULL DEFAULT '',
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0, backend TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '', prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0, input_bytes BIGINT NOT NULL DEFAULT 0,
    output_bytes BIGINT NOT NULL DEFAULT 0, recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, attempt, decision_index)
);
CREATE INDEX IF NOT EXISTS decision_exchanges_model_idx ON decision_exchanges(model, recorded_at DESC);
CREATE INDEX IF NOT EXISTS decision_exchanges_run_idx ON decision_exchanges(run_id, attempt);
CREATE INDEX IF NOT EXISTS decision_exchanges_kind_idx ON decision_exchanges(kind, recorded_at DESC);
`

// Migration 5 retires the static 7900 9B pin that competed with the
// discoverable qwen38-27b-7900 row after deploy/models.json switched to one
// switchable deployment. Stale leases still naming qwen3.5-9b against a host
// serving qwen3.8-27b are what produced farm fingerprint ed63cfe2fa840cee.
const controlPlaneMigration005 = `
DELETE FROM model_deployments WHERE id = 'qwen35-9b-7900';
UPDATE model_deployments
SET label = '7900 XTX',
    model_id = '',
    revision = '',
    artifact = '',
    quantization = '',
    api_model = '',
    discover = TRUE,
    engine_config = CASE WHEN engine_config = '' OR engine_config LIKE '7900-pinned%' THEN '7900-switchable' ELSE engine_config END,
    updated_at = NOW()
WHERE id = 'qwen38-27b-7900';
`

const controlPlaneMigration006 = `
ALTER TABLE objective_failures
    ADD COLUMN IF NOT EXISTS family_key TEXT NOT NULL DEFAULT '';
ALTER TABLE objective_failures
    ADD COLUMN IF NOT EXISTS family_fingerprint TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS objective_failures_family_idx
    ON objective_failures(family_key, updated_at DESC);
`

type controlPlane struct {
	db                *sql.DB
	experimentPersist sync.Mutex
}

var wallControlPlanes sync.Map         // *Wall -> *controlPlane
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
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	var applied bool
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
	var applied2 bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=2)`).Scan(&applied2); err != nil {
		return fmt.Errorf("read migration version 2: %w", err)
	}
	if !applied2 {
		if _, err := tx.Exec(controlPlaneMigration002); err != nil {
			return fmt.Errorf("apply control-plane migration 2: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(2) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 2: %w", err)
		}
	}
	var applied4 bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=4)`).Scan(&applied4); err != nil {
		return fmt.Errorf("read migration version 4: %w", err)
	}
	if !applied4 {
		if _, err := tx.Exec(controlPlaneMigration004); err != nil {
			return fmt.Errorf("apply control-plane migration 4: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(4) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 4: %w", err)
		}
	}
	var applied5 bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=5)`).Scan(&applied5); err != nil {
		return fmt.Errorf("read migration version 5: %w", err)
	}
	if !applied5 {
		if _, err := tx.Exec(controlPlaneMigration005); err != nil {
			return fmt.Errorf("apply control-plane migration 5: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(5) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 5: %w", err)
		}
	}
	var applied6 bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=6)`).Scan(&applied6); err != nil {
		return fmt.Errorf("read migration version 6: %w", err)
	}
	if !applied6 {
		if _, err := tx.Exec(controlPlaneMigration006); err != nil {
			return fmt.Errorf("apply control-plane migration 6: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(6) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record control-plane migration 6: %w", err)
		}
	}
	return tx.Commit()
}
