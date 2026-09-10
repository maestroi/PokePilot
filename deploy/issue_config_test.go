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
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "AGENT_ORCHESTRATOR") && strings.Contains(line, "192.168.50.81") {
			t.Error("LAN Agent Orchestrator examples must not be stack defaults")
			break
		}
	}

	// Inference topology: LiteLLM is the normal entry point, the dedicated
	// 7900 XTX is the direct GPU escape hatch, and direct LAN is the gateway
	// outage safety net. The 4090 is gateway-owned overflow, never the default
	// direct GPU, so Operator can reserve it for coding.
	for _, want := range []string{
		"POKEPILOT_LLM_GATEWAY_URL: ${POKEPILOT_LLM_GATEWAY_URL:-http://litellm:4000/v1}",
		"POKEPILOT_LLM_URL: ${POKEPILOT_LLM_URL:-http://192.168.50.204:8002/v1}",
		"POKEPILOT_LLM_GPU_URL: ${POKEPILOT_LLM_GPU_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT: ${POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT:-off}",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner missing inference setting %q", want)
		}
	}
	for _, want := range []string{
		"POKEPILOT_LITELLM_7900_URL: ${POKEPILOT_LITELLM_7900_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LITELLM_4090_URL: ${POKEPILOT_LITELLM_4090_URL:-http://192.168.50.81:8002/v1}",
		"POKEPILOT_LITELLM_LAN_URL: ${POKEPILOT_LITELLM_LAN_URL:-http://192.168.50.204:8002/v1}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stack missing LiteLLM backend %q", want)
		}
	}

	img := string(dockerfile)
	if strings.Contains(img, "issues-api") || strings.Contains(img, "AGENT_ORCHESTRATOR") {
		t.Error("issue settings must not be baked into the image")
	}
}
