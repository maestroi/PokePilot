package deploy_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	want := "service update --force --detach --with-registry-auth --image ghcr.io/maestroi/pokepilot@sha256:new --update-failure-action rollback --update-monitor 60s --stop-grace-period 6m --update-order start-first --update-parallelism 0 pokefarm_runner"
	if !strings.Contains(logText, want) {
		t.Fatalf("docker calls did not force-roll stale runner; want %q in:\n%s", want, logText)
	}
	if strings.Contains(logText, "--force --detach --with-registry-auth --image ghcr.io/maestroi/pokepilot@sha256:new pokefarm_wall") {
		t.Fatalf("current wall was unnecessarily force-rolled:\n%s", logText)
	}
}

func TestRolloutLatestReusesExistingLitellmConfig(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	// Every farm service is absent; litellm runs on a hand-named config while
	// the content-addressed config already exists (docker config create would
	// fail with AlreadyExists).
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	case "$3" in
	pokefarm_wall|pokefarm_litellm) ;;
	*) exit 1 ;;
	esac
	case "$*" in
	*ConfigName*) echo 'pokefarm_litellm_yaml_v3' ;;
	*File.Name*) echo '/app/config.yaml' ;;
	esac
	exit 0
fi
if [ "$1" = "config" ] && [ "$2" = "create" ]; then
	echo 'config already exists' >&2
	exit 1
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
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	if strings.Contains(string(logData), "config create") {
		t.Fatalf("recreated an existing litellm config:\n%s", logData)
	}
	if !strings.Contains(string(logData), "--config-rm pokefarm_litellm_yaml_v3") {
		t.Fatalf("litellm was not moved onto the existing config:\n%s", logData)
	}
}

// A worker node that is Down (or a manager the manager cannot reach) keeps
// reporting its tasks as Running at their old image forever: Swarm has no agent
// left to stop them. Counting those tasks as stale made every two-minute timer
// tick force-roll the whole runner fleet, which restarted runners and destroyed
// their in-flight runs indefinitely — a rollout that can never converge. They
// must be ignored, while a genuinely stale task on a reachable node still rolls.
func TestRolloutLatestIgnoresTasksOnNodesSwarmCannotActOn(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"

if [ "$1" = "node" ] && [ "$2" = "ls" ]; then
	echo 'vm-swarm-worker-02|Ready|'
	echo 'vm-swarm-worker-04|Ready|Unreachable'
	echo 'vm-swarm-worker-05|Down|'
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	# litellm is absent, so the script skips its config reconciliation entirely.
	if [ "$3" = "pokefarm_litellm" ]; then
		exit 1
	fi
	case "$*" in
	*Spec.TaskTemplate.ContainerSpec.Image*) echo 'ghcr.io/maestroi/pokepilot@sha256:new' ;;
	*UpdateStatus*) echo 'completed' ;;
	esac
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "ps" ]; then
	svc="${!#}"
	case "$svc" in
	pokefarm_runner)
		# Three tasks stranded on a Down node and one on an unreachable manager,
		# all at the old digest, plus one healthy current task.
		echo 'Running 4 hours ago|ghcr.io/maestroi/pokepilot@sha256:old|vm-swarm-worker-05'
		echo 'Running 4 hours ago|ghcr.io/maestroi/pokepilot@sha256:old|vm-swarm-worker-05'
		echo 'Running 4 hours ago|ghcr.io/maestroi/pokepilot@sha256:old|vm-swarm-worker-05'
		echo 'Running 4 hours ago|ghcr.io/maestroi/pokepilot@sha256:old|vm-swarm-worker-04'
		echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:new|vm-swarm-worker-02'
		;;
	pokefarm_ui)
		# A genuinely stale task on a reachable node is still a stuck rollout.
		echo 'Running 5 minutes ago|ghcr.io/maestroi/pokepilot@sha256:old|vm-swarm-worker-02'
		;;
	*)
		echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:new|vm-swarm-worker-02'
		;;
	esac
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
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}

	// Positive postcondition: the four tasks the manager cannot stop are
	// reported as stranded, not stale, so the healthy runner fleet is left alone.
	for _, want := range []string{
		"pokefarm_runner has 4 Running task(s) on node(s) Swarm cannot act on; not counting them as stale",
		"pokefarm_runner already sha256:new (1 running task(s))",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	logText := string(logData)
	if strings.Contains(logText, "--update-parallelism 0 pokefarm_runner") {
		t.Fatalf("runner was force-rolled for tasks Swarm cannot stop:\n%s", logText)
	}
	// The other half: a stale task on a reachable node still forces a roll.
	if !strings.Contains(string(out), "pokefarm_ui spec is sha256:new but 1/1 Running task(s) are stale; force-roll") {
		t.Fatalf("a reachable stale task no longer rolls:\n%s", out)
	}
	if !strings.Contains(logText, "--force --detach --with-registry-auth --image ghcr.io/maestroi/pokepilot@sha256:new --update-failure-action rollback --update-monitor 60s pokefarm_ui") {
		t.Fatalf("ui was not force-rolled for a reachable stale task:\n%s", logText)
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

// Swarm rolled the wall back from the published digest because its new task
// crash-looped inside the update monitor. The timer must not re-roll that same
// digest every two minutes; it waits for a newer image.
func TestRolloutLatestHoldsDigestSwarmRolledBack(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	case "$*" in
	*PreviousSpec*)
		if [ "$3" = "pokefarm_wall" ]; then
			echo 'rollback_completed|ghcr.io/maestroi/pokepilot@sha256:new'
		else
			echo 'completed|ghcr.io/maestroi/pokepilot@sha256:old'
		fi
		;;
	*Spec.TaskTemplate.ContainerSpec.Image*)
		echo 'ghcr.io/maestroi/pokepilot@sha256:old'
		;;
	esac
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
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "rolled pokefarm_wall back from sha256:new") {
		t.Fatalf("rollback hold not reported:\n%s", out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	if strings.Contains(string(logData), "service update") {
		t.Fatalf("re-rolled a digest Swarm already rejected:\n%s", logData)
	}
}

// With start-first and a 6m drain window, replaced runners stay Running at
// the old digest while Swarm wants them Shutdown. They must not count as
// stale: force-rolling for them restarted every fresh runner each tick.
func TestRolloutLatestIgnoresDrainingTasks(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	case "$*" in
	*PreviousSpec*) echo 'completed|ghcr.io/maestroi/pokepilot@sha256:old' ;;
	*Spec.TaskTemplate.ContainerSpec.Image*) echo 'ghcr.io/maestroi/pokepilot@sha256:new' ;;
	*UpdateStatus*) echo 'completed' ;;
	esac
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "ps" ]; then
	echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:new|worker-1|Running'
	echo 'Running 9 minutes ago|ghcr.io/maestroi/pokepilot@sha256:old|worker-2|Shutdown'
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
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	if strings.Contains(string(logData), "--image") {
		t.Fatalf("force-rolled for a draining task:\n%s\n%s", out, logData)
	}
}

// TestRolloutLatestPrunesUntaggedImagesButKeepsReferencedDigests pins the
// reclaim the manager relies on to survive unattended weeks. Every merge
// publishes a digest that the timer pulls within ~2 minutes; without a prune
// the untagged predecessors accumulate until the filesystem holding the Swarm
// control plane and pokefarm_postgres is full.
//
// The keep-set must include the rollback target (PreviousSpec) and any digest a
// live task still runs, because dropping those forces `docker service rollback`
// to re-pull from the registry.
func TestRolloutLatestPrunesUntaggedImagesButKeepsReferencedDigests(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	mockDocker := `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"

if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	if [ "$3" = "pokefarm_litellm" ]; then
		exit 1
	fi
	case "$*" in
	# Keep-set query passes both in one template; the rollout loop passes only
	# the spec image and the rollback-hold check passes only PreviousSpec.
	*ContainerSpec.Image*PreviousSpec*) echo 'ghcr.io/maestroi/pokepilot@sha256:live'; echo 'ghcr.io/maestroi/pokepilot@sha256:prev' ;;
	*ContainerSpec.Image*) echo 'ghcr.io/maestroi/pokepilot@sha256:live' ;;
	*PreviousSpec*) echo 'completed|ghcr.io/maestroi/pokepilot@sha256:live' ;;
	*UpdateStatus*) echo 'completed' ;;
	esac
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "ps" ]; then
	case "$*" in
	*'{{.Image}}'*) echo 'ghcr.io/maestroi/pokepilot@sha256:task' ;;
	*) echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:live|worker-1|Running' ;;
	esac
	exit 0
fi
if [ "$1" = "images" ]; then
	echo 'sha256:aaa <none>'
	echo 'sha256:bbb <none>'
	echo 'sha256:ccc <none>'
	echo 'sha256:ddd latest'
	exit 0
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
	case "$3" in
	sha256:aaa) echo 'ghcr.io/maestroi/pokepilot@sha256:live' ;;
	sha256:bbb) echo 'ghcr.io/maestroi/pokepilot@sha256:prev' ;;
	sha256:ccc) echo 'ghcr.io/maestroi/pokepilot@sha256:orphan' ;;
	esac
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
		"FARM_IMAGE_DIGEST_REF=ghcr.io/maestroi/pokepilot@sha256:live",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read docker log: %v", err)
	}
	logText := string(logData)

	if !strings.Contains(string(out), "pruned 1 untagged ghcr.io/maestroi/pokepilot image(s)") {
		t.Fatalf("did not report a prune:\n%s", out)
	}
	if !strings.Contains(logText, "rmi sha256:ccc") {
		t.Fatalf("unreferenced untagged image was not reclaimed:\n%s", logText)
	}
	for _, keep := range []string{"sha256:aaa", "sha256:bbb"} {
		if strings.Contains(logText, "rmi "+keep) {
			t.Fatalf("removed referenced image %s (rollback target or live task):\n%s", keep, logText)
		}
	}
	if strings.Contains(logText, "rmi sha256:ddd") {
		t.Fatalf("removed the tagged :latest image:\n%s", logText)
	}
}

// TestRolloutLatestSurvivesAnEmptyImageList guards the prune against turning a
// quiet registry into a failed timer tick: with nothing to reclaim the script
// must still exit 0.
func TestRolloutLatestSurvivesAnEmptyImageList(t *testing.T) {
	tmp := t.TempDir()
	mockDocker := `#!/usr/bin/env bash
set -eu
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	if [ "$3" = "pokefarm_litellm" ]; then
		exit 1
	fi
	case "$*" in
	*ContainerSpec.Image*PreviousSpec*) echo 'ghcr.io/maestroi/pokepilot@sha256:live'; echo 'ghcr.io/maestroi/pokepilot@sha256:prev' ;;
	*ContainerSpec.Image*) echo 'ghcr.io/maestroi/pokepilot@sha256:live' ;;
	*PreviousSpec*) echo 'completed|ghcr.io/maestroi/pokepilot@sha256:live' ;;
	*UpdateStatus*) echo 'completed' ;;
	esac
	exit 0
fi
if [ "$1" = "service" ] && [ "$2" = "ps" ]; then
	case "$*" in
	*'{{.Image}}'*) ;;
	*) echo 'Running 1 minute ago|ghcr.io/maestroi/pokepilot@sha256:live|worker-1|Running' ;;
	esac
	exit 0
fi
if [ "$1" = "images" ]; then
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
		"FARM_IMAGE_DIGEST_REF=ghcr.io/maestroi/pokepilot@sha256:live",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "pruned") {
		t.Fatalf("reported a prune with no candidates:\n%s", out)
	}
}

func TestRolloutLatestSkipsWhileDeploysAreFrozen(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")
	until := time.Now().Add(time.Hour).Unix()
	mockDocker := fmt.Sprintf(`#!/usr/bin/env bash
set -eu
printf '%%s\n' "$*" >> "$MOCK_DOCKER_LOG"
if [ "$1" = "service" ] && [ "$2" = "inspect" ]; then
	case "$*" in
	*deploy-frozen-until*) echo '%d' ;;
	esac
	exit 0
fi
exit 0
`, until)
	if err := os.WriteFile(filepath.Join(tmp, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "./rollout-latest.sh")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_DOCKER_LOG="+logPath,
		"FARM_IMAGE_DIGEST_REF=ghcr.io/maestroi/pokepilot@sha256:new",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rollout-latest.sh: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "deploys frozen until") {
		t.Fatalf("missing freeze message:\n%s", out)
	}
	logData, _ := os.ReadFile(logPath)
	if strings.Contains(string(logData), "service update") {
		t.Fatalf("frozen rollout must not update services:\n%s", logData)
	}
}
