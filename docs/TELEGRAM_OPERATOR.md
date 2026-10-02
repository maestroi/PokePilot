# Telegram operator bot

`poketelegram` is an optional remote operator for a running PokePilot farm. It
uses Telegram long polling and talks to PokéWall/replay through their supported
HTTP APIs. It does not need emulator access, a Docker socket, or SSH.

## What it can do

Commands are intentionally compact for phone use:

| Command | Action |
| --- | --- |
| `/status` | Wall version, active/queued/recent failed runs, workers, triage groups, replay health |
| `/runs` | Active run IDs, game, seed, frame, badge count and current goal |
| `/run <id>` | Run progress, map/position, planner state, party, result, recent trace, triage summary, latest frame |
| `/failures` | Grouped recent failure/triage patterns and linked GitHub issues |
| `/alerts` | Active Alertmanager alerts plus recently resolved ones, when configured |
| `/triage <id>` | Queue the existing PokéWall investigation for the run's failure group |
| `/replay <id>` | Queue a replay render when a replay endpoint is configured |
| `/stop <id>` | Cooperative cancel, with a two-step confirmation |
| `/restart <id>` | Restart from the newest replayable checkpoint when possible, otherwise fresh-clone the run; confirmation required |

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
| `POKEPILOT_ADMIN_BASE_URL` | `https://admin.rompilot.app` | Admin links in messages |
| `POKEPILOT_SPECTATOR_BASE_URL` | `https://rompilot.app/runs` | Public run links |
| `POKETELEGRAM_HTTP_ADDR` | `:8080` | Health/metrics listener |
| `POKETELEGRAM_EVENT_INTERVAL` | `15s` | Farm/event polling interval |
| `POKETELEGRAM_STALL_AFTER` | `15m` | No-frame-progress threshold |
| `POKETELEGRAM_CONTROL_SPACING` | `2s` | Per-operator destructive-action rate limit |

Run `poketelegram -check-config` in deployment validation to catch malformed
durations or a missing token before rollout.

## Swarm deployment

The bot has its own overlay so a missing Telegram token can never prevent the
core farm from deploying:

```sh
export FARM_IMAGE=ghcr.io/maestroi/pokepilot:<digest-or-tag>
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_ALLOWED_USER_IDS='123456789'
export TELEGRAM_NOTIFY_CHAT_IDS='123456789'

docker stack deploy -c deploy/telegram.yml pokefarm-telegram
```

The overlay joins the existing `pokefarm_default` network. Override
`POKEPILOT_FARM_NETWORK` if the farm stack has another name. The replay
service is an external sidecar in the current farm deployment, so
`deploy/telegram.yml` maps `FARM_REPLAY_URL` into the bot rather than
inventing an in-stack replay hostname.

For Docker/Swarm secrets, mount the token at (for example)
`/run/secrets/poketelegram_bot_token`, leave `TELEGRAM_BOT_TOKEN` unset, and
set:

```text
TELEGRAM_BOT_TOKEN_FILE=/run/secrets/poketelegram_bot_token
```

Only the Telegram service needs that secret. Do not add it to wall, runner, UI,
or replay services.

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
docker service logs -f pokefarm-telegram_telegram
curl http://<telegram-task>:8080/healthz
curl http://<telegram-task>:8080/metrics
```

If commands stop responding, check Telegram API reachability first, then
`POKEPILOT_OPERATOR_URL`. If only `/replay` fails, verify
`FARM_REPLAY_URL`/`POKEPILOT_REPLAY_URL`. If `/alerts` says it is not
configured, set `POKEPILOT_ALERTMANAGER_URL`.

To rotate a leaked token, revoke/regenerate it with @BotFather, replace the
Swarm secret or environment value, and force-update only the Telegram service.
