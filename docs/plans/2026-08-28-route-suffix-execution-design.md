# Route Suffix Execution Design

## Problem

`world.FindRouteAvoiding` can return a correct multi-leg route that revisits a
map at a different tile. Route 2 requires exactly this shape:

```text
Route 2 south -> south gate -> Viridian Forest -> north gate
-> Route 2 north -> Pewter
```

`skill.GoTo` currently executes only the first edge and then discards the
remaining route. From the south gate it performs a fresh shortest-path search,
which chooses the reverse warp back to Route 2 and recreates the impossible
direct attempt to Pewter. The navigation-stall guard now terminates this cycle,
but the route executor still needs to follow the correct route it already
computed.

## Scope

Change only `skill.GoTo`'s map-route execution policy. Preserve the map graph,
tile pathfinder, live sprite blockers, per-tile failed-leg bans, battle handling,
and `ErrNavigationStalled` safeguard.

## Route Cursor

`GoTo` keeps a pending route suffix across successful map transitions.

1. When there is no pending suffix, call `world.FindRouteAvoiding` using the
   failed legs recorded for the player's current `(map, x, y)`.
2. Execute the first pending edge with `Traverse`.
3. On success, verify live RAM reports the edge's destination map, remove that
   edge, record the transition in the stall guard, and continue with the next
   pending edge.
4. If the live map does not match the next edge's source, discard the suffix
   and compute a new route from the measured world.
5. If a leg returns `ErrLegUnwalkable`, record the existing per-tile ban,
   discard the suffix, and replan from the same measured state.
6. Battles and other failures retain their current typed return behavior.

`Traverse` already rejects an arrival map different from `edge.To`; the source
check remains a defensive boundary for future callers or transition behavior.

## Correctness Boundaries

- Route 2 can re-enter map `0x0d` because the pending route distinguishes the
  southern and northern arrivals by the sequence selected before the detour.
- A dynamic NPC collision remains owned by `Traverse`, which re-reads RAM and
  replans tile movement locally.
- A battle aborts `GoTo`; `Travel` resolves it and starts a fresh route from the
  post-battle state.
- Exact state repetition and the 64-transition ceiling remain terminal for the
  current navigation objective.
- No region graph, persistent blocker memory, or map-specific route hardcoding
  is introduced.

## Tests

Add a pure route-cursor regression with the Route 2 graph shape. After the
direct north edge is blocked, it must retain and yield every edge of the forest
detour even though recomputing from the south gate would choose the reverse
warp. Also test that an unexpected current map discards the suffix and that a
leg failure clears it before replanning.

Keep the existing Route 2 graph regression, navigation-stall tests,
forest-gate tests, battle-on-warp recovery, and complete ROM-backed suite green.
