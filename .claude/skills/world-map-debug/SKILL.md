---
name: world-map-debug
description: Use when investigating map, routing, warp, collision, world-graph, or location failures. Start with RomPilot World Explorer for spatial context and a shareable failure link, then use the deterministic probe for exact walkability/state questions and worldverify for graph-wide inconsistencies. Never hand-reconstruct a collision grid from the UI.
---

# World map debugging

Use the public World Explorer as the orientation layer for map failures. It can show the decomp-rendered map, named connections and warps, static people/trainers/items/signs, an optional live run overlay, and the Kanto atlas.

It is not the authority for exact movement. Collision/path questions still belong to skill/probe_test.go, and graph-wide consistency belongs to cmd/worldverify.

The normal order is:

    run / issue / failing state
            ↓
    World Explorer       where is this, what connects here, what looks suspicious?
            ↓
    TestProbe            can this exact tile/path/first leg actually be traversed?
            ↓
    worldverify          is the compiled world graph itself inconsistent?

## 1. Get the authoritative location

For a farm run, inspect:

    pokepilot_get_run_debug(run_id)

Prefer final progress/debug evidence over the run's top-level map when they disagree. The top-level location can be stale.

If a failing .state is available, use it rather than guessing:

    POKEMON_RED_ROM=roms/pokemon_red.gb \
    PROBE_STATE=/absolute/path/to/failure.state \
    go test ./skill -run '^TestProbe$' -v

That prints the live map and coordinates from RAM. A save-state-derived location is stronger evidence than a screenshot or an old heartbeat.

## 2. Open a stable World Explorer link

Public Explorer lives at:

    https://rompilot.app/world

Useful query parameters:

- map=ROUTE_13 — map name; hex/decimal ids also work.
- x=49&y=8 — diagnostic coordinate.
- debug=1 — exact semantic/collision view and technical map metadata.
- run=<run-id> — attach a public live run as an overlay.
- follow=1 — continuously follow that run's current map.
- atlas=1 — open the Kanto overview.

Examples:

    https://rompilot.app/world?map=ROUTE_13&x=49&y=8&debug=1
    https://rompilot.app/world?run=<run-id>&map=ROUTE_13&x=49&y=8&debug=1
    https://rompilot.app/world?run=<run-id>&follow=1&atlas=1

For a failure report, attach the run but do not set follow=1. That keeps the diagnostic map/coordinate stable while still showing the run overlay when it is on that map. Use follow=1 only when intentionally watching a live agent move.

Put the stable World Explorer URL in the issue/PR diagnosis when a location is material. It lets a human open the same map immediately.

## 3. What Explorer can answer

Use Explorer to answer visual/spatial questions quickly:

- Which named map is this?
- What maps connect north/south/east/west?
- Which named warp does this door/stair/cave entrance reach?
- Is the suspicious coordinate near a seam, warp, ledge, building, NPC, item, or trainer?
- Does the decomp-rendered map make the reported route obviously suspicious?
- Where is this map in the Kanto atlas?
- If a public run is attached, where is its player/trail/current sprite overlay?

Explorer mode uses the decomp block/tile artwork when available. Debug mode uses PokePilot's semantic geometry.

### Do not treat artwork as collision proof

The rendered tiles are presentation. A tree, ledge, floor tile, doorway, or border that looks traversable/non-traversable does not prove the runtime collision answer.

Runless people/trainers/items shown by Explorer come from static decomp object definitions. They are useful landmarks, not proof of the current RAM position of a moving sprite. A live run's sprite overlay is the current observation.

If the diagnosis contains words like walkable, reachable, blocked, shortest path, or can get to, run the probe.

## 4. Measure exact local geometry

Do not copy/read the full collision grid into context. Use the permanent probe:

    POKEMON_RED_ROM=roms/pokemon_red.gb \
    PROBE_MAP=0x0c PROBE_AT=15,13 \
    go test ./skill -run '^TestProbe$' -v

Common forms:

    # Can I get from this standing tile to one exact tile?
    PROBE_MAP=0x0d PROBE_AT=49,8 PROBE_TO=50,8 \
      go test ./skill -run '^TestProbe$' -v

    # Does a current sprite blocker change the answer?
    PROBE_MAP=0x0d PROBE_AT=49,8 PROBE_TO=50,8 \
    PROBE_BLOCK='50,8' \
      go test ./skill -run '^TestProbe$' -v

    # What route/transition legs will Travel use?
    PROBE_MAP=0x0d PROBE_ROUTE=0x2f \
      go test ./skill -run '^TestProbe$' -v

    # Ask from the exact failed RAM state.
    PROBE_STATE=/tmp/failure.state PROBE_ROUTE=0x02 \
      go test ./skill -run '^TestProbe$' -v

The probe reports a small local window, edge reachability, nearby object home tiles, warps/connections, and route legs. Read its answer instead of simulating movement by hand.

## 5. Escalate graph-wide problems to worldverify

Use worldverify when the symptom suggests the exported/compiled world itself is wrong rather than one local path:

- dead entry/exit ports;
- an outdoor seam attached to padding instead of walkable ground;
- unreachable maps/components;
- inconsistent connection capabilities;
- suspicious duplicate/copy-map topology.

Example:

    go run ./cmd/worldverify -rom ./roms/pokemon_red.gb

Use the smallest relevant worldverify flags when available. Do not replace a focused local probe with a full verifier run if the question is only whether one tile is reachable.

## 6. Read the owning game data when needed

Explorer and probes tell you what is wrong. The vendored decomp often tells you why.

For Red, use pokered/; for Yellow, use pokeyellow/. Read map headers, object definitions, scripts, and transition data at the game-specific adapter layer. Follow docs/ARCHITECTURE.md: do not patch generic routing with a named Red map because one map exposed a missing capability/transition concept.

Useful distinction:

- static map/block/warp/object fact -> decomp/ROM adapter;
- current player/sprite/script state -> RAM / failing .state;
- path/reachability answer -> probe;
- global topology invariant -> worldverify;
- visual orientation/shareable diagnosis -> World Explorer.

## 7. Failure write-up checklist

For a map-related defect, leave the next investigator these facts when known:

    run:       <run id>
    build:     <runner revision>
    location:  <friendly map> (<map id>) @ x,y
    world:     https://rompilot.app/world?map=<NAME>&x=<X>&y=<Y>&debug=1
    state:     <artifact / local repro state>
    probe:     <one-line measured result>
    graph:     <worldverify finding, only if relevant>
    root cause:<owning invariant/data layer>

Do not paste giant collision grids, full verbose traces, ROM bytes, or save-state bytes into issues or agent context.

## Rules

- Read AGENTS.md and, before gameplay/runtime architecture changes, docs/ARCHITECTURE.md.
- Never infer exact collision from the graphical map.
- Never infer a moving sprite's current position from its static decomp POI.
- Never make a second live run your reproduction of the first; use the failed state.
- Never hardcode named-map exceptions into generic routing/planning.
- Never commit ROMs, saves, or .state files.
- Prefer one stable World Explorer deep link plus one measured probe result over paragraphs of spatial speculation.
