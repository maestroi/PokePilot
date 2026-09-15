package farm

import (
	"strings"
	"testing"
)

func TestIsPostgresRegistrySource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		want   bool
	}{
		{name: "postgres", source: "postgres://pokepilot:secret@postgres:5432/pokepilot", want: true},
		{name: "postgresql", source: " postgresql://db.example/pokepilot ", want: true},
		{name: "json file", source: "/etc/pokepilot/models.json", want: false},
		{name: "empty", source: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isPostgresRegistrySource(tc.source); got != tc.want {
				t.Fatalf("isPostgresRegistrySource(%q) = %v, want %v", tc.source, got, tc.want)
			}
		})
	}
}

func TestLoadModelRegistryPostgresEnvRequiresConfiguredDSN(t *testing.T) {
	const envName = "POKEPILOT_TEST_DATABASE_URL"
	t.Setenv(envName, "")
	_, err := LoadModelRegistry(postgresRegistryEnvPrefix + envName)
	if err == nil {
		t.Fatal("expected missing database URL error")
	}
	if !strings.Contains(err.Error(), envName) {
		t.Fatalf("error %q does not identify the missing environment variable", err)
	}
}
