# Qwagent farm-triage loop

Approved 2026-09-11. A local systemd user timer optionally invokes
`opencode --auto --model qwen3.8-27b/qwen3.8-27b` (`qwagent`) against one
actionable PokeFarm failure and opens a PR if the repro plus `make test-short`
go green.

This is not part of the gameplay runtime. It does not change planner, skill,
or adapter policy. The architecture contract in `docs/ARCHITECTURE.md` still
binds the *agent that writes the fix*.

## Goal

Every 30 minutes, if the operator has turned the loop on, claim the top
unused farm failure and let local Qwen try a real fix: dedicated worktree,
reproduce from the failing `.state`, patch the shared invariant, branch,
commit, push, open a PR. Never touch `main`. Never start a second session
while one is running. Never open a second PR for a key that already has one.

## Approach

Hybrid picker + qwagent (option A).

The script owns lock, worktree reset, queue selection, claim, and launch.
Qwen owns reproduce / patch / gates / `gh pr create`. Queue logic is not
delegated to the model.

`qwagent` is the existing alias
`opencode --auto --model qwen3.8-27b/qwen3.8-27b`. The service calls
`opencode run` with those flags, not the interactive TUI.

## Operator control

The timer is **off by default**. `make qwagent-triage-install` installs units
and zsh helpers; it does not enable the timer.

Helpers live next to the existing `qwstart` / `qwstop` aliases in `~/.zshrc`
(sourced from `deploy/qwagent-triage.zsh`):

| Command | Effect |
|---|---|
| `qwtriage-on` | `systemctl --user enable --now qwagent-triage.timer` |
| `qwtriage-off` | `systemctl --user disable --now qwagent-triage.timer` |
| `qwtriage-once` | one shot of the service, timer stays as it was |
| `qwtriage-status` | timer + service status |
| `qwtriage-logs` | follow journald for the units |

## Components

| Path | Role |
|---|---|
| `deploy/qwagent_triage.go` | Deterministic picker (actionable + PR-dedup) |
| `deploy/qwagent_triage_test.go` | Fixture tests for that picker |
| `cmd/qwagent-triage` | `pick` / `claimed` CLI used by the script |
| `deploy/qwagent-triage.sh` | Oneshot runner |
| `deploy/qwagent-triage.prompt.md` | Packet for OpenCode (`@pokefarm-triage`) |
| `deploy/qwagent-triage.service` / `.timer` | User systemd units |
| `deploy/qwagent-triage.zsh` | `qwtriage-*` helpers |
| `make qwagent-triage-install` | Install units + source the zsh file |

Runtime, not git:

- Token: `POKEPILOT_MCP_TOKEN` from `~/.config/pokepilot/env`
- Lock / logs: `~/.local/share/pokepilot/qwagent-triage/`
- Worktree: `~/Documents/projects/PokePilot-qwagent-triage` (created once, reset to `origin/main` each attempt)

## Data flow

1. Timer fires (`OnUnitInactiveSec=30min`).
2. `flock -n` on the lock file. Held → exit 0.
3. `timeout 50m` around the rest so a hung OpenCode cannot skip forever.
4. Fetch `origin`, hard-reset the worktree to `origin/main`.
5. `GET https://pokemon.labstack.cc/v1/triage` (same JSON the MCP tool reads).
6. `gh pr list --state open` and drop groups whose title contains `[triage:<key>]`.
7. Pick the first remaining actionable group (wall already sorts by `count`).
8. Empty → log idle, exit 0. Do not start Qwen.
9. `POST /v1/triage/{key}/investigate`, write a packet (key, run id, detail, count).
10. `opencode run --auto --model qwen3.8-27b/qwen3.8-27b --dir <worktree>` with the prompt + packet.
11. Qwen follows `.claude/skills/pokefarm-triage/SKILL.md`: download `.state`, scratch repro, shared-invariant fix, delete scratch test, `make test-short`, `fix/<slug>` branch, commit, push, `gh pr create` with `[triage:<key>]` in the title.

The script does not start Qwen unless it has a free key. Qwen does not pick from the queue.

## Error handling

Skip this tick (exit 0, no Qwen):

- lock held
- no actionable unused group
- wall unreachable
- worktree reset failed

Abort the attempt, no commit/PR:

- OpenCode / `:8002` down
- 50-minute timeout
- repro still red or `make test-short` red
- branch is not `fix/*`, or `main` moved, or the commit contains `.state` / ROM / `zz_*` scratch tests — script refuses `gh pr create`

A failed attempt is forgotten: the next tick resets the worktree. No key blacklist. An open PR is the only durable skip. No auto-merge.

## Testing

- Picker unit tests in `deploy/` (ROM-free; part of `make test-short`).
- `deploy/qwagent-triage.sh --dry-run` prints the chosen key and the OpenCode command; it does not claim or launch.
- No emulator tests.

## Non-goals

- Cursor `/loop` (dies with the chat).
- Letting Qwen choose the queue item.
- Auto-merge.
- Running on the dirty `main` worktree.
- `go test ./skill/...` as a gate (that boots the ROM for tens of minutes).
