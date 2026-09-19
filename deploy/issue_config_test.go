package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestIssueConfigUsesGitHubAdapter(t *testing.T) {
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
	issuesIdx := strings.Index(s, "\n  issues:")
	uiIdx := strings.Index(s, "\n  ui:")
	runnerIdx := strings.Index(s, "\n  runner:")
	if wallIdx < 0 || issuesIdx < 0 || uiIdx < 0 || runnerIdx < 0 || !(wallIdx < issuesIdx && issuesIdx < uiIdx && uiIdx < runnerIdx) {
		t.Fatalf("farm.yml service order: wall=%d issues=%d ui=%d runner=%d", wallIdx, issuesIdx, uiIdx, runnerIdx)
	}
	wall := s[wallIdx:issuesIdx]
	issues := s[issuesIdx:uiIdx]
	ui := s[uiIdx:runnerIdx]
	runner := s[runnerIdx:]

	for _, want := range []string{
		"-issues-api",
		"http://issues:8080",
		"-issues-project",
		"pokepilot",
		"-issues-ui",
		"https://github.com/${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}",
	} {
		if !strings.Contains(wall, want) {
			t.Errorf("wall command missing %q", want)
		}
	}
	for _, want := range []string{
		"command: [\"pokeissues\", \"-http\", \":8080\"]",
		"POKEPILOT_GITHUB_REPO: ${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}",
		"POKEPILOT_GITHUB_TOKEN: ${POKEPILOT_GITHUB_TOKEN:-}",
		"POKEPILOT_RUN_BASE_URL: ${POKEPILOT_RUN_BASE_URL:-https://admin.rompilot.app}",
	} {
		if !strings.Contains(issues, want) {
			t.Errorf("issues service missing %q", want)
		}
	}
	if strings.Contains(wall, "POKEPILOT_GITHUB_TOKEN") || strings.Contains(ui, "POKEPILOT_GITHUB_TOKEN") || strings.Contains(runner, "POKEPILOT_GITHUB_TOKEN") {
		t.Error("GitHub credential must reach only the issues adapter")
	}
	for _, want := range []string{
		"POKEPILOT_MODEL_REGISTRY: /etc/pokepilot/models.json",
		"POKEPILOT_MODELHOST_TOKEN: ${POKEPILOT_MODELHOST_TOKEN:-}",
		"file: ./models.json",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stack missing model registry setting %q", want)
		}
	}
	if strings.Contains(s, "AGENT_ORCHESTRATOR") || strings.Contains(s, "orchestrator.labstack.cc") {
		t.Error("farm stack must no longer depend on private Agent Orchestrator")
	}

	// Inference topology: runners normally bypass LiteLLM and call the 7900 XTX
	// directly. The stable "auto" profile uses CPU/LAN only after a transport
	// failure or the 120s 7900 request timeout. The 4090 and CPU are explicit
	// selectable routes. LiteLLM remains deployed but its gateway URL is opt-in.
	for _, want := range []string{
		"POKEPILOT_LLM_GATEWAY_URL: ${POKEPILOT_LLM_GATEWAY_URL:-}",
		"POKEPILOT_LLM_URL: ${POKEPILOT_LLM_URL:-http://192.168.50.204:8000/v1}",
		"POKEPILOT_LLM_GPU_URL: ${POKEPILOT_LLM_GPU_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LLM_GPU_MODEL: ${POKEPILOT_LLM_GPU_MODEL:-qwen3.5-9b}",
		"POKEPILOT_LLM_GPU_TIMEOUT: ${POKEPILOT_LLM_GPU_TIMEOUT:-120s}",
		"POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT: ${POKEPILOT_LLM_GPU_RECOVERY_REASONING_EFFORT:-off}",
		"POKEPILOT_LLM_4090_URL: ${POKEPILOT_LLM_4090_URL:-http://192.168.50.81:8002/v1}",
		"POKEPILOT_LLM_4090_MODEL: ${POKEPILOT_LLM_4090_MODEL:-pokepilot-4090}",
		"POKEPILOT_LLM_4090_RECOVERY_REASONING_EFFORT: ${POKEPILOT_LLM_4090_RECOVERY_REASONING_EFFORT:-off}",
		"POKEPILOT_LLM_GATEWAY_GPU_MODEL: ${POKEPILOT_LLM_GATEWAY_GPU_MODEL:-pokepilot-4090}",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("runner missing inference setting %q", want)
		}
	}
	// Virtual trade stays off until gomeboy's serial scheduler can exit a
	// stalled Cable Club handshake (PR #1141 only fails the skill fast).
	for _, line := range strings.Split(runner, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "POKEPILOT_VIRTUAL_TRADER_URL:") {
			t.Errorf("runner must not enable virtual trader until gomeboy is fixed; found %q", trimmed)
		}
	}
	for _, want := range []string{
		"POKEPILOT_LITELLM_7900_URL: ${POKEPILOT_LITELLM_7900_URL:-http://192.168.50.130:8002/v1}",
		"POKEPILOT_LITELLM_4090_URL: ${POKEPILOT_LITELLM_4090_URL:-http://192.168.50.81:8002/v1}",
		"POKEPILOT_LITELLM_LAN_URL: ${POKEPILOT_LITELLM_LAN_URL:-http://192.168.50.204:8000/v1}",
		"POKEPILOT_LITELLM_LAN_KEY: ${POKEPILOT_LITELLM_LAN_KEY:-${llm_token:-}}",
		// hosted_vllm/ forwards chat_template_kwargs; openai/ drops them and
		// leaves Qwen 3.8 on its default xhigh thinking path.
		"POKEPILOT_LITELLM_7900_MODEL: ${POKEPILOT_LITELLM_7900_MODEL:-hosted_vllm/qwen3.5-9b}",
		"POKEPILOT_LITELLM_4090_MODEL: ${POKEPILOT_LITELLM_4090_MODEL:-hosted_vllm/pokepilot-4090}",
		"POKEPILOT_LITELLM_LAN_MODEL: ${POKEPILOT_LITELLM_LAN_MODEL:-hosted_vllm/qwen3.5-4b}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stack missing parked LiteLLM backend %q", want)
		}
	}

	img := string(dockerfile)
	for _, want := range []string{"go build -o /out/pokeissues ./cmd/pokeissues", "COPY --from=build /out/pokeissues /usr/local/bin/pokeissues"} {
		if !strings.Contains(img, want) {
			t.Errorf("farm image missing %q", want)
		}
	}
	if strings.Contains(img, "POKEPILOT_GITHUB_TOKEN") {
		t.Error("GitHub token must not be baked into the image")
	}
}
