package farm

import "testing"

func TestModelDeploymentParallelLimitDefaultsToOne(t *testing.T) {
	if got := (ModelDeployment{}).ParallelLimit(); got != 1 {
		t.Fatalf("default parallel limit = %d, want 1", got)
	}
	if got := (ModelDeployment{MaxParallelWorkers: 3}).ParallelLimit(); got != 3 {
		t.Fatalf("explicit parallel limit = %d, want 3", got)
	}
}

func TestSetParallelLimitPersistsJSONRegistry(t *testing.T) {
	path := t.TempDir() + "/models.json"
	registry := ModelRegistry{Deployments: []ModelDeployment{
		{ID: "model-a", ModelID: "a", Compute: "gpu", Endpoint: "http://a/v1", APIModel: "a", Enabled: true, MaxParallelWorkers: 1},
	}}
	if err := SaveModelRegistry(path, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateDeploymentParallelLimit(path, "model-a", 3); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadModelRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Deployment("model-a")
	if !ok || got.ParallelLimit() != 3 {
		t.Fatalf("updated = %#v ok=%v", got, ok)
	}
	if _, err := UpdateDeploymentParallelLimit(path, "model-a", 0); err == nil {
		t.Fatal("expected invalid limit error")
	}
}
