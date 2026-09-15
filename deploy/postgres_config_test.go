package deploy

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
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("postgres.yml must contain %q", required)
		}
	}
	if strings.Contains(text, "5432:") || strings.Contains(text, "published: 5432") {
		t.Fatal("postgres.yml must not publish the database port")
	}
}
