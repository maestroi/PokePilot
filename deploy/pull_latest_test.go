package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullLatestBootstrapsRolloutFromPublishedDigest(t *testing.T) {
	tmp := t.TempDir()
	dockerLog := filepath.Join(tmp, "docker.log")
	rolloutLog := filepath.Join(tmp, "rollout.log")
	fakeRollout := filepath.Join(tmp, "rollout-latest.sh")
	fakeLitellm := filepath.Join(tmp, "litellm.yaml")

	if err := os.WriteFile(fakeRollout, []byte(`#!/usr/bin/env bash
set -euo pipefail
litellm=missing
if [ -f "$(dirname "$0")/litellm.yaml" ]; then
	litellm=present
fi
printf '%s|%s\n' "${FARM_IMAGE_DIGEST_REF:-}" "$litellm" > "$MOCK_ROLLOUT_LOG"
`), 0o755); err != nil {
		t.Fatalf("write fake rollout: %v", err)
	}
	if err := os.WriteFile(fakeLitellm, []byte("model_list: []\n"), 0o644); err != nil {
		t.Fatalf("write fake litellm: %v", err)
	}

	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"

if [ "$1" = "pull" ]; then
	exit 0
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
	echo 'ghcr.io/maestroi/pokepilot@sha256:new'
	exit 0
fi
if [ "$1" = "create" ]; then
	echo 'mock-container'
	exit 0
fi
if [ "$1" = "cp" ]; then
	mkdir -p "$3"
	cp "$MOCK_ROLLOUT_SOURCE" "$3/rollout-latest.sh"
	cp "$MOCK_LITELLM_SOURCE" "$3/litellm.yaml"
	exit 0
fi
if [ "$1" = "rm" ]; then
	exit 0
fi
exit 1
`
	if err := os.WriteFile(filepath.Join(tmp, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatalf("write docker mock: %v", err)
	}

	cmd := exec.Command("bash", "./pull-latest.sh")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DOCKER_LOG="+dockerLog,
		"MOCK_ROLLOUT_LOG="+rolloutLog,
		"MOCK_ROLLOUT_SOURCE="+fakeRollout,
		"MOCK_LITELLM_SOURCE="+fakeLitellm,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pull-latest.sh: %v\n%s", err, out)
	}

	gotRollout, err := os.ReadFile(rolloutLog)
	if err != nil {
		t.Fatalf("read rollout log: %v", err)
	}
	if got := strings.TrimSpace(string(gotRollout)); got != "ghcr.io/maestroi/pokepilot@sha256:new|present" {
		t.Fatalf("rollout received %q, want exact pulled digest and colocated litellm config", got)
	}

	gotDocker, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	logText := string(gotDocker)
	for _, want := range []string{
		"pull ghcr.io/maestroi/pokepilot:latest",
		"create ghcr.io/maestroi/pokepilot@sha256:new /bin/true",
		"cp mock-container:/usr/local/share/pokepilot/deploy/.",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("docker calls missing %q in:\n%s", want, logText)
		}
	}
}

func TestRolloutLatestForceRollsStaleRunningTask(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"

if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	svc="$3"
	if [ "$svc" = "pokefarm_litellm" ]; then
		exit 1
	fi
	case "$*" in
	*Spec.TaskTemplate.ContainerSpec.Image*)
		echo 'ghcr.io/maestroi/pokepilot@sha256:new'
		;;
	*UpdateStatus*)
		echo 'completed'
		;;
	esac
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "ps" ]; then
	svc="${!#}"
	if [ "$svc" = "pokefarm_runner" ]; then
		echo 'Running 2 hours ago|ghcr.io/maestroi/pokepilot@sha256:old'
	else
		echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:new'
	fi
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "update" ]; then
	exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(tmp, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatalf("write docker mock: %v", err)
	}

	cmd := exec.Command("bash", "./rollout-latest.sh")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DOCKER_LOG="+logPath,
		"FARM_IMAGE_DIGEST_REF=ghcr.io/maestroi/pokepilot@sha256:new",
		"FARM_REPLAY_HOST=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pokefarm_runner spec is sha256:new but 1/1 Running task(s) are stale; force-roll") {
		t.Fatalf("output did not identify stale runner:\n%s", out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	logText := string(logData)
	want := "service update --force --detach --with-registry-auth --image ghcr.io/maestroi/pokepilot@sha256:new pokefarm_runner"
	if !strings.Contains(logText, want) {
		t.Fatalf("docker calls did not force-roll stale runner; want %q in:\n%s", want, logText)
	}
	if strings.Contains(logText, "--force --detach --with-registry-auth --image ghcr.io/maestroi/pokepilot@sha256:new pokefarm_wall") {
		t.Fatalf("current wall was unnecessarily force-rolled:\n%s", logText)
	}
}

func TestFarmImageCarriesRolloutBundle(t *testing.T) {
	data, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"COPY deploy/rollout-latest.sh /usr/local/share/pokepilot/deploy/rollout-latest.sh",
		"COPY deploy/litellm.yaml /usr/local/share/pokepilot/deploy/litellm.yaml",
		"COPY deploy/replay-sidecar.sh /usr/local/share/pokepilot/deploy/replay-sidecar.sh",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile does not embed rollout bundle entry %q", want)
		}
	}
}

func TestPullLatestShellSyntax(t *testing.T) {
	for _, script := range []string{"./pull-latest.sh", "./rollout-latest.sh", "./replay-pull.sh", "./replay-sidecar.sh"} {
		cmd := exec.Command("bash", "-n", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash -n %s: %v\n%s", script, err, out)
		}
	}
}
