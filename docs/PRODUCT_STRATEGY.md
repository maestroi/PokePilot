# PokePilot and RomPilot product strategy

> **Status:** proposed direction, 2026-10-16. This is a product planning
> document, not an implementation-status ledger, legal clearance, or a change
> to the binding engineering rules in [ARCHITECTURE.md](ARCHITECTURE.md).
> Delivery details and current priorities live in
> [PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md).

## North star

Build an autonomous game-playing platform that can **complete verifiable
challenges, explain what happened, and transform interesting runs into
entertainment people choose to watch**.

There are two related, but distinct, products:

- **PokePilot (engine/operator platform):** semantic game agents, reproducible
  execution, recovery, qualification, experimentation, farm orchestration,
  telemetry, replay, and eventually an authorized creator/developer offering.
- **RomPilot (public experience):** live viewing, challenge programming,
  leaderboards, replay episodes/Shorts, and limited audience participation.

The engine remains useful even if an entertainment channel does not take off.
The entertainment experience should make sense to people who have never heard
of ROM state, agent objectives, or emulator checkpoints.

## Who it serves

| Audience | Job to be done | First value |
| --- | --- | --- |
| Operator/developer | Run, debug, compare and improve unattended agents | Reproducible runs, qualification, failures and cost |
| Viewer | Understand the challenge and enjoy its outcome | Clear live status, story-driven replay, meaningful highlights |
| Community member | Influence *which safe challenge* happens next | Validated challenge catalog, scheduled votes, results |
| Creator or developer (future) | Operate authorized agents or produce game content | Reusable orchestration, evidence, edited media and APIs |

The existing root [PRODUCT.md](../PRODUCT.md) remains the UI/product contract
for the **private operator console**. This strategy does not replace it.
See [SPECTATOR.md](SPECTATOR.md) for the public/private security boundary.

## Differentiation

The advantage is not simply `LLM + game video`. It is the combination of:

1. **Semantic gameplay truth:** objectives have typed results and positive
   postconditions; claims are checked against actual game state.
2. **Deterministic provenance:** runs, checkpoints, event timelines and derived
   media have durable identities and can be inspected or regenerated.
3. **An improvement loop:** farm failures become regressions and comparative
   qualification, rather than endless manual rescues.
4. **An editorial loop:** media uses real run events to identify stories, not
   synthetic claims or a model guessing from raw pixels.
5. **Audience programming:** viewers choose among validated challenges; public
   inputs never become unvalidated gameplay instructions.

Preserve the generic semantic core and game-specific adapter boundaries in
[ARCHITECTURE.md](ARCHITECTURE.md). Do not trade these away for a demo.

## Product loops

### Engineering loop (must work first)

```text
define supported challenge
  -> launch verified game + agent
  -> observe structured outcomes
  -> recover or classify failure
  -> replay / create smallest regression
  -> compare fixed benchmark cohorts
  -> expand qualified capabilities
```

A successful code build is not evidence that a game can finish. Every
supported game/goal/revision combination should have a qualification claim
and reproducible evidence. Unsupported goals must not be selectable merely
because a UI label or game flag exists.

### Content loop (next major vertical slice)

```text
qualified run + canonical recording + semantic event timeline
  -> producer: story, evidence-backed beats, novelty/quality score
  -> quality gate: episode | short | archive | do-not-publish
  -> versioned, validated edit plan
  -> deterministic rendered media
  -> title/description/chapters/thumbnail variants
  -> operator review, then idempotent publication
  -> watch/engagement feedback for future programming
```

A run may legitimately produce **no public content**. Producer/editor models
choose *what* to show; trusted deterministic code renders it. Never hand a
model arbitrary shell/FFmpeg execution or alter source gameplay/replay
artifacts during media production.

### Audience loop (only after content quality)

Present live gameplay with the current challenge, objective, progress,
recovering/thinking/stalled state, and recent major events. Link source runs
to replay episodes and verified results. Let viewers vote only on an
allowlisted, capability-validated challenge catalog. Keep public surfaces
read-only with respect to run internals and private operator APIs.

## What to prioritize — and what not to

**Now:** reliable unattended completion for *supported* challenges, bounded
recovery, realistic ROM-backed qualification, replay availability, a polished
public viewing path, and one high-quality end-to-end episode.

**Next:** repeatable content production and human-review publishing, then
small audience experiments, challenge voting and model/run comparisons.

**Later:** more games and modes, automatic publishing (only after quality
gates prove reliable), and a general-purpose commercial offering.

**Not current goals:** charging users to run copyrighted Pokémon ROMs, adding
every possible game before releasing anything, equating livestream uptime
with audience value, mass-publishing unedited runs, or building a billing
system before validating demand.

## Commercial hypothesis and boundaries

Commercialization is a **hypothesis to test**, not a promised revenue stream.

1. **Audience test:** editorially distinct, human-reviewed videos/Shorts and
   useful livestreams. Validate retention and repeat viewing before spending
   heavily on volume.
2. **Community test:** optional fan support and interactive programming,
   subject to platform/rights requirements and demonstrable demand.
3. **Creator/developer test:** interview prospective users and pilot the
   game-independent agent/benchmark/media infrastructure with content and
   games for which commercial use is explicitly authorized.
4. **Only then:** consider hosted plans, paid APIs, production tooling,
   sponsorships or usage billing, chosen based on measured unit economics.

Before any commercial launch, confirm the applicable game/IP rights, ROM and
asset provenance, publisher terms, platform rules, music/TTS/image licenses,
privacy obligations and any required professional legal review. The project
must not redistribute copyrighted ROMs/save states or assume that streaming
permission automatically grants permission for a paid hosted game service.
The architecture should accommodate original, open-licensed and otherwise
commercially authorized games rather than depend on Pokémon licensing.

## Measurement: define baselines before targets

| Metric | Definition / evidence |
| --- | --- |
| Qualified completion rate | Verified successful finishes / eligible attempts in a fixed game/goal/seed/model/build cohort |
| Recovery success | Recoveries meeting a positive game-state postcondition / qualified recoverable incidents |
| Run efficiency | Emulated frames, wall time and model/resource cost per verified completion; failures reported separately |
| Media yield and reliability | Publishable candidates per completed run; successful renders / accepted jobs, with latency and retry counts |
| Public experience | Playback/start success, initial load latency, mobile errors, replay completion and repeat visits |
| Editorial performance | Video click-through and retention, Shorts completion, returning viewers; measured per concept/format |
| Business viability | Cost per approved episode, human review time, production cost vs. revenue and/or pilot customer willingness to pay |

Prefer Grafana for operator/reliability signals and privacy-respecting,
appropriately consented distribution analytics for audience signals. Avoid
using `run_id` or trace identifiers as Prometheus labels; link into logs and
traces instead. Proposed reliability targets should be chosen **after**
baseline cohorts exist.

## Decision and release rules

- **Capability gate:** a goal is advertised as supported only after
  game-state verification and real-ROM qualification for its specified profile.
- **Source-of-truth gate:** canonical recording, semantic timeline and run
  evidence survive any media failure or re-edit.
- **Editorial gate:** no invented battles, records or quotes. Every claimed
  beat is traceable to a run event; low-quality results may be archive-only.
- **Publication gate:** manual approval first; retries are idempotent, with
  explicit ownership, rate limiting and ability to stop distribution.
- **Security gate:** no raw model payloads, ROMs, credentials, checkpoint
  contents or private debug data on public surfaces.
- **Business gate:** no paid game-specific service without rights review and
  validated demand/cost assumptions.

Review priorities at the end of each vertical slice. Prefer closing one
cross-stack experience over starting several unconnected features.

## Existing work this strategy builds upon

- [#968](https://github.com/maestroi/PokePilot/issues/968): Gold/Silver support;
  [#114](https://github.com/maestroi/PokePilot/issues/114): ROM-backed qualification.
- [#2331](https://github.com/maestroi/PokePilot/issues/2331): observability
  and outcome metrics.
- [#172](https://github.com/maestroi/PokePilot/issues/172): replay,
  broadcasting and publication transport.
- [#796](https://github.com/maestroi/PokePilot/issues/796): content studio
  producer, editor, packaging and quality gates.
- [#2670](https://github.com/maestroi/PokePilot/issues/2670): coherent
  operator UI and connected media workspace.
- [#2194](https://github.com/maestroi/PokePilot/issues/2194): Twitch/YouTube
  audience integrations.
- [#2136](https://github.com/maestroi/PokePilot/issues/2136): speedrun
  optimization and route comparisons.

Track actual implementation state in the linked GitHub issues and PRs, not
in prose assertions here. See [PRODUCT_ROADMAP.md](PRODUCT_ROADMAP.md) for
prioritized delivery slices.
