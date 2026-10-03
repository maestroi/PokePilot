# Telegram operator bot

`poketelegram` is an optional remote operator for a running PokePilot farm. It
uses Telegram long polling and talks to PokéWall/replay through their supported
HTTP APIs. It does not need emulator access, a Docker socket, or SSH.

## What it can do

Commands are intentionally compact for phone use:

| Command | Action |
| --- | --- |
| `/menu` | Button home screen: runs, failures, health, fixer, alerts, status |
| `/status` | Wall version, active/queued/recent failed runs, workers, triage groups, replay health |
| `/runs` | Active runs as tappable cards (game, seed, frame, badges, goal) |
| `/run <n\|id>` | Run card: progress, map/position, planner state, party, recent trace, triage summary, latest frame |
| `/flag <n\|id> [note]` | Flag a run as stuck (see "Flag stuck"); the note is optional, or reply to the confirm prompt with one |
| `/stop <n\|id>` | Cooperative cancel, with a two-step confirmation |
| `/restart <n\|id>` | Restart from the newest replayable checkpoint when possible, otherwise fresh-clone the run; confirmation required |
| `/frame <n\|id>` | Latest frame of a run |
| `/health` | Wall, watcher, planner endpoints, nodes, services, disks |
| `/fixer` | Paid/free fixer starts, daily cap, blocked triage keys, triage PRs |
| `/failures` | Grouped recent failure/triage patterns and linked GitHub issues |
| `/alerts` | Open checks and Alertmanager alerts plus recently resolved ones |
| `/board` | Post/refresh the pinned live board |
| `/triage <id>`, `/replay <id>` | Queue the PokéWall investigation / a replay render |

`<n>` is the number shown in the last `/runs` list. Replying to any run card
with `flag`, `stop`, `restart`, `frame`, `run`, `replay` or `triage` runs that
command on the card's run.

Alertmanager omits resolved alerts from its default query, so `/alerts` asks for
them explicitly and lists recent recoveries under a separate heading. A failure
to fetch resolved alerts never hides the active ones.

The monitor sends notifications for completed/failed runs, frame stalls,
PokéWall outages/recovery, replay render ready/failed transitions, and
Alertmanager firing/resolved transitions. Point Alertmanager at the bot for
Swarm/update/service alerts already produced by the observability stack.

It also reports a **completed progression goal** separately. A run finishing the
progress goal it was given has reached the end of that progression track: the
experimental Gen-II preset points one at `gs_supported_frontier`, so this is how
the operator learns the agent will not advance until more content is supported.
The decision reads the structured `goal_kind`/`goal_id` fields on the run's
stats, never the goal's prose summary, and it includes the goal ID in the
message.

## Alerts

Every check (wall, run stalls, planner endpoints, and everything `pokewatch`
reports) has a grace count: it must be bad that many consecutive evaluations
before the bot posts an alert card, so one blip does not page you. A still-bad
check is reminded every 12h, and a recovery posts a resolved note. The card's
**Mute 12h** button silences reminders for that check (a mute is shown on
`/alerts` and the reminder comes due when it expires). Cards for run-specific
alerts carry a restart summary/button where a restart makes sense.

## Flag stuck

`/flag` (or the Flag button) asks the wall to stop that run with reason
`stuck`: the wall rewrites the cancel stop of the flagged attempt into a normal
`stuck` failure, so the usual finish dump, triage, GitHub issue and fixer pick
it up (`POST /v1/runs/{id}/flag-stuck`). The bot follows up with the issue link
when it is filed. If the runner never stops, the wall settles the attempt as
stuck after 10 minutes with no finish dump, so **no issue is filed**; the bot
says so after an hour.

## Watcher and node reporter

`pokewatch` (service `watch`, manager only, Docker socket) checks every minute:
node ready and manager quorum, service replicas for the `POKEWATCH_STACKS`
stacks, Swarm rollbacks, disk free per node mount (`POKEPILOT_WATCH_MIN_FREE_GB`),
node reports arriving, the fixer paid cap, triage keys that exhausted every
ladder tier, `[triage:]` PRs open over 12h, and Docker API access. It pushes the
snapshot to the bot's `POST /v1/ops`. `pokewatch -node` (service `node`, global,
host `/` read-only at `/host`) reports each node's disks and the fixer ledger.

**Deploy freeze:** after `POKEPILOT_FREEZE_ROLLBACKS` (3) rollbacks within
`POKEPILOT_FREEZE_SECONDS` (24h) the watcher sets the label
`pokepilot.deploy-frozen-until=<unix>` on `pokefarm_wall` (`POKEWATCH_FREEZE_SERVICE`);
`deploy/rollout-latest.sh` skips while it is in the future. Runs are unaffected,
and the watcher clears the label when it expires.

## Create the bot

Create a bot with Telegram's **@BotFather** and keep the token secret. To find
your numeric user/chat ID without trusting a third-party bot, send one message
to the new bot and inspect Telegram's `getUpdates` response while the operator
service is stopped.

The bot is **default deny**. Configure at least one of:

- `TELEGRAM_ALLOWED_USER_IDS`: comma/space separated numeric Telegram user IDs.
- `TELEGRAM_ALLOWED_CHAT_IDS`: comma/space separated numeric chat IDs.

Notification destinations are separate:

- `TELEGRAM_NOTIFY_CHAT_IDS`: comma/space separated chat IDs that receive event notifications.
- `TELEGRAM_CHAT_ID`: backward-compatible single private chat. When set, that
  chat is both authorized and used for notifications.

An empty allowlist does not make the bot public; it refuses all Telegram
commands.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | required | Bot token |
| `TELEGRAM_BOT_TOKEN_FILE` | empty | Read the token from a mounted secret file instead of env |
| `TELEGRAM_ALLOWED_USER_IDS` | empty | Authorized users |
| `TELEGRAM_ALLOWED_CHAT_IDS` | empty | Authorized chats |
| `TELEGRAM_NOTIFY_CHAT_IDS` | empty | Event notification chats |
| `POKEPILOT_OPERATOR_URL` | `http://wall:8080` | PokéWall/operator API |
| `POKEPILOT_REPLAY_URL` | empty | Replay render API; optional |
| `POKEPILOT_ALERTMANAGER_URL` | empty | Alertmanager base URL; optional |
| `TELEGRAM_CHAT_ID` | empty | Legacy single private chat (authorized and notified) |
| `POKEPILOT_GITHUB_REPO` | `maestroi/PokePilot` | Repo for issue links |
| `POKEPILOT_OPS_TOKEN_FILE` | empty | Shared secret authorizing `POST /v1/ops` from `pokewatch` |
| `POKEPILOT_WATCH_DIGEST_HOUR` | `9` | UTC hour of the daily digest |
| `POKEPILOT_ADMIN_BASE_URL` | `https://admin.rompilot.app` | Admin links in messages |
| `POKEPILOT_SPECTATOR_BASE_URL` | `https://rompilot.app/runs` | Public run links |
| `POKETELEGRAM_HTTP_ADDR` | `:8080` | Health/metrics listener |
| `POKETELEGRAM_EVENT_INTERVAL` | `15s` | Farm/event polling interval |
| `POKETELEGRAM_STALL_AFTER` | `15m` (`30m` in `ops.yml`) | No-frame-progress threshold |
| `POKETELEGRAM_CONTROL_SPACING` | `2s` | Per-operator destructive-action rate limit |

Run `poketelegram -check-config` in deployment validation to catch malformed
durations or a missing token before rollout.

## Swarm deployment

One stack, `pokefarm-ops` (`deploy/ops.yml`), holds the bot, `pokewatch` and
the node reporter. It is separate from `farm.yml` so a missing Telegram token
can never prevent the core farm from deploying. Only `watch` mounts the Docker
socket; `telegram` is unprivileged.

```sh
export FARM_IMAGE=ghcr.io/maestroi/pokepilot:<digest-or-tag>
export TELEGRAM_ALLOWED_USER_IDS='123456789'
export TELEGRAM_NOTIFY_CHAT_IDS='123456789'

printf %s "$TOKEN" | docker secret create poketelegram_bot_token -
openssl rand -hex 32 | docker secret create pokefarm_ops_token -
printf %s "$GH_READONLY_TOKEN" | docker secret create pokewatch_github_token -
docker stack deploy -c deploy/ops.yml pokefarm-ops
```

The bot joins the existing `pokefarm_default` network (override with
`POKEPILOT_FARM_NETWORK`) plus a private `ops` overlay shared with `watch` and
`node`. The replay service is an external sidecar, so `deploy/ops.yml` maps
`FARM_REPLAY_URL` into `POKEPILOT_REPLAY_URL`. The bot token is only read from
the `poketelegram_bot_token` secret file; do not add it to other services.

## Safety model

- Authorization is an explicit user/chat allowlist with default deny.
- `/stop` and `/restart` create a two-minute confirmation bound to the same
  Telegram user and chat.
- Confirmed destructive actions are rate-limited per operator.
- Control operations emit structured `audit {...}` log records containing
  actor/chat/action/target/outcome but never the bot token.
- Restart never mutates emulator memory directly: it asks PokéWall for the
  newest replayable checkpoint, creates a reproduction run, then cooperatively
  cancels the original. If no replayable checkpoint exists it uses PokéWall's
  clone endpoint.
- The two mutating control operations the bot performs (`/stop` and `/triage`)
  go through the same `operatorapi.Client` the admin/MCP control plane uses, so
  the endpoint, request shape and error handling have one owner instead of one
  copy per surface.
- Existing pokeui/MCP flows are unchanged; the bot consumes the same supported
  APIs.

## Observability and runbook

The service exposes:

- `GET /healthz` — 200 while the operator API is reachable, 503 while degraded.
- `GET /metrics` — Prometheus text metrics for updates, commands,
  unauthorized attempts, Telegram/operator errors, notifications, actions,
  action failures, pending confirmations, wall health, and last successful
  operator poll.

Scrape and dashboard assets live in `deploy/monitoring/`:

- `prometheus-poketelegram.yml` — a scrape job to merge into your Prometheus
  `scrape_configs` (DNS discovery of `tasks.telegram`; no Docker socket needed).
- `grafana-poketelegram.json` — a dashboard with a panel for every metric above.
- `README.md` — metric reference, suggested alert rules, and the current gap.

Note that **this repository deploys no Prometheus/Grafana/Alertmanager stack**,
so nothing scrapes these metrics or feeds `/alerts` until one is wired up
alongside the farm. The bot never requires it.

Useful checks:

```sh
docker service logs -f pokefarm-ops_telegram
curl http://<telegram-task>:8080/healthz
curl http://<telegram-task>:8080/metrics
```

If commands stop responding, check Telegram API reachability first, then
`POKEPILOT_OPERATOR_URL`. If only `/replay` fails, verify
`FARM_REPLAY_URL`/`POKEPILOT_REPLAY_URL`. If `/alerts` says it is not
configured, set `POKEPILOT_ALERTMANAGER_URL`.

To rotate a leaked token, revoke/regenerate it with @BotFather, replace the
Swarm secret or environment value, and force-update only the Telegram service.
