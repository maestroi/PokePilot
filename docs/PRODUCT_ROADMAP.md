# PokePilot delivery roadmap: reliable runs to watchable episodes

> **Planning snapshot:** 2026-10-16. This is a prioritized *proposal*, not a
> statement that every linked issue is open or unimplemented. The linked
> GitHub issue/PR is authoritative for status. Re-check it before picking up
> a slice. The product rationale, audience and commercial boundaries are in
> [PRODUCT_STRATEGY.md](PRODUCT_STRATEGY.md).

## One delivery principle

Ship **vertical experiences with evidence**, not disconnected features.
The near-term demonstration is:

> Select a supported challenge → complete a qualified autonomous run →
> preserve its recording/event evidence → produce an edited, truthful
> watchable episode → approve its packaging → make it available to watch.

Do not hold this demonstration hostage to completing every generation,
new game, social feature or paid offering. Choose an actually qualified run
and a game/revision that the renderer can handle, and expand from there.

## Sequencing

| Horizon (indicative) | Milestone / owner | Demonstrated exit gate |
| --- | --- | --- |
| Now / 0–30 days | **M0: trustworthy runs** — gameplay, qualification, operations | A fixed supported challenge cohort has verified outcomes, failure evidence, durable replays and observability |
| Next / 30–60 days | **M1: first produced episode** — media + studio | One historical completed run generates a reviewed, genuinely edited full episode and a short candidate without editing source replay |
| Then / 60–90 days | **M2: viewer-to-episode path** — public site + admin media | A new viewer can find a challenge, watch live/replay without confusing blank/buffer states, and view its verified result |
| After M1/M2 | **M3: distribution and audience test** — studio + platforms | Small manually approved publishing cadence and explicit, safe viewer challenge participation |
| After audience validation | **M4: differentiated scale** — gameplay + platform | Model/run competitions, optimization and a rights-cleared creator/developer pilot with measurable economics |

These horizons are planning aids, **not release promises**. Advance on
evidence at the gate, not the calendar.

## M0: stable autonomous play and honest qualification

**Why:** an editor, spectator and paid service cannot compensate for runs
that hang, claim false progression or lose their artifacts.

Use existing work rather than creating a second reliability stack:

- [#114](https://github.com/maestroi/PokePilot/issues/114):
  ROM-backed milestone/full-run qualification; keep ROM-free CI distinct.
- [#2331](https://github.com/maestroi/PokePilot/issues/2331):
  Prometheus/Loki/Tempo/Grafana outcomes, failures, latency and alerts.
- [#968](https://github.com/maestroi/PokePilot/issues/968):
  Gold/Silver adapter and campaign progression.
- [#2706](https://github.com/maestroi/PokePilot/issues/2706) and
  [#2707](https://github.com/maestroi/PokePilot/issues/2707):
  actually earn Johto badge 7 and badge 8; do **not** unlock launcher
  targets based only on fact definitions or dropdown choices.
- [#1045](https://github.com/maestroi/PokePilot/issues/1045):
  Yellow full-game qualification/regression where still outstanding.
- [#1543](https://github.com/maestroi/PokePilot/issues/1543):
  compatibility cleanup where it removes risk for supported adapter paths.

**Minimum gate evidence:**

- A support matrix names the exact game/revision, challenge goal, milestone
  frontier, build/model/policy and qualification evidence; only qualified
  targets are selectable.
- Fixed, repeatable seed/checkpoint cohorts show successes **and failures**,
  completion rate, elapsed frames/wall time, model calls and typical stops.
  Choose the baseline before setting numeric success targets.
- Every failed/stalled attempt has a structured outcome, triage evidence and
  replayable checkpoint when supported. Repeated defects yield a smallest
  practical deterministic regression.
- A representative successful run has a durable canonical `run.gbrun`,
  semantic milestone/event timeline and reproducibly generated media.
- Grafana can answer: did this release change completion, stuck time,
  recovery, cost and replay/render failures? Logs/traces drill down to runs
  without high-cardinality metric labels.

**Execution hygiene:** check currently open/draft PRs and merge or close
validated work before starting an overlapping implementation. For example,
review the PRs associated with [#2658](https://github.com/maestroi/PokePilot/issues/2658),
[#2662](https://github.com/maestroi/PokePilot/issues/2662), and Gen 2
badge-7 routing in [#2706](https://github.com/maestroi/PokePilot/issues/2706)
rather than forking the same functionality.

## M1: first edited episode from a real run

This is the **first major new product vertical**. The transport/composition
foundation is in [#172](https://github.com/maestroi/PokePilot/issues/172),
[MEDIA_ENGINE.md](MEDIA_ENGINE.md) and existing event/highlight work.
Do not build a parallel recorder or uploader.

### Dependency chain and suggested implementation PRs

| Slice | Existing owner | Acceptance / artifact |
| --- | --- | --- |
| **M1.1** Evidence-backed producer | [#797](https://github.com/maestroi/PokePilot/issues/797) | Versioned story + event-referenced beats + scores + episode/short/archive/skip decision |
| **M1.2** Declarative episode editor | [#799](https://github.com/maestroi/PokePilot/issues/799) | Validated edit plan with segment timestamps, trim/speed, captions/overlays and narration cues compiled to deterministic media pipeline |
| **M1.3** Packaging | [#801](https://github.com/maestroi/PokePilot/issues/801), [#802](https://github.com/maestroi/PokePilot/issues/802) | Verified event-aware titles, chapters, descriptions and versioned thumbnail variants |
| **M3.1** Publication transport | [#178](https://github.com/maestroi/PokePilot/issues/178) | Approved MP4 upload with provider metadata; not needed for a locally reviewable first episode |
| **M3.2** Quality and review queue | [#803](https://github.com/maestroi/PokePilot/issues/803) | Depends on #178 and packaging; review/approve/reject/skip candidate, idempotent status, no duplicate publication |
| Later: live commentary | [#800](https://github.com/maestroi/PokePilot/issues/800) | Sparse context-driven narration, isolated from gameplay; only if it improves watchability |

**Scope the first pilot tightly:** pick one real finished run that already
has verified source events and replay, one long-form profile and one short
profile. Do not depend on novel gameplay mechanics, multi-game rendering or
automatic uploading for initial success.

**M1 gate:** the operator can select that historical run; get a truthful
producer assessment with evidence refs; preview an episode with repetitive
sections cut/speed-adjusted and milestones retained; preview a short; inspect candidate packaging and manually accept or reject
that preview without requiring a publishing queue. Both outputs retain source run,
timeline, prompt/policy, edit-plan and renderer versions. A rejected or failed
render does not mutate the run or its canonical artifacts. A boring run can
legitimately yield `do-not-publish`.

**Test contract:** validate missing/contradictory event refs, out-of-range
and overlapping timestamp constraints, unsupported operations, retries,
stable artifact identity, redaction, and replay/renderer failures. Include
a deterministic fixture-based test and a private real-run preview check.

## M2: make RomPilot enjoyable to use

- [#2670](https://github.com/maestroi/PokePilot/issues/2670)
  and [#2676](https://github.com/maestroi/PokePilot/issues/2676):
  keep Live/Runs/Failures/Media coherent; connect a source run to its replay,
  highlight, episode, render job and broadcast controls. Do not display
  nonfunctional Studio or publish buttons as though a backend exists.
- Public spectator: center gameplay and the current goal, make
  thinking/recovering/stalled states explicit, distinguish loading from
  nothing-to-watch, provide useful mobile layouts, and expose links to
  replay and verified outcome. Respect [SPECTATOR.md](SPECTATOR.md).

**M2 gate:** on desktop and narrow mobile viewport, a visitor can find
a live run or completed episode, tell what is happening, recover gracefully
from missing/slow media, and understand the result. Operator actions
remain private, public payloads sanitized, and accessibility checks cover
keyboard, contrast and reduced motion. Measure start success and latency
before and after the changes.

## M3: small content/audience experiment

- [#803](https://github.com/maestroi/PokePilot/issues/803) and
  [#178](https://github.com/maestroi/PokePilot/issues/178):
  launch an **approved-only** publishing cadence for a small, diverse set of
  genuinely edited episodes/Shorts. Track every candidate including skips.
- [#2194](https://github.com/maestroi/PokePilot/issues/2194) and
  [#2195](https://github.com/maestroi/PokePilot/issues/2195):
  connect chat votes to the existing validated challenge catalog. Restrict
  votes to published choices, never accept arbitrary gameplay commands.
- Run a 4–6 week learning experiment across a few formats, varying
  hooks, difficulty, model comparisons and video length. Record audience
  retention, returning viewers, click-through, episode cost, and *which
  stories* perform; do not optimize upload count alone.

**M3 gate:** real audiences repeatedly choose to watch some content,
publishing failures cannot corrupt runs or double-post videos, and there is
enough signal to prioritize the next concepts. No paid tier is needed here.

## M4: learning agent, more games, commercial validation

- [#2136](https://github.com/maestroi/PokePilot/issues/2136):
  route-based speedrun mode; evaluate against identical qualified cohorts
  and show verified records.
- [#968](https://github.com/maestroi/PokePilot/issues/968):
  extend Gold/Silver into the League, Kanto and Red **after** each frontier
  is independently qualified. Add other games only when the adapter and
  repeated-play economics justify them.
- Explore creator/developer pilots on games with explicit commercial rights:
  autonomous regression, benchmarks, reusable media pipelines and hosted
  orchestration. Validate price, workload, support burden, IP and data/privacy
  obligations before implementing billing.

## Product scorecard and release checklist

Keep metrics for both *reliability* and *entertainment*, with historical
comparisons grounded in known source data:

- **Agent:** fixed-cohort completion rate, recovery success, p50/p90
  completion frames and wall time, cost per successful run, top failure
  families, stalled time.
- **Media:** render/job success and latency, editor/approval time, qualified
  episode yield, first-frame playback success, cost per candidate.
- **Audience:** click-through, watch retention, returning viewers, voted
  challenge participation. Only collect analytics under applicable
  privacy/consent constraints.
- **Business (later):** cost per published episode, rights-clearance status,
  creator pilot usage and willingness to pay.

For **every milestone PR**: link its owning issue; document the exact source
run/fixture and intended positive postcondition; preserve game adapter and
public/private boundaries; add lowest-cost deterministic tests; report
ROM-free verification plus any private ROM-backed checks separately; and
include a screenshot/video or reproducible artifact for UX/media behavior.

Do not mark a milestone complete from a merged PR alone: attach gate evidence.

## Immediate pickup order

1. **Stabilize the substrate:** close the pending PR/rebase/qualification
   loop; choose one already-supported, reproducible game/goal as the media
   pilot and capture baseline completion + replay evidence.
2. **Implement [#797](https://github.com/maestroi/PokePilot/issues/797):**
   the smallest producer output against a known real replay timeline, including
   `archive/do-not-publish` and event-ref validation. No direct rendering.
3. **Implement [#799](https://github.com/maestroi/PokePilot/issues/799):**
   turn that output into a restricted, deterministic edit plan + first
   long-form preview; add short format once the shared compiler is stable.
4. **Then [#801](https://github.com/maestroi/PokePilot/issues/801) and
   [#802](https://github.com/maestroi/PokePilot/issues/802):**
   package candidate edits, reusing existing media components.
5. **For publishing, implement [#178](https://github.com/maestroi/PokePilot/issues/178),
   then [#803](https://github.com/maestroi/PokePilot/issues/803):**
   wire approved uploads and the durable review queue in dependency order.
6. **Finally:** wire public navigation and measure
   the first audience experiment before selecting a monetization path.

GitHub issues own task implementation and changing status. This page owns
**cross-epic priority, dependencies and release gates**; update it when those
decisions change, ideally in the PR that changes the plan.
