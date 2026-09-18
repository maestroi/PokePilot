package farm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadModelRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	data := `{"deployments":[{"id":"qwen-27b-7900","label":"Qwen 27B","model_id":"qwen3.8-27b","revision":"sha256:abc","compute":"rx7900xtx","endpoint":"http://7900:8080/v1","api_model":"pokepilot-7900","enabled":true},{"id":"qwen-4b-4090","label":"Qwen 4B","model_id":"qwen3.5-4b","compute":"rtx4090","endpoint":"http://4090:8080/v1","api_model":"pokepilot-4090","enabled":true}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadModelRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(registry.EnabledDeployments()); got != 2 {
		t.Fatalf("enabled deployments = %d, want 2", got)
	}
	first, ok := registry.Deployment("qwen-27b-7900")
	if !ok {
		t.Fatal("missing 7900 deployment")
	}
	if got := first.CompatibilityProfile(); got != "auto" {
		t.Fatalf("7900 compatibility profile = %q, want auto", got)
	}
	second, _ := registry.Deployment("qwen-4b-4090")
	if got := second.CompatibilityProfile(); got != "gpu" {
		t.Fatalf("4090 compatibility profile = %q, want gpu", got)
	}
	identity := first.Identity()
	if identity.ModelID != "qwen3.8-27b" || identity.Revision != "sha256:abc" || identity.Compute != "rx7900xtx" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestProductionModelRegistryValidates(t *testing.T) {
	registry, err := LoadModelRegistry(filepath.Join("..", "deploy", "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"qwen38-27b-7900", "qwen35-4b-4090"} {
		d, ok := registry.Deployment(id)
		if !ok || !d.Enabled {
			t.Fatalf("production registry missing enabled %s", id)
		}
		if !d.DiscoverModel && (d.Revision == "" || strings.HasPrefix(d.Revision, "replace-with-")) {
			t.Fatalf("%s still has a placeholder revision %q", id, d.Revision)
		}
	}
	a, _ := registry.Deployment("qwen38-27b-7900")
	b, _ := registry.Deployment("qwen35-4b-4090")
	if a.CompatibilityProfile() != "auto" {
		t.Fatalf("7900 legacy_profile = %q, want auto", a.CompatibilityProfile())
	}
	if !a.DiscoverModel || !a.Default || a.ParallelLimit() != 4 {
		t.Fatalf("7900 discovery/default/concurrency = %#v", a)
	}
	if b.CompatibilityProfile() != "gpu" || b.APIModel != "pokepilot-4090" || b.ControlURL == "" {
		t.Fatalf("4090 4B deployment = %#v", b)
	}
}

func TestModelRegistryRejectsDuplicateDeployment(t *testing.T) {
	registry := ModelRegistry{Deployments: []ModelDeployment{
		{ID: "same", ModelID: "a", Compute: "x", Endpoint: "http://x/v1", APIModel: "a", Enabled: true},
		{ID: "same", ModelID: "b", Compute: "y", Endpoint: "http://y/v1", APIModel: "b", Enabled: true},
	}}
	if err := registry.Validate(); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestRegistryDiscoveryAllowsModelIdentityFromEndpoint(t *testing.T) {
	registry := ModelRegistry{Deployments: []ModelDeployment{
		{ID: "dynamic", Compute: "gpu", Endpoint: "http://gpu/v1", Enabled: true, DiscoverModel: true, Default: true},
	}}
	if err := registry.Validate(); err != nil {
		t.Fatalf("dynamic deployment should validate: %v", err)
	}
	enabled := registry.EnabledDeployments()
	if len(enabled) != 1 || enabled[0].ID != "dynamic" {
		t.Fatalf("enabled deployments = %#v", enabled)
	}
}

func TestRegistryDefaultSortsBeforeHardwareOrModelName(t *testing.T) {
	registry := ModelRegistry{Deployments: []ModelDeployment{
		{ID: "a", ModelID: "a", Compute: "aaa", Endpoint: "http://a/v1", APIModel: "a", Enabled: true},
		{ID: "z", ModelID: "z", Compute: "zzz", Endpoint: "http://z/v1", APIModel: "z", Enabled: true, Default: true},
	}}
	if err := registry.Validate(); err != nil {
		t.Fatal(err)
	}
	enabled := registry.EnabledDeployments()
	if len(enabled) != 2 || enabled[0].ID != "z" {
		t.Fatalf("default deployment was not first: %#v", enabled)
	}
}
