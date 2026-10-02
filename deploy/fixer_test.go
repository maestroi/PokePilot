package deploy

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFixerLoopParses(t *testing.T) {
	cmd := exec.Command("bash", "-n", "fixer-loop.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

func TestFixerStackMountsROMAndKeepsItsOwnState(t *testing.T) {
	body, err := os.ReadFile("fixer.yml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"/rom:ro",
		"FIXER_STATE_DIR",
		"POKEPILOT_FIXER_CURSOR_API_KEY",
		"POKEPILOT_FIXER_GITHUB_TOKEN",
		"POKEPILOT_MCP_TOKEN",
		"node.labels.pokepilot.fixer == true",
		"POKEPILOT_CURSOR_MODEL",
		"pokepilot-fixer:local",
		// Replicas: own checkout per slot, one shared ledger/claims/qwen lock.
		"slot-{{.Task.Slot}}/state",
		"POKEPILOT_TRIAGE_LEDGER: /var/lib/pokefixer/ledger.tsv",
		"POKEPILOT_TRIAGE_CLAIMS: /var/lib/pokefixer/claims",
		"POKEPILOT_QWEN_LOCK: /var/lib/pokefixer/qwen.lock",
		"cursor/claude-opus",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("fixer.yml missing %q", want)
		}
	}
	if strings.Contains(s, "pokemon_red.gb:") || strings.Contains(s, "COPY roms") {
		t.Error("fixer stack must mount the ROM, not bake it")
	}
}

func TestFixerLoopUsesTokenNotSSH(t *testing.T) {
	body, err := os.ReadFile("fixer-loop.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"gh auth setup-git",
		"POKEMON_RED_ROM",
		"CURSOR_API_KEY",
		"qwagent-triage.sh",
		"https://github.com/",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("fixer-loop.sh missing %q", want)
		}
	}
}
