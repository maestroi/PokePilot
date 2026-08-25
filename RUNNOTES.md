# RUNNOTES — S4-1 typed Objective (done, all tests green with ROM)

## What changed
Commit "agent: a typed objective that dispatches to the skills".

- `agent/doc.go` (new): package doc; states the one-way dependency.
- `agent/objective.go` (new): `Kind` (KindGoTo/KindTalk/KindStarter),
  `Objective{Kind,Place,X,Y,Note}`, `Execute(m, romData, o) error`,
  `Objective.String()`.
  - KindGoTo: skill.Place(o.Place) then skill.GoTo. Unknown place is an
    error naming it (%q); no default fallback.
  - KindTalk: skill.Face(o.X,o.Y) then skill.Talk.
  - KindStarter: skill.GetStarter(StarterSquirtle, StatAwareMove(romData)).
    No extra idempotency guard (GetStarter is already idempotent).
  - Every error wraps with `agent: <o.String()>: ...` so failures in a
    long loop are attributable. Unknown Kind is an error, not a no-op.
  - String() is plain/stable: "go to <place>", "talk at (x,y)",
    "take a starter", "unknown kind N". S4-5 shows these to the model.
- `agent/objective_test.go` (new): ROM-gated via fixture.Load
  (reds_bedroom), plus TestString which needs no ROM.

## Verified
- `go build ./...`, `go vet ./agent/...` clean.
- With POKEMON_RED_ROM set: all 4 agent tests PASS (Starter 9.3s,
  GoToPallet 7.8s, UnknownPlace, String). Full `go test ./... -skip
  TestGoToViridianPokecenter` green.
- Without ROM: 3 tests SKIP cleanly, TestString still PASSes.
- Dependency direction: `go list -deps ./skill | grep pokepilot/agent`
  is empty; skill/*.go has no agent import. agent imports skill only.

## Gotchas for next task
- TestExecuteGoToPallet walks from Oak's lab (0x28) back to Pallet Town
  (0x00) via the map graph — works on main today.
- Still do NOT touch Route 1 (0x0C) or "viridian pokemon center" walks:
  TestGoToViridianPokecenter is red on the Route 1 collision bug
  (plan fdc1544f), someone else's task.
- Fixture cache: agent tests generate their own agent/testdata/fixtures
  (gitignored); first run pays one full boot (~10 s).
- Note field on Objective is display-only; never parse it.

## Next task
- S4-5: put Objective.String() lines in front of the model. Keep the
  String() output stable — it is now pinned by TestString.
