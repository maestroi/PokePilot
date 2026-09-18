-- PokePilot control-plane schema.
--
-- Large replay/debug artifacts remain in S3-compatible storage. PostgreSQL is
-- for queryable metadata and operator-controlled configuration.

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
    api_model TEXT NOT NULL,
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

CREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx
    ON model_deployments(enabled, compute, label, id);

-- Seed the deployment set that was previously represented by
-- deploy/models.example.json. ON CONFLICT deliberately preserves operator
-- edits if the init SQL is ever re-applied manually.
INSERT INTO model_deployments (
    id, label, model_id, discover_model, is_default, revision, artifact, quantization, compute, endpoint,
    api_model, enabled, control_url, endpoint_token_env, token_env, engine, engine_version,
    engine_config, max_parallel_workers, legacy_profile
) VALUES
    (
        'qwen38-27b-7900', 'Farm · RX 7900 XTX', '', TRUE, TRUE,
        '', '', '', 'RX 7900 XTX', 'http://192.168.50.130:8002/v1',
        '', TRUE, '', '', '', 'llama.cpp', 'replace-with-server-version',
        '7900-openai-discovery-4slot', 4, 'auto'
    ),
    (
        'qwen35-4b-4090', 'Qwen 3.5 4B · RTX 4090', 'qwen3.5-4b', FALSE, FALSE,
        'replace-with-model-revision-or-sha256', '/srv/models/qwen3.5-4b/model.gguf',
        'replace-with-quantization', 'RTX 4090', 'http://192.168.50.81:8002/v1',
        'pokepilot-4090', TRUE, 'http://192.168.50.81:8091', '', 'POKEPILOT_MODELHOST_TOKEN',
        'llama.cpp', 'replace-with-server-version', '4090-switchable', 1, 'gpu'
    ),
    (
        'qwen35-9b-4090', 'Qwen 3.5 9B · RTX 4090', 'qwen3.5-9b', FALSE, FALSE,
        'replace-with-model-revision-or-sha256', '/srv/models/qwen3.5-9b/model.gguf',
        'replace-with-quantization', 'RTX 4090', 'http://192.168.50.81:8002/v1',
        'pokepilot-4090', TRUE, 'http://192.168.50.81:8091', '', 'POKEPILOT_MODELHOST_TOKEN',
        'llama.cpp', 'replace-with-server-version', '4090-switchable', 1, 'gpu'
    )
ON CONFLICT (id) DO NOTHING;
