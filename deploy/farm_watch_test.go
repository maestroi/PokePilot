package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The watchdog pages on the GRACE-th consecutive bad tick, stays quiet while
// the same problem persists, and sends one recovery message.
func TestFarmWatchNotifiesOnceThenRecovers(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	mocks := map[string]string{
		// The wall is down while $tmp/wall-down exists.
		"curl": `#!/usr/bin/env bash
[ -f "$MOCK_DIR/wall-down" ] && exit 7
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
echo '{"runs":[{"status":"running","frame":1}]}' >"$out"
`,
		"systemctl": `#!/usr/bin/env bash
case "$*" in *show*) echo success ;; esac
exit 0
`,
		"gh": "#!/usr/bin/env bash\necho '[]'\n",
	}
	for name, body := range mocks {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "wall-down"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tick := func() string {
		cmd := exec.Command("bash", "./farm-watch.sh")
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"MOCK_DIR="+tmp,
			"POKEPILOT_ENV=/dev/null",
			"POKEPILOT_WATCH_STATE="+filepath.Join(tmp, "state"),
			"POKEPILOT_TRIAGE_STATE="+filepath.Join(tmp, "triage"),
			"POKEPILOT_WALL_URL=http://wall.invalid",
			"POKEPILOT_WATCH_DIGEST_HOUR=99",
			"TELEGRAM_BOT_TOKEN=",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("farm-watch.sh: %v\n%s", err, out)
		}
		return string(out)
	}
	if out := tick(); strings.Contains(out, "unreachable") {
		t.Fatalf("paged on the first bad tick (grace 2):\n%s", out)
	}
	if out := tick(); !strings.Contains(out, "🔴 PokePilot: wall http://wall.invalid unreachable") {
		t.Fatalf("did not page after grace:\n%s", out)
	}
	if out := tick(); strings.Contains(out, "unreachable") {
		t.Fatalf("re-paged before the 12h reminder:\n%s", out)
	}
	if err := os.Remove(filepath.Join(tmp, "wall-down")); err != nil {
		t.Fatal(err)
	}
	if out := tick(); !strings.Contains(out, "✅ PokePilot recovered: wall") {
		t.Fatalf("no recovery message:\n%s", out)
	}
}

// Three rollbacks freeze only the deploy timer; the freeze lifts on its own.
func TestFarmWatchFreezesDeploysAfterRepeatedRollbacks(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	mocks := map[string]string{
		"curl": `#!/usr/bin/env bash
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
echo '{"runs":[{"status":"running","frame":1}]}' >"$out"
`,
		"systemctl": "#!/usr/bin/env bash\necho success\n",
		"gh":        "#!/usr/bin/env bash\necho '[]'\n",
		// docker service inspect reports $MOCK_DIR/status; systemctl is logged.
		"ssh": `#!/usr/bin/env bash
case "$*" in
*systemctl*) echo "$*" >>"$MOCK_DIR/ssh.log" ;;
*) cat "$MOCK_DIR/status" ;;
esac
`,
	}
	for name, body := range mocks {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(tmp, "state")
	tick := func(completed string) {
		if err := os.WriteFile(filepath.Join(tmp, "status"), []byte("pokefarm_runner rollback_completed "+completed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", "./farm-watch.sh")
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"MOCK_DIR="+tmp,
			"POKEPILOT_ENV=/dev/null",
			"POKEPILOT_WATCH_STATE="+state,
			"POKEPILOT_TRIAGE_STATE="+filepath.Join(tmp, "triage"),
			"POKEPILOT_WALL_URL=http://wall.invalid",
			"POKEPILOT_SWARM_MANAGER=manager.invalid",
			"POKEPILOT_WATCH_DIGEST_HOUR=99",
			"TELEGRAM_BOT_TOKEN=",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("farm-watch.sh: %v\n%s", err, out)
		}
	}
	sshLog := func() string {
		b, _ := os.ReadFile(filepath.Join(tmp, "ssh.log"))
		return string(b)
	}
	tick("t1")
	tick("t1") // same completion time is one event, not two
	tick("t2")
	if strings.Contains(sshLog(), "systemctl") {
		t.Fatalf("froze deploys after only two rollbacks:\n%s", sshLog())
	}
	tick("t3")
	if !strings.Contains(sshLog(), "systemctl stop pokefarm-pull.timer") {
		t.Fatalf("did not freeze deploys after three rollbacks:\n%s", sshLog())
	}
	// Backdate the freeze past its window: deploys come back.
	if err := os.WriteFile(filepath.Join(state, "deploy-frozen"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tick("t3")
	if !strings.Contains(sshLog(), "systemctl start pokefarm-pull.timer") {
		t.Fatalf("did not resume deploys after the freeze:\n%s", sshLog())
	}
}
