# RUNNOTES — S3-1 (done)

## What changed
Commit d552219 "state: read named story event flags from wEventFlags".

- `red/state/progress.go`: added `Event` (uint16 bit index into wEventFlags),
  the six named constants (indices 0, 33, 34, 35, 36, 38), `HasEvent(m, e)`
  which reads `sym.EventFlags + uint16(e)/8` and tests bit `e%8`, and
  `Event.String()` (named constants; `unknown(N)` otherwise).
- `red/state/progress_test.go` (new): synthetic-Mem table tests, no ROM.
  Explicit byte/bit arithmetic asserts for EventGotStarter (34 -> 0xD74B bit 2)
  and EventFollowedOakIntoLab (0 -> 0xD747 bit 0).

## Why
Slice 3 drives the story past the Pallet gate; every skill in the slice
terminates on one of these flags. Indices are DERIVED from
`const/const_skip` in pokered `event_constants.asm`, not measured on the ROM —
if a later task sees a flag misbehave, suspect the index first (one-line fix).

## Verification
`go build ./...`, `go vet ./...` clean; `go test -count=1 -skip
TestGoToViridianPokecenter ./...` all green. The skip is the known red test
that stays red until the last task in this plan.

## Must know for next task
- `HasEvent` is in package `state`, exported; no Cutscene helper was added
  (out of scope for S3-1). skill/, world/, emu/ untouched.
- The Pallet gate is `EVENT_FOLLOWED_OAK_INTO_LAB` (bit 0) — the flag a
  story-driving task will check first.
- ROM path (if a task needs one):
  /home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb
