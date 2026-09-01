# Pokefarm wall operator UI — design

Status: agreed, not implemented.
Date: 2026-08-29.

Replace the 2-second HTML table on the pokefarm wall with an operator
console: live run cards, workers, finished-run history, enqueue, and
cancel. The browser still never reaches the swarm overlay; `pokeui` stays
the only host-published process.

This does not change `emu/`, `skill/`, `agent/`, `red/`, or the farm wire
types runners already speak. No Vue, no Node, no new Go module.

---

## 1. Why

The wall already tracks tiles, workers, heartbeats, and cooperative
cancel. The page that shows them is a full-page `<table>` refresh. The
farm design (`docs/archive/2026-08-26-farm-design.md` §5) wanted a card grid
you click for trace and history, plus start/stop from the wall. This is
that surface, watch and operate.

---

## 2. Architecture

```
browser  --:18080-->  pokeui (host-published, overlay-attached)
                         |  GET /                     embedded HTML/CSS/JS
                         |  GET /v1/dashboard  ──proxy──>  wall
                         |  POST /v1/specs     ──proxy──>  wall
                         |  POST /v1/runs/{id}/cancel ──>  wall
                         |  GET /frame?run=    ──proxy──>  wall
                         v
                       wall (overlay only, no host ports)
                         |  live screens from runners
                         v
                       runners
```

`pokeui` is no longer a static file server. Live JSON and frames come from
the wall over the overlay; the frames volume is unused on this path.

The wall grows one read API, `GET /v1/dashboard`. Existing farm routes
(`lease`, `heartbeat`, `finish`, `workers` ping, `specs`, `cancel`,
`/frame`) stay as they are. Runner behaviour does not change.

`GET /` on the wall itself remains the in-network debug table. The browser
does not use it.

### Deploy

On this host swarm ingress published ports are unreachable from outside,
which is why a default-bridge `docker run -p` was used. Attaching a
standalone container to the stack overlay also fails unless the network is
`--attachable`.

Put `pokeui` in `deploy/farm.yml` as a third service and publish its port
in **host mode** (binds the node, skips the ingress mesh). It then has
overlay DNS to `http://wall:8080` and a host port the browser can hit.
`make farm-up` stops starting a separate `pokefarm_ui` container.

The wall’s `-publish` directory is optional leftover: keep the publisher
code and tests, drop `-publish` from the stack command so the frames bind
mount is no longer required for the UI.

---

## 3. Layout

One page, no client router. The live Game Boy screen is the card.

```
┌─ bar: pokefarm · N running · N queued · N idle workers ─┐
│ [Queue a run]                                             │
├─ live ────────────────────────────────────────────────────┤
│  cards: screen (if running) · status · id · starter→dest │
│  seed · frame · map · xy · attempt n/3 · Cancel          │
├─ workers ─────────────────────────────────────────────────┤
│  chips: addr · idle | running <id> · seen Ns ago         │
├─ history ─────────────────────────────────────────────────┤
│  finished tiles: reason, dest, attempts                   │
└───────────────────────────────────────────────────────────┘
```

Click a live or history card to open a detail pane (trace / stop-so-far,
or finish reason + detail + last heartbeat trace). No save-states and no
dump JSON in the browser.

**Queue a run** is a panel (not a separate route) with the spec fields the
wall already accepts: `run_id`, `planner`, `starter`, `dest`, `seed`,
`fps`, `max_rounds`, `max_frames`. POST `/v1/specs`.

**Cancel** on queued / leased / running cards. POST `/v1/runs/{id}/cancel`.

Poll `GET /v1/dashboard` every 2s and patch the DOM. Scroll and the open
detail stay put. After a successful queue or cancel, fetch immediately.

Visual: Game Boy bezel cards on a dark olive ops board (LCD green,
cartridge red). One type pair, 8px rhythm. Distinctive, not a generic
dark dashboard.

---

## 4. Data

`GET /v1/dashboard` is one JSON snapshot of what the table already
rendered:

```json
{
  "now": 1756420000,
  "runs": [
    {
      "run_id": "route-2",
      "status": "running",
      "planner": "scripted",
      "starter": "charmander",
      "dest": "viridian pokemon center",
      "seed": 42,
      "fps": 60,
      "max_rounds": 3,
      "max_frames": 1000,
      "attempts": 1,
      "frame": 1200,
      "map": 12,
      "x": 5,
      "y": 6,
      "trace": "stepped north",
      "stop_so_far": "",
      "reason": "",
      "detail": ""
    }
  ],
  "workers": [
    { "addr": "10.0.1.5:8099", "run_id": "route-2", "seen_ago": "1s" }
  ]
}
```

- Runs stay in wall insertion order. The page splits live vs history on
  `status === "done"`.
- `map` is a JSON number; the UI prints `0x0c`.
- `run_id` on a worker is `""` while idle.
- No `worker_addrs`, no save-state, no dump files.

Frames stay `GET /frame?run=<id>&t=<now>` with `now` as the cache buster,
proxied by `pokeui`. Only `status === "running"` has a screen.

---

## 5. Errors

Page copy, not stack traces:

| Condition | UI |
|---|---|
| Overlay / wall down | Banner “wall unreachable”; form disabled; last good snapshot kept if any |
| POST specs 409 | “run already active” on the form |
| POST cancel 409 | “already finished” on the card |
| Other 4xx/5xx | wall `error` string on the form or card |
| Running run, no frame | empty bezel, not a broken-image icon |

`pokeui` is not an open proxy. Only:

- `GET /`
- `GET /v1/dashboard`
- `POST /v1/specs`
- `POST /v1/runs/{id}/cancel`
- `GET /frame`

Anything else is 404, including runner-only routes (`/v1/lease`,
heartbeat, finish). Upstream timeout or connect error is 502 with
`{"error":"wall unreachable"}`. Dashboard and frames are `Cache-Control:
no-store`.

`-wall` is required. There is no file-server fallback.

---

## 6. Tests

- Wall: `GET /v1/dashboard` round-trip for queued, running, done, and
  workers. Snake_case field names.
- `pokeui`: embed at `/`; allowlisted proxy; 502 when wall is down;
  no-store on dashboard and frames; `/v1/lease` is 404.
- Existing publish, farm-client, and wall lifecycle tests stay green.
- No browser e2e in this pass.

---

## 7. Out of scope

- Vue / Vite / Node in the farm image
- Enqueue defaults invented by the wall (empty seed is seed 0, as today)
- Serving finish dumps, save-states, or last-frame PNG from dumps
- LLM queue / max-runners controls (farm design §5, later)
- Changing heartbeat cadence, retry budget, or reaper behaviour
