package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestComposeCanonicalHosts(t *testing.T) {
	files := []string{"farm.yml", "postgres.yml"}
	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		s := string(body)
		if strings.Contains(s, "pokemon.maestroi.cc") {
			t.Errorf("%s still treats pokemon.maestroi.cc as a configured host", name)
		}
	}

	postgres, err := os.ReadFile("postgres.yml")
	if err != nil {
		t.Fatalf("read postgres.yml: %v", err)
	}
	s := string(postgres)
	for _, want := range []string{
		"POKEPILOT_PUBLIC_BASE_URL: ${POKEPILOT_PUBLIC_BASE_URL:-https://rompilot.app}",
		"POKEPILOT_ADMIN_BASE_URL: ${POKEPILOT_ADMIN_BASE_URL:-https://admin.rompilot.app}",
		"POKEPILOT_API_BASE_URL: ${POKEPILOT_API_BASE_URL:-https://api.rompilot.app}",
		"POKEPILOT_SPECTATOR_URL: ${POKEPILOT_SPECTATOR_URL:-https://rompilot.app}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("postgres.yml missing %q", want)
		}
	}
}
