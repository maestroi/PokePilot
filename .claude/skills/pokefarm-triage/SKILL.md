---
name: pokefarm-triage
description: Use when investigating PokeFarm failures or fixing runs — consume one selected failure, reproduce its exact failed .state locally before changing code, fix the shared invariant, verify the same replay plus short tests, and ship a triage-keyed PR. Interactive use may inspect the actionable queue; unattended qwagent must use the packet already selected by the shell.
---

# PokeFarm run triage

The farm runs a committed build. A failure it reports is locally replayable:
every round persists a `.state` artifact, and replaying that state through the
same skill call should fail the same way. Never fix from the error string alone
— reproduce first, then fix, then re-run the same repro.

## 1. What is failing

There are two entry modes.

### Interactive triage

Use:

```
pokepilot_get_triage()                         # current actionable view
pokepilot_get_triage(include_resolved=true)    # history/audit only
pokepilot_list_runs(limit=20)                  # raw run evidence/context
```

PokePilot groups failures by stable fingerprint. Linked Agent Orchestrator
metadata may include `status`, `resolution`, `occurrence_count`, and
`fixed_revision`, but Orchestrator is not required for the local qwagent repair
loop. Do not recreate work by grouping old `pokepilot_list_runs` detail strings:
those rows are evidence and can describe bugs fixed by later revisions.

### Unattended qwagent triage

The shell has already selected exactly one failure and attached a packet with
`key`, `run_id`, `example`, `count`, and `fingerprint`. **Do not call
`pokepilot_get_triage` to choose another item.** The packet is the work item.

The selector uses a deterministic local lifecycle so it can keep working while
Agent Orchestrator is missing or stale:

- no triage PR for a key -> actionable;
- open PR containing `[triage:<key>]` -> claimed, skip;
- merged PR containing `[triage:<key>]`, while the representative failure came
  from a build before that merged repair -> repaired / awaiting post-fix
  evidence, skip;
- the same key reproduced by a `runner_version` whose Git history contains the
  merged repair -> regression, actionable again.

A proven post-fix regression is stronger evidence than stale remote
`resolved/fixed` metadata. Conversely, missing run-version or Git ancestry
proof must fail closed: do not create a duplicate repair merely because remote
issue state is unavailable.

`reason` values commonly include `failed`/`error` for an objective failure and
`budget` for a planner loop. A live run with no terminal reason is not a repair
item yet.

## 2. Evidence for one run

```
pokepilot_get_run_debug(run_id)
```

Use `finish.detail`, `finish.runner_version`, `trace_tail`, progress, artifacts,
and the last model exchange to understand what actually happened. The top-level
map can be stale; prefer final progress/debug evidence when they disagree.

For unattended work, do not use the debug payload to reconsider whether the
packet should have been selected. Use it to reproduce and diagnose that packet.

## 3. Pull the failing round's state

Artifacts are per round: `round-<N>-frame-<F>-<objective>.state`. Take the one
whose objective matches `finish.detail`.

`admin.rompilot.app`'s plain `GET /v1/runs/{id}/artifacts/{name}/content` route
sits behind Cloudflare Access for browser sessions — a bare
`Authorization: Bearer $POKEPILOT_MCP_TOKEN` curl to it 302s to the Access
login page, it does not download the artifact. Use the MCP tool instead, which
reaches `pokewall` server-to-server and never crosses that edge:

```
pokepilot_get_run_artifact_content(run_id=<run-id>, name=<artifact-name>)
```

It returns `content_base64` for small **inline** artifacts (`.state`, `.ram`,
knowledge/failure JSON), bounded by the MCP response cap; decode it to a local
file, e.g.:

```bash
python3 -c "import base64,sys; open('/tmp/r46.state','wb').write(base64.b64decode(sys.argv[1]))" "$CONTENT_BASE64"
```

It refuses artifacts pokewall marks as remotely stored (`run.gbrun` stays
S3-only; see `docs/RUN_INSPECTOR.md`'s `.gbrun` replay player section — it is
not part of the state repro this skill needs).

`.state` files load with `emu.LoadState`; the ROM is normally
`roms/pokemon_red.gb` or the checkout-independent configured ROM path.
Never commit ROMs, saves, or replay states.

## 4. Reproduce it locally

Write a throwaway `skill/zz_repro_scratch_test.go` (delete it when done) that
loads the state and calls the same skill the objective calls. See
`agent/objective.go` for the mapping.

Example shape:

```go
m, _ := emu.Open(os.Getenv("POKEMON_RED_ROM"))
m.LoadState(stateBytes)
dest, _ := skill.Place("cerulean city")
res, err := skill.TravelFlee(m, rom, dest, skill.StatAwareMove(rom), 20)
```

Then run only the exact repro:

```bash
POKEMON_RED_ROM=$PWD/roms/pokemon_red.gb REPRO_STATE=/tmp/r46.state \
  go test ./skill -run TestScratchRepro -v
```

The same replay is the proof of the fix: it must fail before the patch and pass
after it.

When the reason is not obvious, instrument only the relevant return path, replay
again, then remove the instrumentation. Do not reason from a collision grid by
hand; use `skill/probe_test.go` as required by `AGENTS.md`.

### Do not reproduce a failure through the planner

The `.state` replay pins the objective that failed and calls its skill directly.
A live planner can choose something else, which turns the reproduction into a
different experiment.

Use `.state` + a focused scratch test to prove the defect and fix. Use
`pokerepro -play` only when you specifically need to ask whether a run gets past
an earlier boundary after the change.

### When the failing round does not contain the bug

If the failing round arrives already poisoned — for example by an earlier party
wipe, bad knowledge, or planner decision — replay from an earlier objective
checkpoint with its paired knowledge instead of guessing how the state formed.

```bash
W=https://admin.rompilot.app
curl -sS "$W/v1/runs/<run-id>/checkpoints"
go run ./cmd/pokerepro -wall $W -run <run-id> -checkpoint latest
```

This is slower and may involve a live LLM, so use it only when the exact failing
round cannot reproduce the causal defect.

## 5. Fix at the shared point

Follow `docs/ARCHITECTURE.md` and `AGENTS.md`. Fix the invariant at its owning
layer rather than adding a named-map/story exception. Grep callers before
editing: one correct guard in a shared skill is better than compensating patches
in every caller.

Then:

1. re-run the exact failed-state repro;
2. delete `skill/zz_repro_scratch_test.go`;
3. run `make test-short`.

Do not use bare `go test ./skill/...` as the regression gate; that boots the ROM
for long journey tests. The exact state replay supplies the ROM-backed evidence,
and `make test-short` supplies the broad deterministic gate.

If either gate is red, stop. A failing gate is not a shippable repair.

## 6. Ship it

Only once both gates are green:

```bash
git switch -c fix/<short-slug>
git add <only the files changed for the repair>
git commit
git push -u origin HEAD
```

Never commit or push `main`, never `git add -A` over unrelated work, and never
commit `.gb`, `.sav`, `.state`, or `skill/zz_*_test.go`.

For an unattended qwagent packet, always open a PR whose title includes the
stable claim marker:

```
fix(farm): <short symptom> [triage:<key>]
```

The body must name the run id and fingerprint and summarize the reproduction,
root cause, fix, and verification. Do not merge the PR; the repository's normal
merge/deploy machinery owns that step.

For an interactive human-requested investigation, follow the user's requested
shipping boundary; if they asked only for diagnosis, do not push merely because
this skill can.

## Known map

- `docs/RUN_INSPECTOR.md` — artifact/replay endpoints and run inspection.
- `docs/RAM_FORENSICS.md` + `gomeboy-forensics` — instruction-level probes.
- `skill/probe_test.go` — measured walkability/route/state questions.
- `skill/goto.go` / place facts — measured destinations; do not invent literals.
