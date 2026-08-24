# RUNNOTES — S2-1 pathfinder solid tiles (DONE, this commit)

## What I changed
- `world/path.go`:
  - `FindPath`: start tile treated as walkable regardless of the grid
    (local special case inside FindPath; Grid never mutated). A solid
    start means the first step leaves it; A* expansion still requires
    walkable neighbours, so a solid start can never be re-entered.
    Destination check unchanged; signature unchanged.
  - New `FindPathAdjacent(g *Grid, sx, sy, tx, ty int, blocked map[[2]int]bool)
    ([]Step, Step, error)`: BFS from the start (same solid-start case),
    then picks the reachable walkable unblocked orthogonally-adjacent
    neighbour of (tx,ty) with the shortest path; ties break lowest y,
    then lowest x. Returns path + final push Step into (tx,ty). Target
    need not be walkable. ErrNoPath if no neighbour reachable (also if
    start/target out of bounds or start blocked).
- `world/path_test.go`: table tests on hand-built Grids (no ROM):
  - TestFindPathFromSolidStart (2 cases; first step leaves solid start,
    one case forces a specific first step).
  - TestFindPathAdjacent (5 cases: solid warp push, walkable target,
    start already adjacent -> empty path + push, solid start, all
    neighbours solid -> ErrNoPath).
  - TestPathCallsDoNotMutateGrid: solid tiles still solid after both.

## Why
Measured: warp tiles are solid (Red's House warps (7,1)/(2,7)/(3,7) all Walkable==false)
and the player stands on a solid tile after arriving (1F (7,1)); old FindPath returned ErrNoPath.

## Verification
- `go build ./...`, `go vet ./...` clean; full `go test -count=1 ./...`
  passes with POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb.
- `go test -count=1 ./world` passes WITHOUT POKEMON_RED_ROM.
- Gotcha: go vet composites rejects multi-name struct literal keys (`sx, sy: 0, 0`); use explicit per-field keys.

## Notes for next task
- skill/ still only calls FindPath; FindPathAdjacent is new API awaiting
  its consumer. Push model: path to neighbour + one Step into the warp;
  after the push the player is ON the solid warp tile (next call starts
  solid). blocked honoured for start and neighbours, not the target.
