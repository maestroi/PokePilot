# Telegram ops: one bot for alerts, health and actions

Date: 2026-10-03. Status: approved design, pending plan.

## Goal

The operator runs the farm from a phone, often while away. One Telegram bot
must be the only alert source, show useful state without typing IDs, and let
the operator act (including "please investigate this stuck run"). Everything
runs as Swarm services; no host systemd units, no desktop.

This replaces `deploy/farm-watch.sh` (desktop user timer, decommissioned
2026-10-03) and extends `cmd/poketelegram` (#2374/#2399).

## Non-goals

- No Prometheus/Grafana/Alertmanager stack (the bot's existing optional
  Alertmanager hook stays as is).
- No new game/runtime policy. The flag-stuck path reuses the existing `stuck`
  outcome; nothing game-specific enters the bot, watcher or wall.

## Components

Stack `pokefarm-ops` replaces `pokefarm-telegram`. All three services use the
farm image (`ghcr.io/maestroi/pokepilot`).

| Service | Mode / placement | Privilege | Owns |
|---|---|---|---|
| `telegram` (`poketelegram`) | 1 replica, any node | none | All Telegram I/O, the alert state machine, digest, interface, live board |
| `watch` (`pokewatch`) | 1 replica, `node.role == manager` | `/var/run/docker.sock` | Swarm, GitHub and deploy-freeze checks; aggregates `node` reports; pushes check results to `telegram` |
| `node` (`pokewatch -node`) | `mode: global` | host `/` read-only at `/host` | Its node's free disk; on the fixer node, the fixer ledger summary |

`watch` reschedules to any reachable manager on node loss. `telegram` keeps the
documented no-socket/no-SSH safety model.

### Data flow

```
node (every node) --POST /v1/node-report--> watch --POST /v1/checks--> telegram --> Telegram API
                                              ^ docker API, GitHub API, planner /health
telegram --> wall/replay APIs (as today)
```

- `node` → `watch`: every 5 min, `{node, free_gb_by_mount, fixer?: {paid_24h, free_24h, spent_keys[], parked_keys[]}}`.
  `watch` treats a node with no report for 15 min as a failed check on that node.
- `watch` → `telegram`: every 1 min, the full list of check results
  `[{name, ok, message, links[], subject?}]`. A full list (not deltas) makes
  restarts of either side idempotent.
- Both internal endpoints require a shared secret header
  (Swarm secret `pokefarm_ops_token`) and listen only on the stack's overlay.

## Checks

The alert state machine lives in `telegram` and applies to every check, both
the ones it computes itself (wall, stall) and the ones `watch` pushes:
grace ticks before the first alert, a reminder every 12h while it stays bad,
recovery edits the original alert, and the operator can mute it for 12h.
State is in memory only, so `telegram` stays free to run on any node (Swarm
named volumes are not reliable here and a bind mount would pin it). After a
restart the first full check list is not paged check by check; the bot sends
one "restarted: N checks failing" message listing them and adopts them as
open alerts. Mutes do not survive a restart.

| Check | Source | Grace | Notes |
|---|---|---|---|
| wall reachable | telegram | 2 | existing |
| frame stall per run | telegram | 1 | existing; alert carries run buttons |
| node down / manager unreachable | watch (docker API) | 2 | new; reports quorum, e.g. "2/3 managers" |
| service replicas (`pokefarm_*`, `pokefixer_*`) | watch | 3 | |
| Swarm rollback | watch | 1 | also drives the freeze below |
| deploy frozen | watch | 1 | informational while frozen |
| disk free < 20G (per node, per mount) | node → watch | 2 | `POKEPILOT_WATCH_MIN_FREE_GB` |
| node report missing | watch | 1 | |
| planner endpoint unreachable | telegram | 2 | probes each live run's `stats.endpoint` + `/health`; the bot already holds the run list, so `watch` needs no wall access |
| paid fixer cap reached | node (fixer) → watch | 1 | `POKEPILOT_PAID_DAILY_CAP` |
| spent / parked triage keys | node (fixer) → watch | 1 | links each key's issue |
| `[triage:]` PRs open > 12h | watch (GitHub) | 1 | |

### Deploy freeze (replaces `systemctl stop pokefarm-pull.timer`)

`watch` records each rollback (service, completion time). When 3 or more occur
within 24h it sets the label `pokepilot.deploy-frozen-until=<unix>` on the
`pokefarm_wall` service (now + 24h). `deploy/rollout-latest.sh` reads that
label at the start of each tick and exits early while `now < until`. The freeze
lifts by itself; `watch` removes the stale label and `telegram` announces
"deploys resumed". Runs are never touched.

### Fixer ledger summary

`node` reads `/host/opt/pokefixer/ledger.tsv` when it exists. Paid and free
counts are the same awk rules as `farm-watch.sh`. Spent/parked keys use the
existing ladder logic from `deploy/qwagent_triage.go`, moved into an
importable package if it is `package main` today (the move is a refactor with
no behaviour change). The ladder tiers come from the same
`POKEPILOT_TRIAGE_LADDER` value the fixer uses.

## Flag stuck ("please investigate this run")

A running run that the stagnation watchdog has not stopped has no failure
group, so `/triage` cannot reach it. Flagging makes it fail the normal way.

1. Wall: `POST /v1/runs/{id}/flag-stuck {note}` records the note and the
   attempt it applies to on the run, and sets the existing cooperative cancel
   flag so the runner stops at its next safe boundary.
2. Runner: unchanged. The wall rewrites exactly that cancel stop of the
   flagged attempt into the existing `stuck` outcome with detail
   `operator flagged: <note>`, and recovery applies as for any stuck stop
   (a flag is not a user cancel). This is the same typed outcome the
   stagnation watchdog produces; triage, issues and the fixer need no change.
3. Wall: `terminalRunFailure` already turns `stuck` into a blocking objective
   failure → `[farm]` issue → triage key → fixer. The normal recovery ladder
   resumes the run from its checkpoint, unlike a cancel.
4. Fallback: if the runner does not finish within 10 min of the flag (for
   example a wedged emulator loop), the wall settles the attempt as `stuck`
   itself with the same detail plus "runner did not stop; no finish dump".
   Without a dump no issue is filed; the bot says so after an hour, so the
   flag is never silently lost.
5. Bot: two-step confirmation. Replying to the confirmation message with
   text flags with that text as the note; tapping Confirm flags with no
   note. The bot then posts the issue link once triage files it.
6. Implementation note: no runner or heartbeat change is needed. A wall
   cancel already stops the runner (the LLM path reports it as `budget` with
   no detail, the policy paths as `cancelled`); the wall rewrites exactly
   that stop of the flagged attempt to `stuck` in both finish paths
   (`handleFinish` and the control-plane wrapper before `persistFinish`).

## Interface

- **Home card** (`/start`, `/menu`): Status · Runs · Failures · Health ·
  Fixer · Alerts. Every navigation tap edits the same message
  (`editMessageText`); every card has ↻ Refresh and « Back.
- **Runs**: one button per run, e.g. `🟢 red · 🏅5 · Route 12`, paged with
  ‹ ›. Lists also number runs; `/run 2`, `/flag 2` resolve against the
  last list shown in that chat.
- **Run card**: badges bar (`🏅 ●●●●●○○○ 5/8`), maps visited, time since last
  new map, frames/min over 15 min, attempt / lost / recovery-event counts,
  planner state, goal, and the run's result or triage group when present. Buttons: Frame, Replay, Triage,
  🚩 Flag stuck, Stop, Restart (Stop/Restart/Flag confirmed), Admin, Spectator.
- **Reply shortcuts**: replying `flag`, `stop`, `frame` or `run` to any bot
  message that concerns one run acts on that run. The bot keeps a bounded
  message-id → run-id map per chat.
- **Alert cards**: `🔴 <check>`, one-line reason, time failing, buttons Mute
  12h · Open · Details; stall alerts add 🚩 Flag stuck · Open run · Frame.
  Recovery edits the card to `✅ resolved after <d>` and replies to it.
- **Health card**: nodes and quorum, replicas, rollbacks, freeze, disk by node
  (lowest first), planner endpoint.
- **Fixer card**: paid starts vs cap, qwen vs paid starts, spent/parked keys
  with issue links, open `[triage:]` PRs with ages, PRs merged in 24h.
- **Live board** (`/board`): one message edited every minute (runs, health,
  fixer). The operator pins it. Edits do not notify.
- **Digest** at 09:00 (`POKEPILOT_WATCH_DIGEST_HOUR`): PRs merged, farm issues
  opened/closed, fixer starts (qwen / paid vs cap) and merge rate, badges per
  run with change since the last digest, anything muted or frozen. Buttons:
  Runs · Health · Fixer.
- HTML parse mode throughout, with escaping; `setMyCommands` at startup.
- Existing typed commands keep working.

## Configuration and deploy

- `telegram` gains: `POKEPILOT_OPS_TOKEN_FILE`, digest hour. It keeps the
  previous digest's badge counts in memory; after a restart the first digest
  shows no per-run change.
- `watch` needs: `POKEPILOT_GITHUB_REPO`, a read-only GitHub token secret
  (pull requests + issues read), `POKEPILOT_OPS_TOKEN_FILE`, the stack names
  to watch (`pokefarm`, `pokefixer`), freeze thresholds.
- `node` needs: `POKEPILOT_OPS_TOKEN_FILE`, min free GB, ledger path, ladder
  tiers, paid cap.
- `deploy/telegram.yml` becomes `deploy/ops.yml`; the homelab `swarm.sh
  telegram` becomes `swarm.sh ops`. `deploy/farm-watch.*` and the
  `qwagent-triage` desktop timer install are deleted from the repo, along with
  the Makefile target pieces and README section that install them.

## Error handling

- Any single probe failing marks only its own check failed; `watch` never
  aborts a tick on one error.
- `watch` down: `telegram` raises "watcher silent" after 5 min without a push.
  `telegram` down: Swarm restarts it; the healthcheck already covers the wall
  link. The restart summary replaces per-check pages after a restart.
- Telegram API errors are counted (existing metric) and retried on the next
  tick; edits to deleted/too-old messages fall back to sending a new message.

## Testing

- Alert state machine: table test of grace, reminder, recovery, mute, and the
  restart summary.
- `watch`: docker API responses from fixtures (nodes, services with
  `UpdateStatus.State = rollback_*`), GitHub fixtures; freeze set/lift.
- `node`: ledger fixture → paid/free counts and spent/parked keys, matching
  the current `farm-watch.sh` results on the same file.
- Flag stuck: wall handler sets the cancel flag and the attempt; the flagged
  cancel stop (`budget` with no detail, or `cancelled`) is rewritten to
  `stuck` while other outcomes and other attempts are not; a flagged run does
  not settle as a user cancel; the reaper settles a flagged run that keeps
  heartbeating past 10 min.
- Bot interface: callback routing, paging, number/reply resolution, HTML
  escaping, message-edit fallback, using a fake Telegram server.
- `rollout-latest.sh`: existing Go test harness extended for the freeze label.
- Deploy: `docker stack config` on `deploy/ops.yml` in the deploy tests, as
  for the other stack files.
