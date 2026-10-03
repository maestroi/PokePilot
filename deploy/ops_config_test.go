package deploy

import (
	"os"
	"strings"
	"testing"
)

// ops.yml must keep the bot unprivileged and give the socket only to watch.
func TestOpsStackPrivilegeBoundaries(t *testing.T) {
	raw, err := os.ReadFile("ops.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	// A service starts at a line indented exactly two spaces.
	var parts []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 2 && strings.HasPrefix(line, "  ") && line[2] != ' ' && line[2] != '#' || len(parts) == 0 {
			parts = append(parts, "")
			line = strings.TrimPrefix(line, "  ")
		}
		parts[len(parts)-1] += line + "\n"
	}
	section := func(name string) string {
		for _, p := range parts {
			if strings.HasPrefix(p, name+":") {
				return p
			}
		}
		t.Fatalf("service %s missing", name)
		return ""
	}
	if strings.Contains(section("telegram"), "docker.sock") || strings.Contains(section("telegram"), "/host") {
		t.Fatal("telegram must not mount the Docker socket or the host")
	}
	if !strings.Contains(section("watch"), "/var/run/docker.sock") || !strings.Contains(section("watch"), "node.role == manager") {
		t.Fatal("watch needs the socket and a manager placement")
	}
	n := section("node")
	if !strings.Contains(n, "mode: global") || !strings.Contains(n, "/usr/share:/host/rootfs:ro") || !strings.Contains(n, "/opt:/host/opt:ro") {
		t.Fatal("node must be global with narrow read-only host mounts")
	}
	// The whole host root would expose /run/docker.sock and host secrets to a
	// root process.
	if strings.Contains(n, "- /:") || strings.Contains(n, "docker.sock") {
		t.Fatal("node must not mount the host root or the Docker socket")
	}
	w := section("watch")
	if !strings.Contains(w, "POKEWATCH_FREEZE_SERVICE: ${POKEWATCH_FREEZE_SERVICE:-pokefarm-ops_watch}") {
		t.Fatal("watch must label its own service for the deploy freeze")
	}
	if !strings.Contains(text, "pokefarm_ops_token") {
		t.Fatal("shared ops token secret missing")
	}
}
