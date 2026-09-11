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
		if strings.Contains(line, "AGENT_ORCHESTRATOR") && strings.Contains(line, "orchestrator.labstack.cc") && !strings.Contains(line, "${AGENT_ORCHESTRATOR") {
			t.Error("Agent Orchestrator host must stay an operator-provided env value, not a baked stack default")
			break
		}
	}

	// Inference topology: runners normally bypass LiteLLM and call the 7900 XTX
	// directly. The stable "auto" profile uses CPU/LAN only after a transport
	// failure or the 120s 7900 request timeout. The 4090 and CPU are explicit
	// selectable routes. LiteLLM remains deployed but its gateway URL is opt-in.
	for _, want := range []string{
		"POKEPILOT_LLM_GATEWAY_URL: ${POKEPILOT_LLM_GATEWAY_URL:-}",
		"POKEPILOT_LLM_URL: ${POKEPILOT_LLM_URL:-http://192.168.50.204:8000/v1}",
		"POKEPILOT_LLM_GPU_URL: ${POKEPILOT_LLM_GPU_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LLM_GPU_TIMEOUT: ${POKEPILOT_LLM_GPU_TIMEOUT:-120s}",
		"POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT: ${POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT:-off}",
		"POKEPILOT_LLM_4090_URL: ${POKEPILOT_LLM_4090_URL:-http://192.168.50.81:8002/v1}",
		"POKEPILOT_LLM_4090_MODEL: ${POKEPILOT_LLM_4090_MODEL:-qwen3.8-27b}",
		"POKEPILOT_LLM_4090_RECOVERY_REASONING_EFFORT: ${POKEPILOT_LLM_4090_RECOVERY_REASONING_EFFORT:-off}",
		"POKEPILOT_LLM_GATEWAY_GPU_MODEL: ${POKEPILOT_LLM_GATEWAY_GPU_MODEL:-pokepilot-4090}",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner missing inference setting %q", want)
		}
	}
	for _, want := range []string{
		"POKEPILOT_LITELLM_7900_URL: ${POKEPILOT_LITELLM_7900_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LITELLM_4090_URL: ${POKEPILOT_LITELLM_4090_URL:-http://192.168.50.81:8002/v1}",
		"POKEPILOT_LITELLM_LAN_URL: ${POKEPILOT_LITELLM_LAN_URL:-http://192.168.50.204:8000/v1}",
		"POKEPILOT_LITELLM_LAN_KEY: ${POKEPILOT_LITELLM_LAN_KEY:-${llm_token:-}}",
		// hosted_vllm/ forwards chat_template_kwargs; openai/ drops them and
		// leaves Qwen 3.8 on its default xhigh thinking path.
		"POKEPILOT_LITELLM_7900_MODEL: ${POKEPILOT_LITELLM_7900_MODEL:-hosted_vllm/qwen3.8-27b}",
		"POKEPILOT_LITELLM_4090_MODEL: ${POKEPILOT_LITELLM_4090_MODEL:-hosted_vllm/qwen3.8-27b}",
		"POKEPILOT_LITELLM_LAN_MODEL: ${POKEPILOT_LITELLM_LAN_MODEL:-hosted_vllm/qwen3.5-4b}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stack missing parked LiteLLM backend %q", want)
		}
	}

	img := string(dockerfile)
	if strings.Contains(img, "issues-api") || strings.Contains(img, "AGENT_ORCHESTRATOR") {
		t.Error("issue settings must not be baked into the image")
	}
}
