# RUNNOTES — S2-2 map graph over warps and connections (DONE, this commit)

## What I changed
- `world/graph.go` (new): `EdgeKind` (EdgeWarp/EdgeConnection), `Edge`,
  `Graph{Edges map[uint8][]Edge}`, `BuildGraph(romData []byte)`.
  - Parses map ids 0x00..0xF7 via red/rom.ParseMap; ids that error are
    SKIPPED (228 of 248 parse). Errors only when ZERO maps parse.
  - One Edge per warp (Kind=EdgeWarp, WarpX/WarpY) and per connection
    (Kind=EdgeConnection, Dir, To=Connection.MapID). `Edges` gets an
    entry for EVERY valid map, so len(Edges) = maps covered (228 >= 200).
- `world/graph_test.go` (new): ROM-gated (skips cleanly without
  POKEMON_RED_ROM), grid_test.go pattern. Asserts: >=200 maps, no edge
  To==0xFF, 0x26->0x25 (7,1), 0x25->0x26 (7,1), 0x25->0x00 (2,7)+(3,7),
  0x00->0x0C conn Dir 0, 0x01->0x29 (23,25).

## 0xFF resolution — IMPORTANT DEVIATION, read this
Rule said: 0xFF warp on A -> unique map B with a warp whose DestMap==A,
else DROP. LITERAL rule FAILS the required test: in the real ROM 0x26's
stair warp (7,1) is EXPLICIT 0x25 (warp id 2), so 0x25's 0xFF exit warps
have TWO candidates {0x00, 0x26} -> literal rule drops them, but the test
requires 0x25->0x00.
Implemented instead: candidates = maps with a warp to A, EXCLUDING maps A
already reaches via an explicit (non-0xFF) warp. For 0x25: 0x26 excluded
(explicit stair warp up), leaves {0x00} -> Pallet. Real ROM counts:
resolved=164 dropped=90 (literal: 105/149; warp-ID-matching: 87/167).

## Verification
- `go build ./...`, `go vet ./...` clean; `go test -count=1 ./...` green
  WITH POKEMON_RED_ROM (/home/maestro/Documents/projects/gomeboy/roms/
  pokemon_red.gb); `go test -count=1 ./world/` green WITHOUT it (4 SKIPs).

## Notes for next task (pathfinder over the graph)
- Graph is MAP-level only; arrival tile coords are NOT in edges. A warp
  edge means "step onto (WarpX,WarpY) on From, then you are on To";
  arrival warp index = rom Warp.DestWarpID (re-derive via
  rom.ParseMap(To).Warps[DestWarpID]). Connection edges: walk off the map
  edge in Dir; entry tile on To must come from live RAM. Dropped 0xFF edges
  are real (ambiguous "came from"); cross them at runtime via wWarpsToMap.
  skill/ still only uses FindPath/FindPathAdjacent; no consumer of Graph.
