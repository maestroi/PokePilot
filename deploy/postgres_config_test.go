package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestPostgresControlPlaneIsPinnedToDurableNode(t *testing.T) {
	data, err := os.ReadFile("postgres.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"replicas: 1",
		"node.labels.pokepilot.db == true",
		"${FARM_DB_DIR:?FARM_DB_DIR is required}:/var/lib/postgresql",
		"PGDATA: /var/lib/postgresql/18/docker",
		"POKEPILOT_MODEL_REGISTRY: postgres-env://POKEPILOT_DATABASE_URL",
		"POKEPILOT_DATABASE_URL: ${POKEPILOT_DATABASE_URL:?POKEPILOT_DATABASE_URL is required}",
		"pg_isready -U pokepilot -d pokepilot",
		"- /tmp/pokewall",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("postgres.yml must contain %q", required)
		}
	}
	if strings.Contains(text, "5432:") || strings.Contains(text, "published: 5432") {
		t.Fatal("postgres.yml must not publish the database port")
	}
	// The overlay's wall command is the production cutover boundary. Legacy
	// SQLite/state files may still exist for local tests, but must not be part of
	// the Postgres deployment command.
	wall := text[strings.Index(text, "  wall:"):]
	if strings.Contains(wall, "- -state") || strings.Contains(wall, "- -catalog") || strings.Contains(wall, "catalog.db") || strings.Contains(wall, "state.json") {
		t.Fatal("PostgreSQL overlay must not configure legacy wall persistence")
	}
}
