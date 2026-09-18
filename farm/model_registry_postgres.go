package farm

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const modelRegistryPostgresSchema = `
CREATE TABLE IF NOT EXISTS model_deployments (
	id TEXT PRIMARY KEY,
	label TEXT NOT NULL DEFAULT '',
	model_id TEXT NOT NULL DEFAULT '',
	discover_model BOOLEAN NOT NULL DEFAULT FALSE,
	is_default BOOLEAN NOT NULL DEFAULT FALSE,
	revision TEXT NOT NULL DEFAULT '',
	artifact TEXT NOT NULL DEFAULT '',
	quantization TEXT NOT NULL DEFAULT '',
	compute TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	api_model TEXT NOT NULL DEFAULT '',
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	control_url TEXT NOT NULL DEFAULT '',
	endpoint_token_env TEXT NOT NULL DEFAULT '',
	token_env TEXT NOT NULL DEFAULT '',
	engine TEXT NOT NULL DEFAULT '',
	engine_version TEXT NOT NULL DEFAULT '',
	engine_config TEXT NOT NULL DEFAULT '',
	max_parallel_workers INTEGER NOT NULL DEFAULT 1,
	legacy_profile TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE model_deployments
	ADD COLUMN IF NOT EXISTS max_parallel_workers INTEGER NOT NULL DEFAULT 1;
ALTER TABLE model_deployments
	ADD COLUMN IF NOT EXISTS discover_model BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_deployments
	ADD COLUMN IF NOT EXISTS is_default BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE model_deployments
	ADD COLUMN IF NOT EXISTS endpoint_token_env TEXT NOT NULL DEFAULT '';
ALTER TABLE model_deployments
	ALTER COLUMN model_id SET DEFAULT '';
ALTER TABLE model_deployments
	ALTER COLUMN api_model SET DEFAULT '';
CREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx
	ON model_deployments(enabled, compute, label, id);
`

func isPostgresRegistrySource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(source, "postgres://") || strings.HasPrefix(source, "postgresql://")
}

func loadModelRegistryPostgres(dsn string) (ModelRegistry, error) {
	db, err := sql.Open("postgres", strings.TrimSpace(dsn))
	if err != nil {
		return ModelRegistry{}, fmt.Errorf("open model registry postgres: %w", err)
	}
	defer db.Close()

	// Swarm does not provide startup ordering. Give a newly-created database a
	// short window to finish initdb rather than permanently starting PokéWall
	// with an empty model registry after a normal stack deployment.
	var pingErr error
	for attempt := 0; attempt < 20; attempt++ {
		if pingErr = db.Ping(); pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		return ModelRegistry{}, fmt.Errorf("connect model registry postgres: %w", pingErr)
	}
	if _, err := db.Exec(modelRegistryPostgresSchema); err != nil {
		return ModelRegistry{}, fmt.Errorf("ensure model registry schema: %w", err)
	}

	rows, err := db.Query(`
SELECT id, label, model_id, discover_model, is_default, revision, artifact, quantization, compute, endpoint,
       api_model, enabled, control_url, endpoint_token_env, token_env, engine, engine_version,
       engine_config, max_parallel_workers, legacy_profile
FROM model_deployments
ORDER BY compute, label, id`)
	if err != nil {
		return ModelRegistry{}, fmt.Errorf("query model registry postgres: %w", err)
	}
	defer rows.Close()

	registry := ModelRegistry{Deployments: []ModelDeployment{}}
	for rows.Next() {
		var d ModelDeployment
		if err := rows.Scan(
			&d.ID, &d.Label, &d.ModelID, &d.DiscoverModel, &d.Default, &d.Revision, &d.Artifact,
			&d.Quantization, &d.Compute, &d.Endpoint, &d.APIModel, &d.Enabled,
			&d.ControlURL, &d.EndpointTokenEnv, &d.TokenEnv, &d.Engine, &d.EngineVersion,
			&d.EngineConfig, &d.MaxParallelWorkers, &d.LegacyProfile,
		); err != nil {
			return ModelRegistry{}, fmt.Errorf("scan model registry postgres: %w", err)
		}
		registry.Deployments = append(registry.Deployments, d)
	}
	if err := rows.Err(); err != nil {
		return ModelRegistry{}, fmt.Errorf("read model registry postgres: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return ModelRegistry{}, err
	}
	return registry, nil
}

func updatePostgresParallelLimit(dsn, id string, n int) error {
	db, err := sql.Open("postgres", strings.TrimSpace(dsn))
	if err != nil {
		return fmt.Errorf("open model registry postgres: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(modelRegistryPostgresSchema); err != nil {
		return fmt.Errorf("ensure model registry schema: %w", err)
	}
	res, err := db.Exec(
		`UPDATE model_deployments SET max_parallel_workers = $1, updated_at = NOW() WHERE id = $2`,
		n, strings.TrimSpace(id),
	)
	if err != nil {
		return fmt.Errorf("update model registry postgres: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update model registry postgres: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
	}
	return nil
}
