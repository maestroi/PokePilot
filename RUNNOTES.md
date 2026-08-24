# RUNNOTES — S2-5 Traverse one graph edge (DONE, this commit)

## What I changed
- `skill/warp.go` (new):
  - `Traverse(m, romData, e world.Edge) error` — executes one graph edge.
    1. Refuses if RAM CurMap != e.From.
    2. Parses e.From, builds its Grid (rom.ParseMap + world.Build).
    3. EdgeWarp: FindPathAdjacent to (WarpX,WarpY), WalkPath, then HoldUntil
       the push button (budget crossBudget=180) until CurMap != e.From.
    4. EdgeConnection: edgeTarget picks the reachable walkable tile on the
       edge row/col (dir 0/1/2/3 = N/S/W/E), shortest path, ties lowest y
       then x (scan order + strictly-shorter replacement). WalkPath, then
       hold the edge direction until CurMap changes.
    5. StepUntil (arriveBudget=600) for state.Controllable (asserts non-zero
       map dims — the positive fact), then verifies CurMap == e.To.
  - Errors name the edge, current map, and player coords on every failure
    path (no screenshot needed).
- `skill/warp_test.go` (new): TestTraverseWarpChain on reds_bedroom —
  BuildGraph, the single 0x26->0x25 warp, assert CurMap 0x25, player (7,1),
  dims non-zero, Controllable; then a 0x25->0x00 warp, assert CurMap 0x00
  and Controllable. Second leg starts on solid tile (7,1) — covers the
  solid-start pathfinder fix.

## Findings the next task needs
- The 0x25 -> 0x00 door is TWO tiles wide: warps (2,7) and (3,7), both
  DestMap 0xFF resolved to 0x00. The graph therefore carries two warp edges
  0x25->0x00 (and any two-tile door produces duplicate edges). Multi-leg
  routing (S2-6) must dedupe or pick deterministically; the test picks
  lowest WarpY then WarpX.
- 0x25 also has warp (7,1) -> 0x26 (stairs back up). Arrival tile after the
  0x26->0x25 warp is (7,1), a warp/solid tile on 0x25.
- HoldUntil releases the button the frame the map flips; player never
  re-walks on the destination map. 180-frame cross budget is ~1.5x the
  measured ~120 frames.

## Verification
- `go build ./...`, `go vet ./...` clean.
- `go test -count=1 ./...` green WITHOUT POKEMON_RED_ROM (skips) and WITH
  (all ok, skill 5.8s).
- ROM at /home/maestro/.cache/pokered/pokered.gbc via POKEMON_RED_ROM.
