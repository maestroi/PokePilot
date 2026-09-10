# Slice 4 draft plan — sessions, parallel runs, and experiments

Status: **drafted, not published.** Depends on slice 3 and on the trace work
landing first (the session router reuses that handler).

Goal: stop running one careful playthrough and start running many cheap ones.
This is the infrastructure DESIGN.md 3.8b already assumes when it says a useful
decision corpus needs "many cheap unattended runs rather than one careful one".

---

## Why this is not a side quest

Three things in 3.8b are blocked without it:

1. **Counterfactual rollouts need N runs per branch.** Gen 1 is random, so one
   rollout of an alternative is an anecdote.
2. **The corpus needs volume.** Real branch points are dozens per run, so a
   useful dataset means many runs, not a longer one.
3. **Evaluation needs comparison.** A regression benchmark is meaningless until
   the same position can be replayed under different settings.

---

## The RNG subtlety that shapes everything

Gen 1's RNG derives from hardware and input timing. There is no seed to expose.

- Two runs with identical inputs are identical. Phase 1 verified this.
- **Any** difference in decision timing diverges the RNG completely.

Pulling opposite ways:

- Run diversity is free. Different policies produce different luck with no work.
- Fair A/B is **not** free. Running two policies gives different luck, so a
  difference in outcome is not evidence. Either run many and average luck out,
  or branch both arms from the *same save state*, which pins the RNG at that
  point.

That is the second reason 3.8b keys decision records on save states: they are
not only replayable, they are the only controlled comparison available.

---

## Concurrency is bounded by cores, not memory

A headless instance is a few MB, so memory is irrelevant. But GomeBoy runs at
~36x realtime, which means each run saturates a core. Eight to sixteen
concurrent runs on a normal machine; past that they only slow each other down.
Size the pool from `runtime.NumCPU()`, do not promise a hundred.

An emulator is **not** thread-safe. Each session owns its `*emu.Emu`
exclusively, and everything touching it happens on that session's goroutine —
the same rule that already puts frame capture and trace sampling on the
stepping goroutine.

---

## Tasks

| | Task | First edit | Depends |
|---|---|---|---|
| S4-1 | Run arguments: starter, policy, fog, record dir, stop-at milestone | `cmd/pokepilot/main.go` | — |
| S4-2 | `Watch` becomes a mountable `http.Handler`, not a server | `emu/watch.go` | trace work |
| S4-3 | Session registry: start, stop, status, one server, `/s/{id}/...` | `session/session.go` | S4-2 |
| S4-4 | Run N sessions in parallel, pool sized from NumCPU | `session/pool.go` | S4-3 |
| S4-5 | Decision records written to the record dir (3.8b schema) | `journal/journal.go` | S4-1 |
| S4-6 | Session list UI with live thumbnails and per-run trace | `session/ui.go` | S4-4 |

S4-1 through S4-5 produce data. S4-6 makes it pleasant to watch, and is
deliberately last: it is the most work and the least value.

None of this touches `skill/`, `world/` or `red/`, so it cannot collide with
slice 3.

---

## Measure in frames, never seconds

"How fast did it finish" must be **emulated frames**, not wall clock. Frames are
deterministic and machine-independent; seconds vary with load and with how many
sessions share the box. `emu.FrameCount` already tracks it.

Per-run metrics worth recording: frames to each badge, attempts per gate, party
composition at each milestone, and total planner calls.

---

## Later: perturbation runs

Once sessions are parameterised, deliberately make the run harder and see
whether the agent still finishes. This is the strongest available test of
whether it is reasoning or replaying a memorised route.

**Random starter** is the best single perturbation, because the difficulty is
wildly asymmetric and the ROM proves it:

```asm
BrockData:  db $FF, 12, GEODUDE, 14, ONIX, 0
```

Squirtle walks through Brock. Charmander cannot: Rock resists Fire, and in
**Red** there is no easy Fighting-type answer — Route 22 is Rattata, Nidoran and
Spearow (Mankey is Yellow only). The actual solution is a three-step plan:

```asm
ButterfreeEvosMoves:
	db 12, CONFUSION
```

Catch a Caterpie in Viridian Forest, evolve it to Butterfree by 12, and beat
Onix with a special move its physical stats cannot answer — or brute-force by
over-levelling.

An agent that only completes Squirtle runs has overfit. One that derives the
Caterpie plan is reasoning. That single flag is a sharper capability test than
any benchmark we could design.

Other perturbations, in rough order of cruelty: forbid catching (starter only),
cap levels, handicap the move policy, Nuzlocke rules (a fainted mon is released).

Report per perturbation: completion rate, frames to completion, and where the
failures cluster. Failures clustering at one gate is a missing objective;
failures spread evenly is a weak policy.
