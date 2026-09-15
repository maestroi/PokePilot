package farm

import "testing"

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
