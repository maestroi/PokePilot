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
