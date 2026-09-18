package farm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ErrDeploymentNotFound is returned when a registry mutation names an unknown
// deployment id.
var ErrDeploymentNotFound = errors.New("model registry: deployment not found")

// MaxParallelWorkersLimit is the operator-facing ceiling. Farm runs spend most
// of their time emulating, so one inference process can interleave several
// runners; this bound only stops a typo from requesting hundreds of leases.
const MaxParallelWorkersLimit = 32

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
	ID                 string `json:"id"`
	Label              string `json:"label"`
	ModelID            string `json:"model_id"`
	Revision           string `json:"revision,omitempty"`
	Artifact           string `json:"artifact,omitempty"`
	Quantization       string `json:"quantization,omitempty"`
	Compute            string `json:"compute"`
	Endpoint           string `json:"endpoint"`
	APIModel           string `json:"api_model"`
	Enabled            bool     `json:"enabled"`
	// Discover asks the wall to probe the OpenAI-compatible /v1/models endpoint
	// and bind runs to the model actually being served. This is useful for
	// pinned llama.cpp/vLLM/cloud endpoints whose model can change without a
	// PokePilot deploy. Switchable hosts with ControlURL normally leave this off.\n\tDiscover           bool     `json:"discover,omitempty"`
	// DefaultFor gives operator surfaces stable roles without encoding model
	// sizes or hardware in code (for example "farm", "experiment-a").\n\tDefaultFor         []string `json:"default_for,omitempty"`\n\tControlURL         string `json:"control_url,omitempty"`
	TokenEnv           string `json:"token_env,omitempty"`
	Engine             string `json:"engine,omitempty"`
	EngineVersion      string `json:"engine_version,omitempty"`
	EngineConfig       string `json:"engine_config,omitempty"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
	// LegacyProfile is only the compatibility adapter used by existing
	// runners to choose the already-configured compute endpoint. New operator
	// and experiment code selects ID, never this value.
	LegacyProfile string `json:"legacy_profile,omitempty"`
}

// InferenceIdentity is the immutable, secret-free model/deployment identity
// copied into a run at enqueue time. Friendly labels are included for display
// but are never sufficient for reproducibility on their own.
type InferenceIdentity struct {
	DeploymentID       string `json:"deployment_id"`
	Label              string `json:"label,omitempty"`
	ModelID            string `json:"model_id"`
	Revision           string `json:"revision,omitempty"`
	Artifact           string `json:"artifact,omitempty"`
	Quantization       string `json:"quantization,omitempty"`
	Compute            string `json:"compute"`
	Endpoint           string `json:"endpoint"`
	APIModel           string `json:"api_model"`
	ControlURL         string `json:"control_url,omitempty"`
	TokenEnv           string `json:"token_env,omitempty"`
	Engine             string `json:"engine,omitempty"`
	EngineVersion      string `json:"engine_version,omitempty"`
	EngineConfig       string `json:"engine_config,omitempty"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
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
		if strings.TrimSpace(d.ModelID) == "" && !d.Discover {
			return fmt.Errorf("model registry: deployment %q has empty model_id (set discover=true for endpoint discovery)", id)
		}
		if strings.TrimSpace(d.Compute) == "" {
			return fmt.Errorf("model registry: deployment %q has empty compute", id)
		}
		if strings.TrimSpace(d.Endpoint) == "" {
			return fmt.Errorf("model registry: deployment %q has empty endpoint", id)
		}
		if strings.TrimSpace(d.APIModel) == "" && !d.Discover {
			return fmt.Errorf("model registry: deployment %q has empty api_model (set discover=true for endpoint discovery)", id)
		}
		if d.MaxParallelWorkers < 0 {
			return fmt.Errorf("model registry: deployment %q has invalid max_parallel_workers %d", id, d.MaxParallelWorkers)
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

// HasDefaultRole reports whether this deployment is preferred for an operator role.\n// Roles are intentionally free-form so adding another GPU or a cloud pool does not\n// require another enum/code change.\nfunc (d ModelDeployment) HasDefaultRole(role string) bool {\n\trole = strings.TrimSpace(strings.ToLower(role))\n\tfor _, candidate := range d.DefaultFor {\n\t\tif strings.ToLower(strings.TrimSpace(candidate)) == role {\n\t\t\treturn true\n\t\t}\n\t}\n\treturn false\n}\n\nfunc (d ModelDeployment) Identity() InferenceIdentity {
	return InferenceIdentity{
		DeploymentID: d.ID, Label: d.Label, ModelID: d.ModelID,
		Revision: d.Revision, Artifact: d.Artifact, Quantization: d.Quantization,
		Compute: d.Compute, Endpoint: d.Endpoint, APIModel: d.APIModel,
		ControlURL: d.ControlURL, TokenEnv: d.TokenEnv, Engine: d.Engine,
		EngineVersion: d.EngineVersion, EngineConfig: d.EngineConfig,
		MaxParallelWorkers: d.ParallelLimit(),
	}
}

// ParallelLimit returns the deployment worker cap. Zero is intentionally
// backwards-compatible and means one active worker at a time.
func (d ModelDeployment) ParallelLimit() int {
	if d.MaxParallelWorkers <= 0 {
		return 1
	}
	return d.MaxParallelWorkers
}

// SetParallelLimit updates one deployment's live worker cap in memory.
func (r *ModelRegistry) SetParallelLimit(id string, n int) error {
	if n < 1 || n > MaxParallelWorkersLimit {
		return fmt.Errorf("model registry: max_parallel_workers must be between 1 and %d", MaxParallelWorkersLimit)
	}
	id = strings.TrimSpace(id)
	for i := range r.Deployments {
		if r.Deployments[i].ID == id {
			r.Deployments[i].MaxParallelWorkers = n
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// SaveModelRegistry writes a JSON registry file.
func SaveModelRegistry(path string, registry ModelRegistry) error {
	if err := registry.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model registry: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write model registry: %w", err)
	}
	return nil
}

// UpdateDeploymentParallelLimit persists a new worker cap to the registry
// source (JSON file or Postgres) and returns the updated deployment.
func UpdateDeploymentParallelLimit(source, id string, n int) (ModelDeployment, error) {
	registry, err := LoadModelRegistry(source)
	if err != nil {
		return ModelDeployment{}, err
	}
	if err := registry.SetParallelLimit(id, n); err != nil {
		return ModelDeployment{}, err
	}
	pathOrDSN, postgres, err := registryPersistTarget(source)
	if err != nil {
		return ModelDeployment{}, err
	}
	if postgres {
		if err := updatePostgresParallelLimit(pathOrDSN, id, n); err != nil {
			return ModelDeployment{}, err
		}
	} else if err := SaveModelRegistry(pathOrDSN, registry); err != nil {
		return ModelDeployment{}, err
	}
	updated, _ := registry.Deployment(id)
	return updated, nil
}

func registryPersistTarget(source string) (string, bool, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(source), postgresRegistryEnvPrefix) {
		envName := strings.TrimSpace(source[len(postgresRegistryEnvPrefix):])
		if envName == "" {
			return "", true, fmt.Errorf("model registry: postgres environment variable name is empty")
		}
		dsn := strings.TrimSpace(os.Getenv(envName))
		if dsn == "" {
			return "", true, fmt.Errorf("model registry: environment variable %s is empty", envName)
		}
		if !isPostgresRegistrySource(dsn) {
			return "", true, fmt.Errorf("model registry: environment variable %s is not a postgres DSN", envName)
		}
		return dsn, true, nil
	}
	if isPostgresRegistrySource(source) {
		return source, true, nil
	}
	if source == "" {
		return "", false, fmt.Errorf("model registry: source is empty")
	}
	return source, false, nil
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
