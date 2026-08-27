# RUNNOTES — S5c-3: Traverse reaches the warp tile the pathfinder can actually reach

## What changed (for S5c-6: prove the badge)
- skill/warp.go: Traverse's warp leg no longer walks to the graph edge's tile. `warpTarget` (new, unexported) considers EVERY warp tile on the current map that leads to the edge's destination (0xFF/LAST_MAP resolved via the edge the graph already resolved) and picks the first whose FindPathAdjacent route from the player's CURRENT position steps on no other warp tile — entering a warp fires it mid-walk. Re-derived per re-plan; nothing cached as a map/warp property.
- skill/gym_test.go: crossGate DELETED. Its two call sites are ordinary travel legs: leg 2 travels to Place("viridian forest") (0x33 17,43), leg 4 travels straight to the gym (0x2F -> 0x0D -> 0x02 -> 0x36 in one Travel). travelFightsThrough and dismissDialogue stay (dialogue is slice 6).
- New tests: skill/warp_internal_test.go (package skill; asserts the chosen tile: (5,1)->(5,0) and (4,2)->(4,0) on BOTH gates, proving per-position consultation) and TestTraverseGateWarp in skill/warp_test.go (ROM-backed: Traverse on the (4,0) edge crosses and lands at (17,47) — the (5,0) warp's landing; (16,47) would mean (4,0) fired).
- Verified: go test ./... -skip TestGymBoulderBadge with POKEMON_RED_ROM: 162 pass, 0 fail; only skip is TestProbe's permanent PROBE_MAP gate.

## TestGymBoulderBadge status (S5c-6's job)
Gates are no longer the blocker: legs 1-2 now cross the south gate automatically and forest training ran ("trained the lead to level 8 in 2 battles"). It now fails exactly where slice 5 left it: leg 3's forest walk opens the "Hey, wait up!" box at 0x33 (1,18) and `dismissDialogue: text box did not close` (600 frames of A). Dialogue recovery is slice 6's scope.

## Probe output (paste into commit message)
```
$ POKEMON_RED_ROM=... PROBE_MAP=0x32 PROBE_AT=5,1 go test ./skill -run '^TestProbe$' -v
    map 0x0032: 10x8, standing (5,1) walkable=true, 0 tile(s) treated as occupied
    north edge: nearest reachable tile (5,0)
    grid window (@ = you, # = wall, x = occupied):
        y=  0    #####.####
        y=  1    .....@....
        y=  7    #........#
$ POKEMON_RED_ROM=... PROBE_MAP=0x2F PROBE_AT=5,1 go test ./skill -run '^TestProbe$' -v
    map 0x002f: 10x8, standing (5,1) walkable=true, 0 tile(s) treated as occupied
    north edge: nearest reachable tile (5,0)
    grid window (@ = you, # = wall, x = occupied):
        y=  0    #####.####
        y=  1    .....@....
        y=  7    #........#
```
(4,0) is a wall on both gates; (5,0) is the only walkable exit. Warp tables: 0x32 (4,0),(5,0)->0x33; 0x2F (4,0),(5,0)->LAST_MAP=0x0D.

## Carried over
The verbatim preserved TestTravelToPewter (S5b-6, still blocked by the Route 2 ledge split in world/) is in `git show 723fa6d:RUNNOTES.md` if a future task needs it.
