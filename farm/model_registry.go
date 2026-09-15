package farm

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ModelRegistry is the declarative set of inference deployments available to
// PokePilot. It deliberately separates model identity from compute placement:
// the same model may have more than one deployment and a single compute host
// may switch between several approved model artifacts.
type ModelRegistry struct {
	Deployments []ModelDeployment `json:"deployments"`
}

// ModelDeployment describes one selectable inference target. TokenEnv names an
// environment variable containing a bearer token; the token itself is never
// carried in the registry, run spec, dashboard or persisted experiment data.
type ModelDeployment struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ModelID       string `json:"model_id"`
	Revision      string `json:"revision,omitempty"`
	Artifact      string `json:"artifact,omitempty"`
	Quantization  string `json:"quantization,omitempty"`
	Compute       string `json:"compute"`
	Endpoint      string `json:"endpoint"`
	APIModel      string `json:"api_model"`
	Enabled       bool   `json:"enabled"`
	ControlURL    string `json:"control_url,omitempty"`
	TokenEnv      string `json:"token_env,omitempty"`
	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
	EngineConfig  string `json:"engine_config,omitempty"`
	// LegacyProfile is only the compatibility adapter used by existing
	// runners to choose the already-configured compute endpoint. New operator
	// and experiment code selects ID, never this value.
	LegacyProfile string `json:"legacy_profile,omitempty"`
}

// InferenceIdentity is the immutable, secret-free model/deployment identity
// copied into a run at enqueue time. Friendly labels are included for display
// but are never sufficient for reproducibility on their own.
type InferenceIdentity struct {
	DeploymentID  string `json:"deployment_id"`
	Label         string `json:"label,omitempty"`
	ModelID       string `json:"model_id"`
	Revision      string `json:"revision,omitempty"`
	Artifact      string `json:"artifact,omitempty"`
	Quantization  string `json:"quantization,omitempty"`
	Compute       string `json:"compute"`
	Endpoint      string `json:"endpoint"`
	APIModel      string `json:"api_model"`
	ControlURL    string `json:"control_url,omitempty"`
	TokenEnv      string `json:"token_env,omitempty"`
	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
	EngineConfig  string `json:"engine_config,omitempty"`
}

const postgresRegistryEnvPrefix = "postgres-env://"

// LoadModelRegistry accepts the historical JSON file path, a Postgres DSN, or
// the production-safe form postgres-env://ENV_NAME. The env indirection is
// preferred in services whose startup errors log the source string, because it
// keeps credentials out of logs while preserving JSON compatibility locally.
func LoadModelRegistry(source string) (ModelRegistry, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(source), postgresRegistryEnvPrefix) {
		envName := strings.TrimSpace(source[len(postgresRegistryEnvPrefix):])
		if envName == "" {
			return ModelRegistry{}, fmt.Errorf("model registry: postgres environment variable name is empty")
		}
		dsn := strings.TrimSpace(os.Getenv(envName))
		if dsn == "" {
			return ModelRegistry{}, fmt.Errorf("model registry: environment variable %s is empty", envName)
		}
		if !isPostgresRegistrySource(dsn) {
			return ModelRegistry{}, fmt.Errorf("model registry: environment variable %s is not a postgres DSN", envName)
		}
		return loadModelRegistryPostgres(dsn)
	}
	if isPostgresRegistrySource(source) {
		return loadModelRegistryPostgres(source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return ModelRegistry{}, fmt.Errorf("read model registry: %w", err)
	}
	var registry ModelRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return ModelRegistry{}, fmt.Errorf("decode model registry: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return ModelRegistry{}, err
	}
	return registry, nil
}

func (r ModelRegistry) Validate() error {
	seen := map[string]bool{}
	for i := range r.Deployments {
		d := r.Deployments[i]
		id := strings.TrimSpace(d.ID)
		if id == "" {
			return fmt.Errorf("model registry: deployment %d has empty id", i)
		}
		if seen[id] {
			return fmt.Errorf("model registry: duplicate deployment id %q", id)
		}
		seen[id] = true
		if strings.TrimSpace(d.ModelID) == "" {
			return fmt.Errorf("model registry: deployment %q has empty model_id", id)
		}
		if strings.TrimSpace(d.Compute) == "" {
			return fmt.Errorf("model registry: deployment %q has empty compute", id)
		}
		if strings.TrimSpace(d.Endpoint) == "" {
			return fmt.Errorf("model registry: deployment %q has empty endpoint", id)
		}
		if strings.TrimSpace(d.APIModel) == "" {
			return fmt.Errorf("model registry: deployment %q has empty api_model", id)
		}
		switch p := strings.TrimSpace(d.LegacyProfile); p {
		case "", "auto", "gpu", "default":
		default:
			return fmt.Errorf("model registry: deployment %q has invalid legacy_profile %q", id, p)
		}
	}
	return nil
}

func (r ModelRegistry) Deployment(id string) (ModelDeployment, bool) {
	id = strings.TrimSpace(id)
	for _, d := range r.Deployments {
		if d.ID == id {
			return d, true
		}
	}
	return ModelDeployment{}, false
}

func (r ModelRegistry) EnabledDeployments() []ModelDeployment {
	out := make([]ModelDeployment, 0, len(r.Deployments))
	for _, d := range r.Deployments {
		if d.Enabled {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Compute != out[j].Compute {
			return out[i].Compute < out[j].Compute
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func (d ModelDeployment) Identity() InferenceIdentity {
	return InferenceIdentity{
		DeploymentID: d.ID, Label: d.Label, ModelID: d.ModelID,
		Revision: d.Revision, Artifact: d.Artifact, Quantization: d.Quantization,
		Compute: d.Compute, Endpoint: d.Endpoint, APIModel: d.APIModel,
		ControlURL: d.ControlURL, TokenEnv: d.TokenEnv, Engine: d.Engine,
		EngineVersion: d.EngineVersion, EngineConfig: d.EngineConfig,
	}
}

// CompatibilityProfile maps a deployment onto the stable llm_profile wire
// values used by older runners. New callers should never choose deployments by
// this value; it exists only so a mixed-version farm can roll out safely.
func (d ModelDeployment) CompatibilityProfile() string {
	if p := strings.TrimSpace(d.LegacyProfile); p != "" {
		return p
	}
	compute := strings.ToLower(d.Compute)
	switch {
	case strings.Contains(compute, "4090"):
		return "gpu"
	case strings.Contains(compute, "cpu"), strings.Contains(compute, "lan"):
		return "default"
	default:
		return "auto"
	}
}
