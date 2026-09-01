# Navigation Stall Guard Design

## Problem

`skill.GoTo` bounds failed route re-plans, but it does not bound successful map
transitions. A bad route can therefore alternate between maps indefinitely.
The agent-level frame budget cannot stop this because it is checked only after
the objective returns.

The observed failure alternates between Route 2 (`0x0d`) and the Viridian
Forest south gate (`0x32`). Regardless of the underlying route defect, one
navigation objective must never be able to run forever.

## Scope

Add a local safeguard to `skill.GoTo`. It aborts only the current navigation
objective and returns a typed error to its caller. It does not terminate the
process, choose a recovery objective, change map routing, or persist learned
geometry.

## Guard

`GoTo` records the player's `(map, x, y)` state when the call begins and after
every successful `Traverse`.

- If a state appears a second time, navigation has completed a cycle without
  getting closer to its destination. Return an error matching
  `ErrNavigationStalled` immediately.
- If 64 successful transitions occur without reaching the destination or
  repeating an exact state, return the same typed error as a hard fallback.

The error includes the reason, destination, repeated or final state, transition
count, and the recorded state trace so a stopped run is diagnosable.

The guard is per `GoTo` call. A battle still returns `ErrBattle`; `Travel`
handles it under its existing battle budget and begins a fresh route from the
post-battle world.

## Correctness Boundaries

The legitimate Route 2 forest detour remains valid. It enters Route 2 in the
southern band and later re-enters in the northern band, so the map ID repeats
but `(map, x, y)` does not.

Dynamic sprite positions remain fresh RAM observations. The guard records
player progress only and does not cache blockers or convert transient
collisions into geometry.

## Tests

Use a small pure state-tracker test to prove:

1. revisiting an exact `(map, x, y)` returns `ErrNavigationStalled` on the
   second occurrence;
2. revisiting a map at a different tile is allowed;
3. 64 unique successful transitions are allowed and the next transition
   returns `ErrNavigationStalled`;
4. errors include the destination, transition count, and state trace.

Keep existing ROM-backed Route 2, forest-gate, and full-suite tests green.
