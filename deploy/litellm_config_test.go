package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestLiteLLMRoutingConfig(t *testing.T) {
	data, err := os.ReadFile("litellm.yaml")
	if err != nil {
		t.Fatalf("read litellm.yaml: %v", err)
	}
	s := string(data)
	for _, want := range []string{
		"model_name: pokepilot-auto",
		"model_name: pokepilot-7900xtx",
		"model_name: pokepilot-4090",
		"model_name: pokepilot-lan",
		"- pokepilot-auto: [pokepilot-4090, pokepilot-lan]",
		"POKEPILOT_LITELLM_7900_URL",
		"POKEPILOT_LITELLM_4090_URL",
		"POKEPILOT_LITELLM_LAN_URL",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("litellm.yaml missing %q", want)
		}
	}
	for _, forbidden := range []string{"192.168.50.130", "192.168.50.81", "192.168.50.204"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("litellm.yaml must keep physical addresses in deployment env, found %q", forbidden)
		}
	}
}
