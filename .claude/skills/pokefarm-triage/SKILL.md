---
name: pokefarm-triage
description: Use when asked to investigate PokeFarm runs, check the latest farm failures, or fix what is making runs fail — pulls the recent runs over the pokepilot MCP, groups them by failure, downloads the failing round's .state, reproduces the failure locally as a Go test before changing anything, then commits and pushes the fix once the tests pass.
---

# PokeFarm run triage

The farm runs the committed HEAD build (`workers[].version` in
`pokepilot_list_runs` is the commit). A failure it reports is reproducible
locally: every round persists a `.state` artifact, and replaying that state
through the same skill call fails the same way. Never fix from the error
string alone — reproduce first, then fix, then re-run the same repro.

## 1. What is failing

```
pokepilot_get_triage()                 # grouped failures; often empty
pokepilot_list_runs(limit=20)          # the real source: read .reason/.detail
```

`get_triage` returning `{"groups":[]}` means nothing was grouped, not that
nothing is failing. Group `list_runs` yourself by the `detail` string — the
same seed is re-queued after a failure, so one bug shows up as N identical
`detail` lines in a row. Fix the class with the most runs.

`reason` values: `failed` (an objective errored), `budget` (rounds/frames ran
out — usually a planner loop, look at `stats.repeats` and the `choices`
histogram), no reason + `status: running` (still going).

## 2. Evidence for one run

```
pokepilot_get_run_debug(run_id)
```

Gives `finish.detail`, `trace_tail` (the last frames before the stop — the
dialogue text is verbatim here), `progress_final.map_name` (where it actually
died; the run's top-level `map` can be stale), the artifact list, and `run.raw`
— the full LLM prompt/reply of the last decision, including the observation's
`History` and `Failures`, which is where the *other* failures of that run are
listed.

## 3. Pull the failing round's state

Artifacts are per round: `round-<N>-frame-<F>-<objective>.state`. Take the one
whose objective matches `finish.detail`.

```bash
curl -sS -o /tmp/r46.state \
  -H "Authorization: Bearer $POKEPILOT_MCP_TOKEN" \
  "https://pokemon.labstack.cc/v1/runs/<run-id>/artifacts/<artifact-name>/content"
```

The token is the `pokepilot` MCP server's bearer in `~/.claude.json`
(`mcpServers.pokepilot.headers.Authorization`). `.state` files load with
`emu.LoadState`; the ROM is `roms/pokemon_red.gb`.

## 4. Reproduce it locally

Write a throwaway `skill/zz_repro_scratch_test.go` (delete it when done) that
loads the state and calls the same skill the objective calls — see
`agent/objective.go` for the mapping (`KindGoTo` → `skill.Travel` /
`skill.TravelFlee` with `skill.StatAwareMove(rom)`, `maxBattles` 20).

```go
m, _ := emu.Open(os.Getenv("POKEMON_RED_ROM"))
m.LoadState(stateBytes)
dest, _ := skill.Place("cerulean city")
res, err := skill.TravelFlee(m, rom, dest, skill.StatAwareMove(rom), 20)
```

```bash
POKEMON_RED_ROM=$PWD/roms/pokemon_red.gb REPRO_STATE=/tmp/r46.state \
  go test ./skill -run TestScratchRepro -v
```

The whole journey replays in seconds and is deterministic, so it is also the
proof the fix works: same command, different outcome.

When the repro reproduces but the *reason* is not obvious, add temporary
`fmt.Printf` lines inside the branch that returns the error, run again, and
`git checkout` the file afterwards. That is what separates "the guard fired"
from "the guard never ran" — and those want opposite fixes. For an
instruction-level answer (which CPU step changed a byte), use the
`gomeboy-forensics` skill instead.

## 5. Fix at the shared point

Farm failures are almost always one skill misbehaving for every caller. Grep
the callers of the function before editing: a guard in `skill/menu.go` beats
the same guard in `travel.go` and `interact.go`. Then re-run the repro and
delete the scratch test.

The regression gate is `make test-short` — ~6 seconds, because it unsets
`POKEMON_RED_ROM` and skips every test that boots the emulator. Do NOT run the
bare `go test ./skill/...`: those tests play the game frame by frame and take
tens of minutes, which is not a gate, it is a hostage situation. The ROM-level
evidence you need is the one state replay from step 4, which runs in seconds
and exercises the exact path the farm failed on.

## 6. Ship it

Only once BOTH are green: the step-4 repro that used to fail now passes, and
`make test-short` is clean. A red test run is not a fix — say so and stop
instead of committing.

```bash
git switch -c fix/<short-slug>          # never commit straight to main
git add <the files you changed>         # not the scratch test, not /tmp state files
git commit                              # message: symptom, root cause, evidence
git push -u origin HEAD
```

Commit the fix only. Unrelated dirty files in the tree (someone's in-flight
work) stay unstaged — `git status` before `git add`, and name any file you
leave behind in your report.

The message says what the farm saw, where the root cause actually was, and
which run proved it, e.g.:

```
skill: wait for an answered YES/NO prompt to close

Six farm runs died on "text box is a choice and is unanswered ... Would
you like to come in?" — Travel's Museum gate handler answered the prompt,
but selectTwoOption returned before the game tore the menu down, so the
next RecoverDialogue read the stale menu and called the answer unanswered.

Reproduced from run-3uhjsoyo0gx3i12rdm7cjax5pl round 46.
```

End it with the attribution trailers this repo uses (`Co-Authored-By:` and
`Claude-Session:`). Open a PR when the user asks for one; the push is enough
otherwise. Report the branch name and the commit back to the user.

## Known map

- `docs/RUN_INSPECTOR.md` — artifact/replay endpoints, `run.gbrun`.
- `docs/RAM_FORENSICS.md` + `gomeboy-forensics` skill — instruction-level probe.
- `skill/goto.go`'s `Place` table — every measured destination tile, with the
  notes explaining why that tile and not another.
