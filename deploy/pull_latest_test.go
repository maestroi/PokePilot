package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullLatestForceRollsStaleRunningTask(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
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

	cmd := exec.Command("bash", "./pull-latest.sh")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DOCKER_LOG="+logPath,
		"FARM_REPLAY_HOST=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pull-latest.sh: %v\n%s", err, out)
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

func TestPullLatestShellSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", "./pull-latest.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n pull-latest.sh: %v\n%s", err, out)
	}
}
