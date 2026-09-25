package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVueOperatorBulkCleanupUsesSafeFinishedRunDeletePath(t *testing.T) {
	root := filepath.Join("..", "..", "web", "src", "operator")
	files := map[string][]string{
		"runCleanup.ts": {
			`status === 'done'`,
			`FAILURE_REASONS = new Set(['error', 'lost', 'failed', 'stuck'])`,
			`DELETE_CONCURRENCY = 3`,
			`failure-id:`,
		},
		"FailuresView.vue": {
			`getDashboard({ status: 'done' })`,
			`matchingRunsForGroups`,
			`Delete selected`,
			`Delete matching`,
		},
		"RunCleanupPanel.vue": {
			`getDashboard({ status: 'done' })`,
			`Delete matching runs`,
			`S3 artifacts and replay cache`,
		},
	}
	for name, wants := range files {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		if strings.Contains(text, `/artifacts`) && !strings.Contains(name, "test") {
			t.Errorf("%s must delete through /v1/runs/{id}, not artifact deletion directly", name)
		}
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}
