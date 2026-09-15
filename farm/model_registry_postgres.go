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
SELECT id, label, model_id, revision, artifact, quantization, compute, endpoint,
       api_model, enabled, control_url, token_env, engine, engine_version,
       engine_config, legacy_profile
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
			&d.ID, &d.Label, &d.ModelID, &d.Revision, &d.Artifact,
			&d.Quantization, &d.Compute, &d.Endpoint, &d.APIModel, &d.Enabled,
			&d.ControlURL, &d.TokenEnv, &d.Engine, &d.EngineVersion,
			&d.EngineConfig, &d.LegacyProfile,
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
