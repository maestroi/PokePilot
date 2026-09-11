# Qwagent Farm-Triage Loop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Install an optional 30-minute user systemd timer that picks one unused PokeFarm triage key and runs local `qwagent` (`opencode run --auto --model qwen3.8-27b/qwen3.8-27b`) to reproduce, fix, and open a PR.

**Architecture:** Hybrid picker + agent. Go code in `deploy/` decides the next actionable unused key from `GET /v1/triage` plus `gh pr list`. A bash oneshot owns flock, worktree reset, claim, and launch. OpenCode never chooses the queue item. The timer is off until `qwtriage-on`. See `docs/plans/2026-09-11-qwagent-triage-design.md`.

**Tech Stack:** Go 1.26 (`deploy` package + `cmd/qwagent-triage`), bash, systemd user units, `opencode run --auto`, `gh`, existing wall `/v1/triage`.

**Skills:** @pokefarm-triage @docs/ARCHITECTURE.md

Do not commit unless the human asks. Leave the tree dirty after each task.

---

### Task 1: Picker types and first failing test

**Files:**
- Create: `deploy/qwagent_triage.go`
- Create: `deploy/qwagent_triage_test.go`

**Step 1: Write the failing test**

`GET /v1/triage` returns a JSON array of `triageGroup` (see `cmd/pokewall/wall.go`). Copy the actionable rule from `cmd/pokeui/mcp.go` `triageGroupActionable`: active statuses (`open`, `reopened`, `investigating`, `in_progress`, `in-progress`, `todo`, `backlog`) stay actionable even with a stale resolution; a non-empty resolution or a resolved/closed/fixed/done/completed status is not.

```go
package deploy

import (
	"encoding/json"
	"testing"
)

func TestPickSkipsResolvedAndClaimed(t *testing.T) {
	raw := `[
	  {"key":"deadbeef","count":9,"example":"old bug","run_ids":["run-old"],
	   "issue":{"status":"resolved","resolution":"fixed"}},
	  {"key":"cafef00d","count":4,"example":"claimed bug","run_ids":["run-claimed"],
	   "issue":{"status":"open"}},
	  {"key":"0badf00d","count":3,"example":"free bug","run_ids":["run-free"],
	   "issue":{"status":"open"}}
	]`
	var groups []TriageGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		t.Fatal(err)
	}
	got, ok := Pick(groups, []string{
		"fix(farm): claimed bug [triage:cafef00d]",
	})
	if !ok {
		t.Fatal("expected a pick")
	}
	if got.Key != "0badf00d" {
		t.Fatalf("key = %q, want 0badf00d", got.Key)
	}
	if got.RunID() != "run-free" {
		t.Fatalf("run = %q, want run-free", got.RunID())
	}
}
```

Also add `TestPickEmptyWhenNothingFree` that returns `ok == false` when every group is resolved or claimed.

**Step 2: Run test to verify it fails**

```sh
POKEMON_RED_ROM= go test -short -count=1 ./deploy -run 'TestPick' -v
```

Expected: FAIL — `TriageGroup` / `Pick` undefined.

**Step 3: Write minimal implementation**

```go
package deploy

import "strings"

type TriageIssue struct {
	Status          string `json:"status"`
	Resolution      string `json:"resolution"`
	OccurrenceCount int64  `json:"occurrence_count"`
	FixedRevision   string `json:"fixed_revision"`
}

type TriageGroup struct {
	Pattern     string       `json:"pattern"`
	Key         string       `json:"key"`
	Fingerprint string       `json:"fingerprint"`
	Count       int          `json:"count"`
	Example     string       `json:"example"`
	RunIDs      []string     `json:"run_ids"`
	Issue       *TriageIssue `json:"issue,omitempty"`
}

func (g TriageGroup) RunID() string {
	if len(g.RunIDs) == 0 {
		return ""
	}
	return g.RunIDs[0]
}

func Actionable(g TriageGroup) bool {
	if g.Issue == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(g.Issue.Status))
	resolution := strings.ToLower(strings.TrimSpace(g.Issue.Resolution))
	switch status {
	case "open", "reopened", "investigating", "in_progress", "in-progress", "todo", "backlog":
		return true
	}
	if resolution != "" {
		return false
	}
	switch status {
	case "resolved", "closed", "fixed", "done", "completed":
		return false
	}
	return true
}

func Claimed(key string, prTitles []string) bool {
	if key == "" {
		return false
	}
	marker := "[triage:" + key + "]"
	for _, title := range prTitles {
		if strings.Contains(title, marker) {
			return true
		}
	}
	return false
}

func Pick(groups []TriageGroup, prTitles []string) (TriageGroup, bool) {
	best := TriageGroup{}
	found := false
	for _, g := range groups {
		if strings.TrimSpace(g.Key) == "" || !Actionable(g) || Claimed(g.Key, prTitles) {
			continue
		}
		if !found || g.Count > best.Count {
			best = g
			found = true
		}
	}
	return best, found
}
```

**Step 4: Run the tests and make sure they pass**

```sh
POKEMON_RED_ROM= go test -short -count=1 ./deploy -run 'TestPick' -v
```

Expected: PASS.

**Step 5: Do not commit** unless the human asks.

---

### Task 2: CLI `pick` / `claimed`

**Files:**
- Create: `cmd/qwagent-triage/main.go`
- Create: `cmd/qwagent-triage/main_test.go`

**Step 1: Write the failing test**

`pick` reads triage JSON on stdin and `--claimed` titles (one per line or repeated flags), writes one JSON object `{key, run_id, example, count, fingerprint}` or exits 2 when nothing is free.

```go
func TestPickCLI(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "pick", "--claimed", "fix(farm): x [triage:cafef00d]")
	cmd.Dir = "."
	cmd.Stdin = strings.NewReader(`[
	  {"key":"cafef00d","count":4,"example":"claimed","run_ids":["r1"],"issue":{"status":"open"}},
	  {"key":"0badf00d","count":3,"example":"free","run_ids":["r2"],"issue":{"status":"open"}}
	]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"key": "0badf00d"`) && !strings.Contains(string(out), `"key":"0badf00d"`) {
		t.Fatalf("output = %s", out)
	}
}
```

**Step 2: Run it — expect FAIL** (`package main` missing or subcommand unknown).

```sh
POKEMON_RED_ROM= go test -short -count=1 ./cmd/qwagent-triage -v
```

**Step 3: Implement `cmd/qwagent-triage/main.go`**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/deploy"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: qwagent-triage pick [--claimed title]...")
	}
	switch args[0] {
	case "pick":
		return pickCmd(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func pickCmd(args []string) error {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	var claimed repeatFlags
	fs.Var(&claimed, "claimed", "open PR title already claiming a triage key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var groups []deploy.TriageGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		return err
	}
	g, ok := deploy.Pick(groups, claimed)
	if !ok {
		os.Exit(2)
	}
	out := map[string]any{
		"key":         g.Key,
		"run_id":      g.RunID(),
		"example":     g.Example,
		"count":       g.Count,
		"fingerprint": g.Fingerprint,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

type repeatFlags []string

func (r *repeatFlags) String() string { return strings.Join(*r, ", ") }
func (r *repeatFlags) Set(v string) error {
	*r = append(*r, v)
	return nil
}
```

Note: `os.Exit(2)` inside `pickCmd` is awkward for tests. Prefer `return errNothing` and have `main` map it to exit 2:

```go
var errNothing = fmt.Errorf("no actionable unclaimed triage group")
```

In tests, call `run` directly rather than `go run` if that is cleaner — either is fine as long as exit 2 is the script contract.

**Step 4: Tests pass**

```sh
POKEMON_RED_ROM= go test -short -count=1 ./deploy ./cmd/qwagent-triage
```

---

### Task 3: Oneshot script (`--dry-run` first)

**Files:**
- Create: `deploy/qwagent-triage.sh` (executable)
- Modify: `deploy/qwagent_triage_test.go` — optional script smoke that `--dry-run` exists; prefer not to hit the network.

**Contract:**

```sh
# env
POKEPILOT_ROOT          # repo that contains cmd/qwagent-triage (the dedicated worktree)
POKEPILOT_TRIAGE_MAIN   # source checkout used only to `go run` the picker if needed
POKEPILOT_WALL          # default https://pokemon.labstack.cc
POKEPILOT_MCP_TOKEN     # from ~/.config/pokepilot/env
POKEPILOT_TRIAGE_STATE  # default ~/.local/share/pokepilot/qwagent-triage
POKEPILOT_TRIAGE_TREE   # default ~/Documents/projects/PokePilot-qwagent-triage
```

`--dry-run`:

1. Source `~/.config/pokepilot/env` if present (`set -a; . file; set +a`).
2. `curl -fsS -H "Authorization: Bearer $POKEPILOT_MCP_TOKEN" "$POKEPILOT_WALL/v1/triage"`.
3. `gh pr list --repo maestroi/pokepilot --state open --json title --jq '.[].title'` (repo from `git -C "$POKEPILOT_ROOT" remote get-url origin` if easier).
4. Pipe triage JSON into `go run ./cmd/qwagent-triage pick --claimed ...` with `PWD` = the **source repo** (`POKEPILOT_ROOT` defaulting to the directory that contains this script's `../`).
5. Print the pick JSON and the OpenCode command. Do **not** POST investigate, do **not** flock-wait, do **not** launch OpenCode.

Without `--dry-run`, wrap the body in:

```sh
exec 9>"$STATE/lock"
if ! flock -n 9; then
  echo "qwagent-triage: lock held; skip"
  exit 0
fi
timeout --foreground 50m "$0" --locked "$@"
```

or a single script with `flock -n` around the work. `--locked` is an internal flag so timeout/flock do not recurse.

Reset the worktree only on a real run:

```sh
mkdir -p "$POKEPILOT_TRIAGE_TREE"
if [ ! -d "$POKEPILOT_TRIAGE_TREE/.git" ]; then
  git clone --reference "$POKEPILOT_ROOT" "$POKEPILOT_ROOT" "$POKEPILOT_TRIAGE_TREE" \
    || git clone "$(git -C "$POKEPILOT_ROOT" remote get-url origin)" "$POKEPILOT_TRIAGE_TREE"
fi
git -C "$POKEPILOT_TRIAGE_TREE" fetch origin
git -C "$POKEPILOT_TRIAGE_TREE" checkout main
git -C "$POKEPILOT_TRIAGE_TREE" reset --hard origin/main
git -C "$POKEPILOT_TRIAGE_TREE" clean -fd
```

Then POST investigate, write `$STATE/packet.json` and `$STATE/packet.md`, launch:

```sh
opencode run --auto --model qwen3.8-27b/qwen3.8-27b \
  --dir "$POKEPILOT_TRIAGE_TREE" \
  --title "farm triage ${KEY}" \
  --file "$STATE/packet.md" \
  "$(cat "$PROMPT")"
```

After OpenCode exits 0, verify before trusting the agent:

- current branch matches `fix/*`
- `main` still equals `origin/main`
- `git diff --name-only origin/main` does not contain `.state`, `.gb`, `.sav`, or `zz_*_test.go`
- an open PR title contains `[triage:$KEY]` — if the agent forgot `gh pr create` and the branch was pushed, the script may run `gh pr create` itself using the packet. If the branch was not pushed or gates clearly failed, do not create a PR.

Unreachable wall, missing token, or pick exit 2 → log and `exit 0`.

**Step 1:** Write the script with `--dry-run` complete; real-run path can be a stub that echoes "would claim $KEY" until Task 5.

**Step 2:** `bash -n deploy/qwagent-triage.sh`

**Step 3:** Manual `--dry-run` only if the operator's token and `gh` work. Do not treat network failure as a test failure.

---

### Task 4: Prompt packet

**Files:**
- Create: `deploy/qwagent-triage.prompt.md`

The prompt is the packet. It must tell OpenCode it does **not** choose the queue item. Include the triage skill by path, not by memory.

```markdown
# Farm triage attempt

You are running unattended in a dedicated worktree. The shell already
chose the failure. Do not call pokepilot_get_triage to pick another one.

Read and follow `.claude/skills/pokefarm-triage/SKILL.md` and
`docs/ARCHITECTURE.md`.

The packet JSON is attached. Use its `key`, `run_id`, and `example`.

Do:

1. `pokepilot_get_run_debug` / `pokepilot_get_run_artifacts` for `run_id`.
2. Download the failing round `.state` (objective matching finish.detail).
3. Write `skill/zz_repro_scratch_test.go`, reproduce, then fix the shared
   invariant — not a named-map special case.
4. Re-run the repro. Delete the scratch test.
5. `make test-short`. Never `go test ./skill/...`.
6. If either gate is red, stop. Do not commit.
7. `git switch -c fix/<short-slug>` from this worktree's `main`.
8. Commit only the fix. Message: symptom, root cause, run id.
9. `git push -u origin HEAD`
10. `gh pr create` with title
    `fix(farm): <short symptom> [triage:<key>]`
    Body: run id, fingerprint, what you reproduced, what you changed.

Do not:

- commit or push `main`
- commit `.gb`, `.sav`, `.state`, or `skill/zz_*_test.go`
- `git add -A` when unrelated files are dirty
- merge the PR
- start another farm run unless you need an artifact URL you cannot get
  from the existing run
```

**Step 1:** Write the file.
**Step 2:** No test beyond "file exists and mentions `[triage:<key>]` and `make test-short`". A tiny `TestPromptForbidsSkillSuite` in `deploy/` that reads the prompt and checks those strings is enough.

---

### Task 5: Real-run path, systemd units, Makefile, zsh helpers

**Files:**
- Modify: `deploy/qwagent-triage.sh` (complete locked path)
- Create: `deploy/qwagent-triage.service`
- Create: `deploy/qwagent-triage.timer`
- Create: `deploy/qwagent-triage.zsh`
- Modify: `Makefile` (install target + `.PHONY`)
- Modify: `deploy/README.md` (short "Local qwagent triage" section)
- Modify: `~/.zshrc` only via install, not by hand in the repo

**Service** (`Type=oneshot`, `Nice=10`):

```ini
[Unit]
Description=PokePilot qwagent farm-triage attempt

[Service]
Type=oneshot
Nice=10
# EnvironmentFile is optional; the script also sources this path.
EnvironmentFile=-%h/.config/pokepilot/env
ExecStart=%h/Documents/projects/PokePilot/deploy/qwagent-triage.sh
```

Install must rewrite `ExecStart` to the checkout that ran `make qwagent-triage-install` (`$(CURDIR)`), because this machine's path is not portable. Keep a `.in` template:

```ini
ExecStart=@@POKEPILOT_ROOT@@/deploy/qwagent-triage.sh
```

**Timer:**

```ini
[Unit]
Description=Offer one PokeFarm failure to local qwagent

[Timer]
OnBootSec=5min
OnUnitInactiveSec=30min
AccuracySec=1min
Persistent=false

[Install]
WantedBy=timers.target
```

`Persistent=false` so a laptop waking from sleep does not fire a backlog of ticks.

**`deploy/qwagent-triage.zsh`:**

```zsh
alias qwtriage-on='systemctl --user enable --now qwagent-triage.timer'
alias qwtriage-off='systemctl --user disable --now qwagent-triage.timer'
alias qwtriage-once='systemctl --user start qwagent-triage.service'
alias qwtriage-status='systemctl --user status qwagent-triage.timer qwagent-triage.service'
alias qwtriage-logs='journalctl --user -u qwagent-triage.service -u qwagent-triage.timer -f'
```

Add the same five names to the existing `~/.zshrc` help function when installing (or print them from `qwtriage-status`). Do not enable the timer.

**Makefile:**

```make
.PHONY: ... qwagent-triage-install

qwagent-triage-install:
	mkdir -p "$(HOME)/.config/systemd/user"
	sed 's|@@POKEPILOT_ROOT@@|$(CURDIR)|g' deploy/qwagent-triage.service.in \
		> "$(HOME)/.config/systemd/user/qwagent-triage.service"
	cp deploy/qwagent-triage.timer "$(HOME)/.config/systemd/user/qwagent-triage.timer"
	systemctl --user daemon-reload
	@marker='# PokePilot qwagent-triage helpers'
	@if [ -f "$(HOME)/.zshrc" ] && ! grep -q "$$marker" "$(HOME)/.zshrc"; then \
		printf '\n%s\nsource %s/deploy/qwagent-triage.zsh\n' "$$marker" "$(CURDIR)" >> "$(HOME)/.zshrc"; \
		echo "appended source line to ~/.zshrc (open a new shell)"; \
	fi
	@echo "timer installed but not enabled. qwtriage-on to start, qwtriage-off to stop."
```

Do **not** `enable --now` here.

**README:** a short section after "Issue handoff": what it is, that it is opt-in, the five zsh commands, that it opens PRs and never merges, that it needs `POKEPILOT_MCP_TOKEN`, `gh` auth, local Qwen on `:8002`, and `roms/pokemon_red.gb` in the worktree or `~/.config/pokepilot/pokemon_red.gb`.

**Step 1:** Add units + Makefile + zsh + README.
**Step 2:** `make qwagent-triage-install` (operator machine). Confirm `systemctl --user list-timers --all` shows the timer as inactive/disabled.
**Step 3:** `bash -n deploy/qwagent-triage.sh` and `make test-short ARGS='-run TestPick|TestPrompt'`.

---

### Task 6: Wire the help text and a dry-run smoke

**Files:**
- Modify: `~/.zshrc` help block only if install did not already source the zsh file (install path is enough).
- Modify: `deploy/qwagent-triage.sh` if `--dry-run` is still a stub.

Confirm:

```sh
make test-short
bash -n deploy/qwagent-triage.sh
# optional, needs token + gh:
./deploy/qwagent-triage.sh --dry-run
```

Do not run a live OpenCode attempt as part of implementation unless the human asks.

---

## Done when

- `POKEMON_RED_ROM= go test -short -count=1 ./deploy ./cmd/qwagent-triage` is green
- `make qwagent-triage-install` leaves the timer disabled
- `qwtriage-on` / `qwtriage-off` / `qwtriage-once` / `qwtriage-status` / `qwtriage-logs` exist after a new shell
- `--dry-run` prints a key or "idle" without claiming
- A live tick (manual `qwtriage-once`) is operator-only verification, not a CI gate
