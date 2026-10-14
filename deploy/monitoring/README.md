# Monitoring assets

This directory holds scrape configuration for the Prometheus that already
runs outside this repository. Nothing here deploys Prometheus, Grafana, or
Alertmanager, and the application stack must not gain a second Grafana.

Instrumentation conventions live in package `metrics` (`pokepilot_` prefix,
histograms for latency, bounded labels, no `run_id` / `failure_id` /
`family_id` / trace or goal identifiers). Each instrumented process exposes
`GET /metrics` with `pokepilot_build_info{service,revision}`, `pokepilot_up`,
Go runtime metrics, and process metrics. The revision label is the git SHA
stamped at image build; local binaries report `dev`.

## Application scrape targets

Merge `prometheus-pokepilot.yml` into the existing Prometheus `scrape_configs`.
Prometheus has to sit on the farm overlay. Ports are the in-network listeners,
not the published host ports.

| Service | Process | Port | Swarm DNS |
| --- | --- | --- | --- |
| wall | pokewall | 8080 | `tasks.wall` |
| ui | pokeui | 8080 | `tasks.ui` |
| spectator | pokeui `-spectator` | 8080 | `tasks.spectator` |
| runner | pokepilot | 8099 | `tasks.runner` |
| issues | pokeissues | 8080 | `tasks.issues` |
| virtualtrader | gen1-trade-peer | 8080 | `tasks.virtualtrader` |
| replay | pokereplay | 8080 | dedicated renderer; see `deploy/REPLAY_HOST.md` |
| telegram | poketelegram | 8080 | `tasks.telegram` (job in `prometheus-poketelegram.yml`) |
| watch | pokewatch | 8080 | ops overlay only; not reachable from the farm network |

Services that expose `/metrics` carry `prometheus.scrape=true` and
`prometheus.port` deploy labels for a Prometheus that already uses Docker
Swarm service discovery. The DNS jobs above do not need the Docker socket.

Confirm a target:

```sh
curl -s http://<task>:8080/metrics | head
```

Runners listen on 8099. A fresh scrape includes `pokepilot_up`,
`pokepilot_build_info`, `go_goroutines`, and `process_start_time_seconds`.
Later issues add run, failure, and subsystem series to the same endpoint.

## Telegram operator

Assets for scraping and dashboarding `cmd/poketelegram` (the Telegram remote
operator bot):

| File | Purpose |
| --- | --- |
| `prometheus-poketelegram.yml` | Scrape job for the bot's `/metrics` endpoint |
| `grafana-poketelegram.json` | Grafana dashboard for the bot's own metrics |

## Current status

**This repository does not deploy a Prometheus, Grafana or Alertmanager stack.**
`deploy/farm.yml` deploys wall, runner, ui, replay, spectator and friends; there
is no observability service in any overlay here. The bot already exposes its
metrics and these files are ready to consume them, but until the stack that
scrapes them exists they are configuration waiting for its owner.

Two operator features are inert for the same reason:

- `/alerts` and the Alertmanager firing/resolved notifications need
  `POKEPILOT_ALERTMANAGER_URL`; without it the command answers
  "Alertmanager is not configured for this bot."
- Nothing scrapes `poketelegram_*`, so the dashboard is empty until the scrape
  job below is merged.

That is a deployment gap, not a bot gap: the bot never requires the monitoring
stack to be present, and a missing stack cannot affect run execution.

## Scraping

The bot's metrics listener (`POKETELEGRAM_HTTP_ADDR`, default `:8080`) is not
published to the host, so Prometheus must run on the same overlay network as
the farm.

1. Merge the job from `prometheus-poketelegram.yml` into your Prometheus
   `scrape_configs:` list. It discovers `tasks.telegram` over DNS (the `telegram` service of the
   `pokefarm-ops` stack) on the farm network (`pokefarm_default`, which the bot
   joins) and needs no Docker socket. The stack's private `ops` overlay is not
   attachable, so Prometheus cannot join it.
2. If your Prometheus already uses Docker Swarm service discovery with the
   Docker socket mounted, use the commented `dockerswarm_sd_configs` variant in
   that file instead and label the service:

   ```yaml
   deploy:
     labels:
       prometheus.scrape: "true"
       prometheus.port: "8080"
   ```

3. Confirm the target is up:

   ```sh
   curl -s http://<telegram-task>:8080/metrics | head
   ```

Exposed bot metrics (Prometheus counters and gauges; names are unchanged).
The same `/metrics` response also includes `pokepilot_build_info`,
`pokepilot_up`, Go runtime metrics, and process metrics:

| Metric | Meaning |
| --- | --- |
| `poketelegram_updates_total` | Telegram updates received |
| `poketelegram_commands_total` | Commands handled |
| `poketelegram_unauthorized_total` | Callers rejected by the allowlist |
| `poketelegram_telegram_errors_total` | Telegram API failures |
| `poketelegram_operator_errors_total` | Operator API failures |
| `poketelegram_notifications_total` | Notifications delivered |
| `poketelegram_actions_total` | Confirmed control actions executed |
| `poketelegram_action_errors_total` | Control actions that failed |
| `poketelegram_pending_confirmations` | Destructive actions awaiting confirmation |
| `poketelegram_wall_healthy` | 1 while the operator API is reachable |
| `poketelegram_last_operator_poll_timestamp_seconds` | Unix time of the last successful poll |

## Dashboard

Import `grafana-poketelegram.json` and select your Prometheus datasource when
prompted (the dashboard carries a `DS_PROMETHEUS` datasource variable). It has
panels for wall reachability, operator poll age, pending confirmations,
unauthorized attempts, Telegram/operator/action error rates, and notification
and action throughput. Every panel's query is one of the metrics above.

## Suggested alert rules

These are the conditions an operator actually wants paged for. Merge into your
Prometheus rule files:

```yaml
groups:
  - name: poketelegram
    rules:
      - alert: PokeTelegramWallUnreachable
        expr: poketelegram_wall_healthy{job="poketelegram"} == 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: Telegram operator cannot reach the PokePilot operator API
      - alert: PokeTelegramPollStale
        expr: time() - poketelegram_last_operator_poll_timestamp_seconds{job="poketelegram"} > 300
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: Telegram operator has not polled the operator API in 5m
      - alert: PokeTelegramUnauthorizedAttempts
        expr: increase(poketelegram_unauthorized_total{job="poketelegram"}[15m]) > 5
        labels:
          severity: warning
        annotations:
          summary: Repeated unauthorized Telegram callers rejected
      - alert: PokeTelegramActionFailures
        expr: increase(poketelegram_action_errors_total{job="poketelegram"}[15m]) > 0
        labels:
          severity: warning
        annotations:
          summary: A confirmed Telegram control action failed
```

## Alert delivery to Telegram

The bot can receive from Alertmanager rather than Prometheus, so Alertmanager
should also be pointed at the bot's chat. Set `POKEPILOT_ALERTMANAGER_URL` so
`/alerts` can read active and recently resolved alerts, and add a webhook
receiver that forwards the same alerts to the notification chat.
