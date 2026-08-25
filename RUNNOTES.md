# RUNNOTES — S4-2 Observation (done, all tests green with ROM)

## What changed
Commit "agent: the decoded observation a planner is allowed to see".

- `agent/observe.go` (new): `Observation` + `Observe(m *emu.Emu)`. Built ONLY
  from existing red/state decoders (state.Read, state.Controllable,
  state.HasEvent). No new RAM addresses, no new event constants.
  - MapName is "" for now: red/state decodes NO map names (see below).
  - Events = the 7 existing state.Event constants that are set, via their
    String(); order = declaration order in red/state/progress.go.
  - Badges via Progress.Has over the 8 existing Badge constants.
  - List fields (Party/Badges/Events) are non-nil empty: JSON is [] not null.
    Field names ARE the JSON names — stable contract for S4-5.
  - No String()/prompt method on Observation (that is S4-5's, next to the
    prompt).
- `agent/observe_test.go` (new): reuses loadFixture from objective_test.go.
  - Fresh boot: Controllable, PartyCount 0, Map == what state decodes.
  - After KindStarter Execute: PartyCount 1, Events has
    "BattledRivalInOaksLab".
  - JSON round-trip of a fully populated struct; no ROM needed, runs always.

## Verified
- `go build ./...`, `go vet ./agent/...` clean. Without ROM: fixture tests
  SKIP cleanly. With POKEMON_RED_ROM: all 7 agent tests PASS, full
  `go test ./... -skip TestGoToViridianPokecenter` green.

## Left out of Observation (red/state does not decode it yet)
- MapName: no map-name decoder anywhere (red/rom has map headers but no name
  table; skill.Place is name->map only). Needs a decoder + ROM name table;
  do NOT guess an address. Everything else in the struct is filled.

## Gotchas / next
- TestGoToViridianPokecenter still red on Route 1 (plan fdc1544f); skip it
  in full-suite runs.
- knownEvents in observe.go is an explicit list (eventNames is unexported);
  keep it in sync if red/state gains event constants.
- S4-5: send Observation + Objective.String() to a model; add prompt
  rendering next to the prompt, not on Observation.
