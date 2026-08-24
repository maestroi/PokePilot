# RUNNOTES — S2-3 cross-map route search (DONE, this commit)

## What I changed
- `world/route.go` (new): `ErrNoRoute` and `FindRoute(g *Graph, from, to uint8)
  ([]Edge, error)`. Plain BFS: FIFO queue of map ids, first-visit wins, edges
  explored in slice order (deterministic — same call, same route). Parent-edge
  map reconstructs the route; fewest map transitions guaranteed. from == to
  returns an empty slice, nil error. No tile-level pathfinding, no A*.
- `world/route_test.go` (new): ROM-gated via loadGraph (skips without
  POKEMON_RED_ROM). Tests: 0x26->0x26 empty; 0x26->0x25 exactly one EdgeWarp
  at (7,1); 0x26->0x29 (bedroom -> Viridian PokeCenter) succeeds, asserts
  contiguity (edges[0].From==0x26, each From == prev To, last To==0x29) and
  passage through Route 1 (0x0C) via an EdgeConnection; a map id no edge
  reaches (found dynamically) returns ErrNoRoute.

## Verification
- `go build ./...`, `go vet ./...` clean; `go test -count=1 ./...` green WITH
  POKEMON_RED_ROM; `go test -count=1 ./world/` green WITHOUT (skips).
- Real-ROM 0x26->0x29 route (verified by dump): 0x26 -warp(7,1)-> 0x25
  -warp(2,7)-> 0x00 -conn dir 0-> 0x0C -conn dir 0-> 0x01 -warp(23,25)->
  0x29. 5 edges.

## Notes for next task
- FindRoute is map-level only: it says WHICH edges to take, not where you
  land on the destination map. Arrival coords still come from live RAM at
  execution time (warp: rom Warp.DestWarpID; connection: walk off edge in
  Dir). skill/ does not consume it yet.
- Graph deviations from S2-2 still in effect (0xFF resolution rule, 228 maps
  covered, 90 dropped edges). If a route is missing at runtime, suspect the
  graph, not FindRoute.
- Unreachable map ids exist in the ROM (test finds one dynamically); a map
  can be a node with edges out but nothing pointing in.
