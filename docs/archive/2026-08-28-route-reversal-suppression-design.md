# Immediate Route Reversal Suppression Design

## Problem

The navigation-stall guard exposed this cycle while travelling from Route 2's
southern band to Pewter:

```text
0d(7,48) -> 32(4,7) -> 0d(3,44) -> 32(4,7)
```

The first proposed fix was to retain the full route returned by
`world.FindRouteAvoiding`. A real-ROM measurement disproved that design: after
all first hops unreachable from Route 2 south are excluded, the graph itself
returns `0x0d -> 0x32 -> 0x0d -> 0x02`. Retaining that suffix retains the loop.

The graph is map-level and does not know that the return from the south gate
lands in the same Route 2 band. `GoTo` does know which map it just left and can
reject the immediate reversal without inventing persistent geometry.

## Scope

Change only `skill.GoTo`'s next-route decision. Preserve the map graph, tile
pathfinder, live sprite blockers, per-tile failed-leg bans, battle handling,
and `ErrNavigationStalled` safeguard.

## Immediate Reversal Rule

After a successful transition `A -> B`, `GoTo` remembers `A` as the previous
map. While selecting the next route from `B`, it adds every first-hop edge
`B -> A` to the existing `blockedHere` set.

The suppression lasts until another map transition succeeds or the `GoTo` call
returns. It is not stored as world geometry and does not survive a battle;
`Travel` starts a fresh `GoTo` from the measured post-battle state.

For the measured failure:

1. Route 2 south (`0x0d`) exhausts its locally unreachable north-band exits
   and successfully enters the south gate (`0x32`).
2. From `0x32`, both return edges to `0x0d` are suppressed.
3. The real graph then returns the measured route
   `0x32 -> 0x33 -> 0x2f -> 0x0d -> 0x02`.
4. Each later transition suppresses only the map just left, so forward progress
   through the forest remains available.

If every non-reversing first hop is unavailable, navigation reports no route or
its existing typed leg failure rather than deliberately undoing its last step.
The stall guard remains defense in depth for longer or non-adjacent cycles.

## Correctness Boundaries

- Suppression is local to one arrival and one `GoTo` call.
- All parallel edges back to the previous map are suppressed; blocking only one
  of the south gate's paired warps would still allow the other to reverse.
- A map may be revisited later after forward transitions; only immediate
  `A -> B -> A` reversal is excluded.
- Dynamic sprite blockers remain fresh RAM observations and are never recorded.
- No region graph, persistent blocker memory, route hardcoding, or global graph
  rule is introduced.

## Tests

Add a pure Route 2-shaped graph regression where `0x32` offers paired reverse
edges to `0x0d` before the forest edge. Without suppression the shortest route
reverses; after suppression both reverse edges are blocked and the route begins
`0x32 -> 0x33 -> 0x2f -> 0x0d -> 0x02`.

Also prove that only immediate reversals are excluded and the block set remains
local. Keep the navigation-stall tests, existing Route 2 graph regression,
forest-gate tests, battle-on-warp recovery, and complete ROM-backed suite green.
