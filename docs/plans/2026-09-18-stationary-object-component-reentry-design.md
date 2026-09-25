# Stationary-object component re-entry design

## Problem

Farm runs `run-10c8zv1z0rkui3mv2s74i1odim` and
`run-33v7g1kjm2mzw5hqhz77rnfcz` stop at Route 13 `(11,4)`. A visible,
stationary trainer at `(12,4)` splits the map into two walking regions. The
tile-only route graph does not include that object, so it classifies the
Route 14 -> Route 13 row-8 crossing as a return to the trapped region. GoTo's
anti-bounce preference then suppresses the useful crossing and routes toward
Route 15/Cycling Road instead.

The exact round-three state from the second run proves the intended route:
Route 13 -> Route 14 -> Route 13 row 8 -> Route 12 -> Lavender -> Route 8 ->
Underground Path -> Route 7 -> Celadon. It reaches Celadon controllably with
no active battle.

## Invariant

Within one GoTo transaction, component identity must include stationary
objects that were positively observed on a loaded map. A later border crossing
that lands in a different measured component is forward progress, not a map
bounce.

Moving sprite positions remain ephemeral and must never become learned
geometry. An observation must not survive beyond the current GoTo call, and a
map must replace its prior overlay when it is loaded again.

## Design

Build each current-map routing overlay from live block geometry plus objects
whose current sprite observation satisfies all three checks:

- the live sprite slot maps to the same 1-based ROM object;
- that ROM object's movement is `MovementStay`; and
- the live sprite remains on that object's ROM home tile.

This excludes moving sprites, displaced objects, and ROM objects that are
currently hidden, including the case where a different moving sprite happens
to occupy a hidden stationary object's home coordinate. Apply the overlay to
the previous per-GoTo graph snapshot rather than rebuilding from the immutable
graph on every leg. WithMapGrid remains immutable: each call returns a new
snapshot, and reloading a map replaces that map's earlier measurement.

No map ids, named routes, story cases, or error-string policy enter generic
routing.

## Failure behavior

Genuine absence of a route continues to return the existing typed
`world.ErrNoRoute` blockage. The change only prevents a false no-route caused
by forgetting a positively observed component split.

## Verification

- A deterministic ROM-backed graph regression proves Route 14 -> Route 13 row
  8 changes from same-component to fresh-component when the visible stationary
  trainer participates in the topology snapshot.
- A focused helper test proves moving and hidden objects are not persisted.
- The exact farm-state replay must make ordinary TravelFlee reach Celadon via
  Lavender/Underground Path.
- Run focused `world`, `skill`, and `agent` tests plus `make test-short`; the
  known Pokewall S3-credential baseline failure is reported separately.
