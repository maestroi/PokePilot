# External domains

RomPilot is the public product name. The Go module, binaries, and operator
environment variables stay `pokepilot` unless they are part of a user-facing
URL.

```text
rompilot.app          Public RomPilot / spectator
admin.rompilot.app    Private admin/control plane
api.rompilot.app      External API entry point
```

Local development does not need these hosts. `make farm-up` still publishes the
operator UI on `http://localhost:18080` and the spectator UI on
`http://localhost:18081`. Empty URL variables keep generating relative or
localhost links.

## Environment variables

Use the existing `POKEPILOT_*` convention:

| Variable | Production value | Also accepted |
| --- | --- | --- |
| `POKEPILOT_PUBLIC_BASE_URL` | `https://rompilot.app` | `POKEPILOT_SPECTATOR_URL` (legacy alias) |
| `POKEPILOT_ADMIN_BASE_URL` | `https://admin.rompilot.app` | `POKEPILOT_RUN_BASE_URL` (legacy alias for issue/debug links) |
| `POKEPILOT_API_BASE_URL` | `https://api.rompilot.app` | falls back to the public base when unset |

Do not hardcode these hosts in application code. `sites.FromEnv()` is the Go
source of truth; the operator UI reads them from `GET /v1/ui-config`.

## Swarm / Traefik routing

The farm already runs two `pokeui` processes. Keep that split:

| Hostname | Service | Notes |
| --- | --- | --- |
| `rompilot.app` | `spectator` (`pokeui -spectator`) | Public HTML + same-origin `/v1/watch` and `/frame` |
| `www.rompilot.app` | `spectator` | Application 301s to the apex |
| `pokemon.maestroi.cc` | `spectator` | Legacy public host; application 301s to `https://rompilot.app` preserving path and query |
| `api.rompilot.app` | `spectator` | Public API only (`/v1/watch`, `/frame`, `/maps`, `/v1/build`). HTML is 404 |
| `admin.rompilot.app` | `ui` (`pokeui`) | Operator console, MCP, run/debug/artifact routes |
| `pokemon.labstack.cc` | `ui` | Legacy private operator host; keep until clients and Cloudflare Access are cut over |

Postgres, LiteLLM, model runners, workers, queues, and the issue adapter stay
on the overlay. Do not publish them as public subdomains.

## Cloudflare DNS

Create proxied records on the `rompilot.app` zone, all pointing at the same
Traefik/Swarm edge that currently serves `pokemon.labstack.cc`:

| Record | Type | Target | Proxy |
| --- | --- | --- | --- |
| `rompilot.app` | A/AAAA or CNAME | Traefik edge | Proxied |
| `www` | CNAME | `rompilot.app` | Proxied |
| `admin` | CNAME | `rompilot.app` | Proxied |
| `api` | CNAME | `rompilot.app` | Proxied |

TLS can stay on Cloudflare (Flexible/Full is whatever the existing
`*.labstack.cc` stacks use) or terminate on Traefik. Spectator currently uses
Traefik `websecure`; operator historically used `web` behind Cloudflare HTTPS.
Match the existing edge instead of introducing a second certificate path.

### Redirect `pokemon.maestroi.cc`

Keep the old DNS record pointed at the spectator service until links drain, and
rely on the application 301. Additionally add a Cloudflare Redirect Rule so the
migration still works if Traefik is bypassed:

```text
When hostname equals pokemon.maestroi.cc
Then static redirect to https://rompilot.app${uri}
Status 301
Preserve path and query
```

Example: `https://pokemon.maestroi.cc/runs/123` → `https://rompilot.app/runs/123`.

`/?run=ID` links are then canonicalized by pokeui to `/runs/ID`.

### Access / authentication

`rompilot.app` and `api.rompilot.app` are public. Do not put Cloudflare Access
in front of them.

`admin.rompilot.app` is the private control plane. Reuse the existing Access
application (or equivalent upstream auth proxy) that currently protects
`pokemon.labstack.cc`. Spectator clients never receive that credential.

Application cookies are not used for admin sessions today. If they are added,
keep them host-only on `admin.rompilot.app` and do not set `Domain=rompilot.app`.

## CORS

Browser UIs talk same-origin to their own `pokeui`:

- public pages call `/v1/watch` and `/frame` on `rompilot.app`
- admin pages call `/v1/*` on `admin.rompilot.app`

The spectator process allows CORS only for the configured public and API
origins. The operator process allows CORS only for the configured admin origin.
Neither allowlist includes the other, and neither uses `*`. Live polling stays
HTTPS; any future WebSocket URL derived from these bases uses `wss` on HTTPS
hosts.

## Public paths

| Path | Page |
| --- | --- |
| `/` | Live spectator homepage |
| `/runs/:id` | Public run page |
| `/explore` | World/map explorer (`/world` still works) |
| `/replays` | Replay library |
