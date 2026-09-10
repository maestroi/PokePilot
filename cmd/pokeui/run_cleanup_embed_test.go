package main

import (
	"strings"
	"testing"
)

func TestRunCleanupIsMountedAfterMainConsoleScript(t *testing.T) {
	html := string(operatorIndexPage())
	mainScript := strings.Index(html, `<script src="/ui.js"></script>`)
	cleanupScript := strings.Index(html, `id="run-cleanup-script"`)
	if mainScript < 0 {
		t.Fatal("operator page missing main ui.js script")
	}
	if cleanupScript < 0 {
		t.Fatal("operator page missing run cleanup script")
	}
	if cleanupScript <= mainScript {
		t.Fatal("run cleanup must load after ui.js so the Runs view already exists")
	}
}

func TestRunCleanupUsesSafeFinishedRunDeletePath(t *testing.T) {
	js := string(runCleanupJS)
	for _, want := range []string{
		`status === "done"`,
		`ended_at`,
		`DELETE_CONCURRENCY = 3`,
		`method: "DELETE"`,
		`/v1/runs/`,
		`root.confirm`,
		`S3 artifacts and replay cache`,
		`pokefarm-console-view`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("run cleanup missing %q", want)
		}
	}
	if strings.Contains(js, `/artifacts`) {
		t.Error("browser cleanup must delete runs through the safe run endpoint, not call artifact deletion directly")
	}
	if strings.Contains(js, `setInterval(`) {
		t.Error("run cleanup must not add another continuous dashboard poll")
	}
}
