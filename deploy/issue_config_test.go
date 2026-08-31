package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestIssueConfigReachesWallOnly(t *testing.T) {
	yml, err := os.ReadFile("farm.yml")
	if err != nil {
		t.Fatalf("read farm.yml: %v", err)
	}
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	s := string(yml)
	wallIdx := strings.Index(s, "\n  wall:")
	uiIdx := strings.Index(s, "\n  ui:")
	runnerIdx := strings.Index(s, "\n  runner:")
	if wallIdx < 0 || uiIdx < 0 || runnerIdx < 0 || !(wallIdx < uiIdx && uiIdx < runnerIdx) {
		t.Fatalf("farm.yml service order: wall=%d ui=%d runner=%d", wallIdx, uiIdx, runnerIdx)
	}
	wall := s[wallIdx:uiIdx]
	ui := s[uiIdx:runnerIdx]
	runner := s[runnerIdx:]

	for _, flag := range []string{
		"-issues-api",
		"${AGENT_ORCHESTRATOR_API:-}",
		"-issues-project",
		"${AGENT_ORCHESTRATOR_POKEPILOT_PROJECT_ID:-}",
		"-issues-ui",
		"${AGENT_ORCHESTRATOR_UI:-}",
	} {
		if !strings.Contains(wall, flag) {
			t.Errorf("wall command missing %q", flag)
		}
	}
	if strings.Contains(ui, "-issues-") || strings.Contains(runner, "-issues-") {
		t.Error("issue flags must not reach ui or runner")
	}
	if strings.Contains(s, "192.168.50.81") {
		t.Error("LAN Agent Orchestrator examples must not be stack defaults")
	}
	img := string(dockerfile)
	if strings.Contains(img, "issues-api") || strings.Contains(img, "AGENT_ORCHESTRATOR") {
		t.Error("issue settings must not be baked into the image")
	}
}
