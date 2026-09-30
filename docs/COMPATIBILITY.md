# Compatibility baseline

Last reviewed: 2026-09-30 for issue #1543.

PokePilot keeps compatibility only where a persisted artifact or an external
wire contract still depends on it. Every compatibility path below has a concrete
reason and removal gate. Rolling-deployment fallbacks are not an indefinite
support tier.

## Minimum supported baselines

### Knowledge checkpoints

The supported knowledge checkpoint window is **v4 through v6**.

- v6 stores semantic `LocationID` geography and `ObjectiveKey` strategic-plan
  steps.
- v5 native geography is migrated through the active game adapter and its keyed
  strategic plan remains resumable.
- v4 native geography, objective history, requirements, failures, intent, and
  intent age remain migratable. A v4 sentence-only strategic plan is **not**
  executable state: it is discarded and the next round replans from migrated
  knowledge.
- A malformed or obsolete plan does not invalidate otherwise readable
  knowledge; checkpoint loading discards that plan and retains the checkpoint.

Dropping v4 or v5 requires an explicit support-window decision.

### Farm run specs

`farm.Spec` has no schema-version field. The supported persisted/queued
baseline is JSON using the current field names, with newer optional fields
allowed to be absent. `llm_profile` is the one documented transitional
selection field still accepted alongside `llm_deployment`; omitted `game`
continues to mean no cartridge preference rather than a migration alias.

There is no promise to preserve renamed private fields that are not listed in
this document.

### Deployment

Production `pokewall` persistence uses PostgreSQL through `-database` /
`POKEPILOT_DATABASE_URL`. `-state` and `-catalog` are intentional
local/test modes and may not be combined with `-database`.

The operator browser surface is the built Vue application shipped with the same
`pokeui` release. There is no vanilla-console fallback and no supported
mixed-version deployment that depends on old static operator assets.

Static Gen-I world construction starts from the selected profile's typed
`MapProvider`; portable run topology comes from `game.MapTopologyProvider` so
Gen II can keep its native wide map IDs. Generic world code does not detect
games from raw ROM bytes.

## Retained compatibility paths

| Path | Why it is retained | Removal gate |
| --- | --- | --- |
| `Knowledge.NativeLocations`, native observation translation, `SawMap` / `TalkedTo` native forms | v4/v5 checkpoint geography and some transient emulator samples still use native map ids. Durable knowledge itself is semantic. | The oldest supported checkpoint is v6 **and** every supported observation producer supplies semantic location identity. |
| Checkpoint files without persisted game/revision identity | Older supported checkpoint versions predate profile identity metadata. Files that do carry identity are already rejected on mismatch. | The checkpoint support window excludes every version that predates game/revision identity. |
| `farm.Spec.LLMProfile` / `llm_profile` | Persisted queued runs and older runners can still carry the pre-deployment selector; current enqueue code can translate it to deployment identity. | Every supported queued spec contains `llm_deployment` (or inference identity) **and** the minimum supported runner no longer reads `llm_profile`. |
| Exact-occurrence `issueLinks` lookup in `cmd/pokewall/objective_failures.go` | Persisted wall state written before family fingerprints keyed issue links by occurrence fingerprint. The alias prevents a duplicate issue when such a record recurs. | Supported/migrated wall state contains only family-keyed issue links. |
| Normalized-prose fallback in `cmd/pokewall/failureIdentity` | Historical finish dumps can predate the canonical `failure-id` marker and still need deterministic triage identity. | The supported archive/repro window contains no finish dump without a canonical failure marker. |

## Compatibility removed by #1543

The following are deliberately **not** part of the baseline anymore:

- `agent.NewKnowledge(topology any)` and byte-map topology coercion. Knowledge
  construction uses `*KnowledgeTopology`.
- Sentence-only strategic plans as executable checkpoint state. v4 knowledge is
  retained, but an unkeyed plan is discarded and replanned.
- `world.BuildGraph([]byte)`, `ROMProviderFactory`,
  `RegisterROMProviderFactory`, and `ProviderForROM`. Callers select a profile
  and pass its `MapHeaderProvider`.
- Mixed-deployment `pokewall` outcome/catalog wrappers. Catalog issue overlay
  and no-catalog RAM outcomes are named for their current responsibilities,
  while production history comes from PostgreSQL.
- The vanilla operator HTML/CSS/JS bundle and its Go injection helpers. Vue owns
  operator root routing; missing Vue build assets fail visibly instead of
  falling back to stale browser code.

Future compatibility additions should update this file in the same change that
introduces them, including their removal condition.
