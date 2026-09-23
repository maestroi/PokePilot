package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mockDockerReplaySidecar stands in for the docker CLI so the real
// replay-sidecar.sh can be exercised without a daemon. It records every call,
// models "the container exists" as files under $MOCK_DIR, and answers the
// health probe the way a live sidecar would.
const mockDockerReplaySidecar = `#!/usr/bin/env bash
set -eu
state="$MOCK_DIR"
printf '%s\n' "$*" >> "$state/calls"
case "${1:-}" in
network)
	exit 0
	;;
image)
	case "$*" in
	*'--format {{.Id}}'*) echo 'sha256:newimage' ;;
	esac
	exit 0
	;;
inspect)
	case "$*" in
	*'{{.Image}}')
		[ -f "$state/image" ] || exit 1
		cat "$state/image"
		;;
	*'Config.Labels'*)
		[ -f "$state/spec" ] || exit 1
		cat "$state/spec"
		;;
	*)
		[ -f "$state/exists" ] || exit 1
		echo '{}'
		;;
	esac
	exit 0
	;;
run)
	printf '%s\n' "$*" >> "$state/runs"
	touch "$state/exists"
	echo 'sha256:newimage' > "$state/image"
	prev=""
	for arg in "$@"; do
		if [ "$prev" = "--label" ]; then
			case "$arg" in
			pokefarm.replay.spec=*) printf '%s' "${arg#pokefarm.replay.spec=}" > "$state/spec" ;;
			esac
		fi
		prev="$arg"
	done
	exit 0
	;;
exec)
	echo '{"status":"ok","s3_configured":true}'
	exit 0
	;;
rm)
	rm -f "$state/exists" "$state/image" "$state/spec"
	exit 0
	;;
esac
exit 0
`

type sidecarHarness struct {
	dir     string
	state   string
	envFile string
	rom     string
	render  string
	card    string
	env     []string
}

func newSidecarHarness(t *testing.T) *sidecarHarness {
	t.Helper()
	dir := t.TempDir()
	h := &sidecarHarness{
		dir:     dir,
		state:   filepath.Join(dir, "state"),
		envFile: filepath.Join(dir, "replay.env"),
		rom:     filepath.Join(dir, "pokemon_red.gb"),
		render:  filepath.Join(dir, "renderD128"),
		card:    filepath.Join(dir, "card1"),
	}
	bin := filepath.Join(dir, "bin")
	for _, d := range []string{h.state, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	writeTestFile(t, filepath.Join(bin, "docker"), mockDockerReplaySidecar, 0o755)
	writeTestFile(t, h.envFile, "POKEPILOT_REPLAY_ENCODER=vaapi\n", 0o644)
	writeTestFile(t, h.rom, "rom", 0o644)
	writeTestFile(t, h.render, "", 0o644)
	writeTestFile(t, h.card, "", 0o644)

	h.env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DIR="+h.state,
		"FARM_IMAGE=ghcr.io/maestroi/pokepilot:test",
		"FARM_REPLAY_ENV_FILE="+h.envFile,
		"FARM_REPLAY_ROM="+h.rom,
		"FARM_REPLAY_RENDER_DEVICE="+h.render,
		"FARM_REPLAY_CARD_DEVICE="+h.card,
		"FARM_REPLAY_NETWORK=pokefarm_gpu",
		"FARM_REPLAY_HEALTH_TIMEOUT=5",
	)
	return h
}

func writeTestFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// run executes the real script with an optional env override layered on top.
func (h *sidecarHarness) run(t *testing.T, overrides ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "./replay-sidecar.sh")
	cmd.Env = append(append([]string(nil), h.env...), overrides...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (h *sidecarHarness) runs(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.state, "runs"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read runs log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// The sidecar cannot be a Swarm service: Docker 28 drops `devices:` from a
// stack-deployed task (HostConfig.Devices=null) and `docker service create` has
// no --device flag, so the iGPU only reaches a standalone container. This test
// pins the whole definition that keeps the GPU usable.
func TestReplaySidecarReconcilesTheIgpuContainer(t *testing.T) {
	h := newSidecarHarness(t)

	out, err := h.run(t)
	if err != nil {
		t.Fatalf("first reconcile failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "healthy") {
		t.Fatalf("first reconcile did not verify the health postcondition:\n%s", out)
	}

	runs := h.runs(t)
	if len(runs) != 1 {
		t.Fatalf("want exactly 1 docker run, got %d:\n%s", len(runs), strings.Join(runs, "\n"))
	}
	run := runs[0]
	for _, want := range []string{
		"--restart unless-stopped",
		"--name pokefarm-replay",
		// The render node is the whole reason this container exists.
		"--device " + h.render,
		"--device " + h.card,
		"--group-add 44",
		"--group-add 993",
		// pokeui and the spectator resolve this name over the attachable overlay.
		"--network pokefarm_gpu",
		"--network-alias replay",
		"--env-file " + h.envFile,
		"--volume " + h.rom + ":/rom/pokemon_red.gb:ro",
		"ghcr.io/maestroi/pokepilot:test pokereplay -http :8080 -wall http://wall:8080 -rom /rom/pokemon_red.gb",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("docker run missing %q:\n%s", want, run)
		}
	}
	if strings.Contains(run, "--privileged") {
		t.Errorf("sidecar must not need --privileged:\n%s", run)
	}
}

func TestReplaySidecarIsIdempotentAndRecreatesOnChange(t *testing.T) {
	h := newSidecarHarness(t)
	if out, err := h.run(t); err != nil {
		t.Fatalf("first reconcile: %v\n%s", err, out)
	}

	// Same image and same definition: a no-op, so the timer cannot churn the
	// container (and drop the cached MP4 it serves) every two minutes.
	out, err := h.run(t)
	if err != nil {
		t.Fatalf("second reconcile: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already current") {
		t.Fatalf("second reconcile was not a no-op:\n%s", out)
	}
	if runs := h.runs(t); len(runs) != 1 {
		t.Fatalf("unchanged definition recreated the container: %d runs", len(runs))
	}

	// A definition edit (here the listen address) must be picked up, otherwise
	// a stale container would silently keep serving the old spec.
	out, err = h.run(t, "FARM_REPLAY_LISTEN=:9090")
	if err != nil {
		t.Fatalf("changed reconcile: %v\n%s", err, out)
	}
	if !strings.Contains(out, "recreating") {
		t.Fatalf("changed definition did not recreate the container:\n%s", out)
	}
	runs := h.runs(t)
	if len(runs) != 2 {
		t.Fatalf("want 2 docker runs after a definition change, got %d", len(runs))
	}
	if !strings.Contains(runs[1], "-http :9090") {
		t.Errorf("recreated container kept the old listen address:\n%s", runs[1])
	}
}

// The GPU is the reason this container is not a service, so a host without the
// render node must fail loudly instead of quietly encoding on the CPU (or
// worse, leaving pokeui with a `replay` name nothing answers).
func TestReplaySidecarRequiresTheRenderNodeAndImage(t *testing.T) {
	h := newSidecarHarness(t)

	out, err := h.run(t, "FARM_REPLAY_RENDER_DEVICE="+filepath.Join(h.dir, "absent"))
	if err == nil {
		t.Fatalf("missing render node did not fail:\n%s", out)
	}
	if !strings.Contains(out, "absent") || !strings.Contains(out, "iGPU node") {
		t.Errorf("missing render node produced an unhelpful error:\n%s", out)
	}
	if runs := h.runs(t); len(runs) != 0 {
		t.Fatalf("missing render node still started a container: %v", runs)
	}

	out, err = h.run(t, "FARM_IMAGE=")
	if err == nil {
		t.Fatalf("missing FARM_IMAGE did not fail:\n%s", out)
	}
	if !strings.Contains(out, "FARM_IMAGE") {
		t.Errorf("missing FARM_IMAGE produced an unhelpful error:\n%s", out)
	}
}

// The manager cannot roll the sidecar itself: it has no SSH trust into the iGPU
// worker (measured: root@192.168.50.151 -> root@192.168.50.68 is
// "Permission denied (publickey)"). The old rollout branch therefore never
// converged, which is how the sidecar drifted onto a hand-run image.
func TestReplaySidecarIsOwnedByTheWorkerNotTheManager(t *testing.T) {
	rollout := readDeployFile(t, "rollout-latest.sh")
	for _, dead := range []string{"ssh", "pokefarm-replay-up"} {
		if strings.Contains(rollout, dead) {
			t.Errorf("rollout-latest.sh still reaches for the sidecar over the network (%q)", dead)
		}
	}
	services := regexp.MustCompile(`(?m)^SERVICES=\(([^)]*)\)`).FindStringSubmatch(rollout)
	if services == nil {
		t.Fatal("rollout-latest.sh has no SERVICES list")
	}
	// A stack service cannot be given the device, and a software replica would
	// steal the `replay` DNS name from the real sidecar.
	if strings.Contains(services[1], "replay") {
		t.Errorf("replay must not be a stack service; SERVICES=(%s)", services[1])
	}

	service := readDeployFile(t, "pokefarm-replay-pull.service")
	if !strings.Contains(service, "ExecStart=/usr/local/sbin/pokefarm-replay-pull") {
		t.Error("pokefarm-replay-pull.service must run the worker bootstrap")
	}
	timer := readDeployFile(t, "pokefarm-replay-pull.timer")
	if !strings.Contains(timer, "WantedBy=timers.target") {
		t.Error("pokefarm-replay-pull.timer must install itself")
	}

	// The bootstrap must execute the definition extracted from the image, so
	// the container spec versions with the image instead of a host copy.
	pull := readDeployFile(t, "replay-pull.sh")
	for _, want := range []string{
		"BUNDLE_PATH=/usr/local/share/pokepilot/deploy",
		"SIDECAR=replay-sidecar.sh",
		`FARM_IMAGE="$DIGEST_REF" "$tmpdir/${SIDECAR}"`,
	} {
		if !strings.Contains(pull, want) {
			t.Errorf("replay-pull.sh missing %q", want)
		}
	}
}

func readDeployFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
