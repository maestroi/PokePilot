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

// watchHarness builds a PATH of mocks and returns a tick function plus the temp
// dir, so each check below can vary exactly one probe.
func watchHarness(t *testing.T, mocks map[string]string, extraEnv ...string) (func() string, string) {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range mocks {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tick := func() string {
		cmd := exec.Command("bash", "./farm-watch.sh")
		cmd.Env = append(append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"POKEPILOT_ENV=/dev/null",
			"POKEPILOT_WATCH_STATE="+filepath.Join(tmp, "state"),
			"POKEPILOT_TRIAGE_STATE="+filepath.Join(tmp, "triage"),
			"POKEPILOT_WALL_URL=http://wall.invalid",
			"POKEPILOT_WATCH_DIGEST_HOUR=99",
			"TELEGRAM_BOT_TOKEN=",
		), extraEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("farm-watch.sh: %v\n%s", err, out)
		}
		return string(out)
	}
	return tick, tmp
}

// healthyMocks keeps every probe but the one under test quiet.
func healthyMocks() map[string]string {
	return map[string]string{
		// curl is used for the dashboard fetch, the endpoint probe and (when
		// Telegram is configured) notify. Capture the args BEFORE shifting them
		// away, or a pattern match against "$*" sees an empty string.
		"curl": `#!/usr/bin/env bash
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
[ -n "$out" ] && [ "$out" != /dev/null ] && echo '{"runs":[]}' >"$out"
exit 0
`,
		"systemctl": `#!/usr/bin/env bash
case "$*" in *show*) echo success ;; esac
exit 0
`,
		"gh": "#!/usr/bin/env bash\necho '[]'\n",
		"df": `#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/root 100000000 10000000 90000000 10%% /\n'
`,
	}
}

// A manager filesystem with no room left must page even when every other probe
// is healthy: this is the failure that would otherwise take the farm down
// without a word.
func TestFarmWatchPagesOnLowManagerDisk(t *testing.T) {
	mocks := healthyMocks()
	mocks["ssh"] = `#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/sda2 51000000 50000000 100000 100%% /\n'
`
	tick, _ := watchHarness(t, mocks, "POKEPILOT_SWARM_MANAGER=root@manager.invalid")

	if out := tick(); strings.Contains(out, "free (under") {
		t.Fatalf("paged on the first bad tick (grace 2):\n%s", out)
	}
	out := tick()
	if !strings.Contains(out, "swarm manager manager.invalid has 0G free (under 20G)") {
		t.Fatalf("did not page on a full manager filesystem:\n%s", out)
	}
	if !strings.Contains(out, "pokefarm_postgres") {
		t.Fatalf("page did not say why the disk matters:\n%s", out)
	}
}

// The manager check is opt-in, and an unreachable manager must not be reported
// as a disk problem; the wall check owns that failure.
func TestFarmWatchSkipsManagerDiskWhenUnreachable(t *testing.T) {
	mocks := healthyMocks()
	mocks["ssh"] = `#!/usr/bin/env bash
exit 255
`
	tick, _ := watchHarness(t, mocks, "POKEPILOT_SWARM_MANAGER=root@manager.invalid")
	for i := 0; i < 3; i++ {
		if out := tick(); strings.Contains(out, "free (under") {
			t.Fatalf("paged for an unreachable manager:\n%s", out)
		}
	}
}

// The local filesystem pages without a grace tick, because df cannot flake.
func TestFarmWatchPagesOnLowLocalDisk(t *testing.T) {
	mocks := healthyMocks()
	mocks["df"] = `#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/nvme0n1p2 1900000000 1890000000 1000000 100%% /\n'
`
	tick, _ := watchHarness(t, mocks)
	if out := tick(); !strings.Contains(out, "has 0G free (under 20G)") {
		t.Fatalf("did not page on a full local filesystem:\n%s", out)
	}
}

// The threshold is configurable, so a host with a deliberately small root can
// raise it and stop the page.
func TestFarmWatchDiskThresholdIsConfigurable(t *testing.T) {
	mocks := healthyMocks()
	// 25G free: fine at the default 20G, bad once the operator asks for 50G.
	mocks["df"] = `#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/root 100000000 75000000 26214400 75%% /\n'
`
	tick, _ := watchHarness(t, mocks)
	if out := tick(); strings.Contains(out, "free (under") {
		t.Fatalf("paged above the default threshold:\n%s", out)
	}
	tick50, _ := watchHarness(t, mocks, "POKEPILOT_WATCH_MIN_FREE_GB=50")
	if out := tick50(); !strings.Contains(out, "has 25G free (under 50G)") {
		t.Fatalf("raised threshold did not page:\n%s", out)
	}
}

// A dead planner endpoint is the single point of failure for every campaign,
// and the watchdog only learns about it from the runs' own reported endpoint.
func TestFarmWatchPagesOnUnreachablePlannerEndpoint(t *testing.T) {
	mocks := healthyMocks()
	mocks["curl"] = `#!/usr/bin/env bash
args="$*"
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
case "$args" in
*192.168.50.130:8002/health*) exit 7 ;;
esac
[ -n "$out" ] && [ "$out" != /dev/null ] &&
	echo '{"runs":[{"status":"running","frame":1,"stats":{"endpoint":"http://192.168.50.130:8002"}}]}' >"$out"
exit 0
`
	tick, _ := watchHarness(t, mocks)
	if out := tick(); strings.Contains(out, "unreachable:") {
		t.Fatalf("paged on the first bad tick (grace 2):\n%s", out)
	}
	out := tick()
	if !strings.Contains(out, "planner endpoint unreachable: http://192.168.50.130:8002") {
		t.Fatalf("did not page on a dead planner endpoint:\n%s", out)
	}
	if !strings.Contains(out, "runs stall until it returns") {
		t.Fatalf("page did not explain the consequence:\n%s", out)
	}
}

// A healthy endpoint must stay quiet, including when runs report no endpoint at
// all (their stats may not be populated yet).
func TestFarmWatchStaysQuietOnHealthyPlannerEndpoint(t *testing.T) {
	mocks := healthyMocks()
	mocks["curl"] = `#!/usr/bin/env bash
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
[ -n "$out" ] && [ "$out" != /dev/null ] &&
	echo '{"runs":[{"status":"running","frame":1,"stats":{"endpoint":"http://192.168.50.130:8002"}},{"status":"leased","frame":0}]}' >"$out"
exit 0
`
	tick, _ := watchHarness(t, mocks)
	for i := 0; i < 3; i++ {
		if out := tick(); strings.Contains(out, "planner endpoint unreachable") {
			t.Fatalf("paged for a healthy endpoint:\n%s", out)
		}
	}
}
