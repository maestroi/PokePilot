# Public spectator mode

The public product is **RomPilot**. `pokeui -spectator` is the public, read-only frontend. It is a separate server mode rather than a cosmetic version of the operator console.

Production hostnames:

```text
rompilot.app          Public RomPilot / spectator
admin.rompilot.app    Private admin/control plane
api.rompilot.app      External API entry point
```

See `docs/DOMAINS.md` for DNS, Cloudflare redirects, and CORS.

## Security boundary

Spectator mode mounts only:

- `GET /` — the spectator page
- `GET /watch.js` — its browser code
- `GET /v1/watch` — a sanitized run snapshot
- `GET /frame?run=...` — a read-only classic game frame
- `GET /render-state?run=...` — validated semantic RenderState for the modern live renderer
- `GET /maps/{name}` — embedded semantic map JSON for the live overlay

It does **not** mount the operator dashboard, queue, cancel, delete, triage, or MCP routes. Requests for those paths never reach `pokewall`.

The public snapshot intentionally omits infrastructure and debugging fields from `/v1/dashboard`, including worker addresses and versions, wall version, seeds, traces, planner questions, raw model exchanges, failure detail, issue links, token counts, model names, and backend/failover metadata. The browser receives only the pieces useful for watching a run: status, route/goal, position, current decision, bounded planner progress, party, money, badges, attempts, finish reason, and the live map overlay (player tile, NPC tiles, recent trail). Sprite picture IDs and slots stay off the public wire.

The page renders run-provided text with DOM `textContent`, and spectator responses add a restrictive Content Security Policy plus `nosniff`, no-referrer, and same-origin framing headers.

## Swarm deployment

`deploy/farm.yml` runs two `pokeui` processes:

- `ui` on `${FARM_WALL_PORT:-18080}` — private operator console; keep this behind trusted network access/authentication. Production hostname: `admin.rompilot.app`.
- `spectator` on `${FARM_SPECTATOR_PORT:-18081}` — read-only surface intended for a public reverse proxy or hostname. Production hostname: `rompilot.app`. `api.rompilot.app` is the same process, limited to public API paths.

The wall and runners still publish no host ports. The spectator process reaches the wall only over the Swarm network.

For example, after the normal farm build/deploy flow, open `http://<farm-host>:18081/` locally or point a TLS reverse proxy at the spectator service for `rompilot.app`. Do not point the public hostname at the operator port.

## Standalone

For local testing against an already-running wall:

```sh
go run ./cmd/pokeui -wall http://127.0.0.1:8080 -http :18081 -spectator
```

The page polls the sanitized snapshot every two seconds. A selected live Pokémon Red run also polls validated semantic `RenderState` independently. The semantic renderer now defaults to the **Gold / Silver** presentation for supported overworld, dialogue, menu, and battle scenes without changing the run. Overworld movement uses a browser-owned bounded animation clock: authoritative player/NPC positions are interpolated at display refresh, while reconnects, map changes, rewinds, and large teleports snap directly to current truth. Battle presentation uses authoritative species, level, HP, status, and move/PP state decoded by the Red adapter with Gold-version sprites. The viewer can switch between the semantic **Gold / Silver** view and **Classic** framebuffer fallback; Tiny Town remains available as the CC0 alternate theme. Theme choice is browser-local and never changes or restarts the run. The semantic endpoint is read-only and buffered on the emulator stepping goroutine, so spectator HTTP requests never inspect or mutate gameplay RAM. See `docs/RENDER_THEMES.md` for the theme contract and provenance.
