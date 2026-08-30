# LLM Stats in Poke UI — Design

**Date:** 2026-08-30
**Status:** approved (maestro, 2026-08-30)

## Context

Each runner's watch page (port 8099) renders a statistics panel fed by
`runStats` (`cmd/pokepilot/stats.go`) → `m.TraceStats` → `/trace.json`:
round (N left), repeat picks, think last/avg, offered avg, tokens, rejected,
transport, fallbacks, the intent line, and the choices bars. It is the answer
to "what is this model DOING with its rounds" — the number that makes a
wandering run visible *while it is still wandering*.

The operator console (pokeui) polls the wall's `/v1/dashboard` and already
shows each run's question/decision ("Plan" block) and last trace — but none of
the stats. So the fleet-wide view is blind to exactly the signal the panel was
built for: ten wandering runs look fine card by card until someone opens each
runner's own page.

## Goal

The same LLM panel that 8099 shows, in poke ui: a compact line on every live
llm run card, and the full panel in the detail pane.

## Decisions

- **Push over the heartbeat, not pull.** The runner already computes the tally
  at ~1 Hz and already crosses this goroutine boundary for Question/Decision
  via `heartbeatSnap`. A per-run stats endpoint on the wall would double the
  console's traffic and add failure modes for data that is already in flight.
- **Typed wire struct, not `json.RawMessage`.** Every other heartbeat field is
  typed; an opaque blob would be the one untyped field on the wire, with no
  compile-time contract and no wall-side tests. The stats type is pure data
  (ints, strings, a choices list) and touches none of the packages `farm` is
  forbidden from importing, so it belongs in `farm`.
- **One type, aliased.** `runStats` becomes `farm.LLMStats`; `cmd/pokepilot`
  keeps `type runStats = farm.LLMStats` so stats.go, its tests, and the watch
  page's JSON keys do not move. The 8099 panel and pokeui render the same
  fields by construction.
- **Kept on finish, reset on retry.** A done run's final tally is the
  interesting number (it explains the outcome); a retried attempt starts
  fresh, exactly like Question/Decision/Trace.
- **Scope is poke ui.** The in-network debug grid and `Publish` output stay
  untouched.

## Data flow

1. **Wire (`farm/spec.go`).** `LLMStats` + `ChoiceCount` with the same JSON
   keys as today's `runStats`. `Heartbeat.Stats *LLMStats
   json:"stats,omitempty"` — nil for scripted runs and old runners, so nothing
   renders and old/new mixes freely (new runner → old wall: unknown field
   ignored; old runner → new wall: nil).
2. **Runner (`cmd/pokepilot`).** `statsPlanner` takes the `heartbeatSnap`
   (runFarmLLM passes it, runLLM passes nil — the local watch page is
   unchanged). `record()` pushes a copy via a new `snap.storeStats(...)`:
   value copy plus copied Choices slice, so the snap never aliases the live
   tally. `storeStatus` preserves `Stats` across ticks the same way it
   preserves Question/Decision, so a sample between asks cannot blank it. New
   leases already clear everything via `snap.store(farm.Heartbeat{RunID: ...})`.
3. **Wall (`cmd/pokewall`).** `Tile.Stats *farm.LLMStats`, set in
   `handleHeartbeat`; added to `tileRow` (dashboard JSON, `stats,omitempty`)
   and `persistedTile` (survives a wall restart). `settleRun` keeps it on the
   done path and nils it in the retry branch.
4. **pokeui (`cmd/pokeui/ui`).**
   - *Live card:* for llm runs with stats, a compact line under "progress":
     `round 1 (32 left) · rep 0/1 · think 4.4s avg`. Scripted runs and
     pre-first-ask llm runs render exactly as today.
   - *Detail pane:* a **Play** block after Plan, llm runs only, mirroring the
     8099 panel row-for-row with the same labels: round, repeat picks, think
     last/avg, offered avg, tokens, rejected, transport, fallbacks — plus the
     intent line and the choices bars (bar = row background, same trick as the
     watch page). Same warn conditions: repeats ≥ half of rounds after three
     rounds, and rejected/transport/fallbacks > 0 get the amber class. Done
     runs show their final tally in the same block under Outcome.

## Testing

- **farm:** heartbeat JSON round-trip with Stats set and unset.
- **pokepilot:** `statsPlanner` pushes the tally to the snap after each ask; a
  `storeStatus` tick preserves stats (and a new-lease store clears them).
- **pokewall:** heartbeat stores Stats and `/v1/dashboard` carries it; finish
  keeps it, retry clears it; state persistence round-trips it.
- **pokeui:** source-grep tests in the existing style asserting the new render
  hooks (stats line on cards, Play block rows, warn classes) exist.

## Verification

`go test ./...`, then a smoke run: wall + pokeui locally, post a spec and a
heartbeat carrying stats, confirm the card line and Play block render — and
that a scripted run shows neither.
