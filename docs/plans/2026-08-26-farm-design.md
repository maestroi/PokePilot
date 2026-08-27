# PokePilot farm — design

Status: agreed, not implemented.
Date: 2026-08-26.

A Swarm-local farm of N PokePilot runners and one wall. The game stays a
plain process. The wall is optional.

This does not change `emu/`, `skill/`, `agent/`, or `red/`. Those packages
must not import farm code.

---

## 1. Hard gates

1. **Pilot-only is the default.** `make run`, `go test ./...`, and an agent
   working a slice never need the wall, Swarm, or extra env. If
   `POKEPILOT_ORCH_URL` is unset, today's CLI is unchanged.
2. **Least in the way of PokePilot.** Farm behaviour lives in `cmd/pokepilot`
   plus a small HTTP client. No Docker API in the runner. No Swarm knowledge
   in the runner.
3. **Same repo.** `cmd/pokepilot` (runner) and `cmd/pokewall` (orchestrator).
   One stack file. Split only if a second agent wants the same farm.

Same rule as MCP in `docs/DESIGN.md`: core is already Go; the extra surface
is opt-in.

---

## 2. Split

```
cmd/pokepilot   play the game; optional farm client
cmd/pokewall    UI, specs, dumps, LLM queue, max/scale
Swarm           how many worker containers exist
```

The runner never talks to Docker. The wall never emulates.

---

## 3. Why not push-only

Push (heartbeat + dump) is not enough to set seed and settings from the
wall. Swarm replicas share one service env, so `-seed` cannot differ per
tile, and a new seed would mean kill/recreate via the Docker API.

The runner therefore **reads a spec** as well as pushing status. That is
how seed, planner, starter, dest, and budget change without Swarm in the
game process.

Flags remain the laptop path. A leased spec is the same fields, filled by
the wall.

---

## 4. Runner surface (the only PokePilot change)

One env var: `POKEPILOT_ORCH_URL`. Unset → run once from flags, exit as
today.

Set → a loop around the existing run:

1. `GET /v1/lease` → spec (or wait/retry if none)
2. Apply spec to the same variables flags already set (`seed`, `planner`,
   `starter`, `goto`, budgets, fps)
3. Run exactly as today (boot, scripted or llm, same stuck/error/budget
   stops)
4. Heartbeat while running
5. `POST /v1/runs/{id}/finish` with the dump, then lease again

Stop a run: the heartbeat response can say `cancel`. The runner finishes
the dump and leases the next spec (or idles). Hard kill is Swarm's job,
not the protocol.

Idle workers keep leasing. Autostart-after-stuck is "the wall has another
spec ready," not a container restart.

### Spec (mirrors existing flags)

```
run_id, seed, planner, starter, dest, fps, max_rounds, max_frames
```

Seed is applied the same way `-seed` already works: burn idle frames after
boot. The wall picks the value (new random on recycle, or a number the
operator typed). The runner does not invent a seed when leased.

### Heartbeat (small)

Map, coords, last trace line, stop-so-far, optional JPEG. Cadence on the
order of a second, not every frame.

### Finish dump (why it died)

Stop reason (`stuck` / `error` / `budget` / `done`), last observation,
last objective + offered list, last N trace lines, save state, last frame.
This is the bundle `docs/DESIGN.md` §3.10 already described; the farm
ships it to the wall instead of only leaving it on disk.

The tile goes amber, keeps the dump in history, and the slot gets a new
seed.

---

## 5. Wall

- Grid of tiles (live card + optional tiny screen). Click → that run's
  trace and dump history.
- Settings per new spec; start / stop / cancel.
- **Max runners** set by the operator. Auto sits under that: if the LLM
  queue wait climbs (or the server 429s/times out), stop handing out `llm`
  leases and drop active LLM runners toward 5. Scripted leases ignore this
  and can stay at the max.
- All planner HTTP from farm workers goes through the wall, so there is
  one queue and one in-flight count. Laptop `make run-llm` still talks to
  the model directly (no wall).
- Dumps stored on a volume the wall owns. No ROM in git; same as fixtures.

Scale of the Swarm service is "how many long-lived workers exist." The
wall should not need to create a container per run.

---

## 6. Local Swarm

Same compose/stack as prod, replica count 2 for a smoke test:

- Overlay: workers reach `http://pokewall:8080`
- ROM: bind mount + `POKEMON_RED_ROM`
- LLM: env pointing at the host (or a sidecar). Workers use the wall as
  proxy when leased; the wall uses `POKEPILOT_LLM_URL`.
- `docker swarm init` once; `docker stack deploy -c deploy/farm.yml pokefarm`

No extra repo, no extra module. Image: this repo, two entrypoints.

---

## 7. Out of scope until the farm exists

- In-process N-goroutine mode as a substitute for Swarm
- Wall talking to the Docker API for settings
- Changing the agent loop, skills, or watch page contract
- Training on dumps (replay/inspect first, as in §3.8b)

---

## 8. What success looks like

- Unset URL: existing tests and `make run` pass with no farm code on the
  path that matters.
- Set URL, wall down: runner waits on lease, does not crash the game loop,
  does not take flags as a farm spec by accident.
- Stuck: dump appears on the wall, tile marked, next lease has a new seed.
- LLM hot: new `llm` leases slow or stop; scripted tiles keep going.
