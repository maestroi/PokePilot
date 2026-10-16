# Compatibility baseline

Last reviewed: 2026-10-17 for issue #1543.

PokePilot keeps compatibility only where a persisted artifact, external contract, or
rolling deployment still depends on it. A compatibility path needs both a reason
to exist and a concrete removal gate; otherwise it is cleanup debt.

## Checkpoint baseline

The supported knowledge checkpoint window is **v4 through v6**.

- v6 stores semantic `LocationID` geography and `ObjectiveKey` strategic plan
  steps.
- v5 native geography is migrated through the active game adapter and its keyed
  strategic plan remains resumable.
- v4 native geography, objective history, requirements, failures, intent, and
  intent age remain migratable. A v4 sentence-only strategic plan is **not**
  executable state anymore: it is discarded and the next round replans from the
  migrated knowledge.
- A malformed or obsolete plan never invalidates otherwise readable knowledge;
  checkpoint loading discards that plan and retains the rest of the checkpoint.

This is the minimum persistence baseline for cleanup work. Dropping v4 or v5
requires an explicit support-window decision rather than opportunistic deletion.

## Retained compatibility paths

| Path | Why it is retained | Removal gate |
| --- | --- | --- |
| `Knowledge.NativeLocations`, native observation translation, `SawMap` / `TalkedTo` native forms | v4/v5 checkpoint geography and some transient emulator samples still use native map ids. Durable knowledge itself is semantic. | Oldest supported checkpoint is v6 **and** every supported observation producer supplies semantic location identity. |
| `POKEPILOT_SPECTATOR_URL` / `POKEPILOT_RUN_BASE_URL` (`sites.EnvSpectatorURL`, `sites.EnvRunBaseURL`) | Older running Swarm tasks may still carry these environment variables. `sites.FromEnv` reads them only when the canonical value is unset. Current stack definitions export `POKEPILOT_PUBLIC_BASE_URL` / `POKEPILOT_ADMIN_BASE_URL`; `farm.yml` also accepts the old `POKEPILOT_RUN_BASE_URL` from local `.env` as a compose-time default. | Confirm every deployed task is updated to the new stack, update any operator `.env` files, and confirm no externally managed environment relies on the aliases. Then delete the aliases and compose-time default. |
| Checkpoint files without persisted game/revision identity | Older supported checkpoint versions predate profile identity metadata. They are still read, while files that do carry identity are rejected on mismatch. | The support window excludes every checkpoint version that predates game/revision identity. |

## Wall storage and HTTP baseline

- PostgreSQL is the **only production** structured-state authority. Its
  control-plane handlers own `/v1/outcomes`; neither SQLite nor RAM is an
  interchangeable persistence fallback.
- Local/dev walls may opt into SQLite with `-catalog`; the catalog handler
  owns historical outcomes, run reads and triage, while live issue links are
  overlaid from local wall state.
- Without `-catalog`, local/dev walls intentionally retain finished tiles and
  serve `GET /v1/outcomes` from RAM. This is a **supported local mode**, not an
  image-rollout fallback. Other paths and methods pass to the normal handler.

## Retired

- The vanilla operator console (`cmd/pokeui/ui/index.html`, `ui.js`, and its
  injected scripts/styles) and the vanilla spectator page (`watch.html`,
  `watch.js`) are deleted. Vue owns every operator and spectator browser
  route; `/`, `/ui.js`, `/watch.js` and the other legacy asset paths now 404
  behind the API handlers. `cmd/pokeui/ui/maps` stays: the Vue map views read
  `/maps/{id}.json`.
- `world.BuildGraph(source any)` no longer type-switches on its argument. It
  takes a `MapHeaderProvider`.
- The raw-ROM provider-factory registry (`worldmodel.RegisterROMProviderFactory`,
  `worldmodel.ProviderForROM`, `world.BuildGraphForROM`) is deleted. Callers
  that hold only ROM bytes resolve the provider through the detected game
  profile with `profiles.MapProvider`, so profile detection is the one place a
  ROM is matched to an adapter. `agent` no longer falls back to a raw-ROM graph
  when a profile lacks `game.MapTopologyProvider`; every runnable profile
  implements it.
- `agent.NewKnowledge` takes `*KnowledgeTopology`; the historical `any` and
  byte-map topology forms are gone.
- The extra `outcomesCompatibility` middleware, whose only purpose was to
  compensate for stale Swarm commands missing `-catalog`, is deleted. The
  normal catalog handler owns the explicit local no-catalog outcomes route.
- The `catalogOperatorCompatibility` label is retired: the remaining
  `catalogOperatorHTTPHandler` performs necessary SQLite history reads and
  overlays, not a mixed-version deployment fallback.
- The issue adapter uses the canonical admin URL via `sites.FromEnv`, and the
  checked-in farm/postgres stack exports canonical URL variables. Old variable
  names are no longer required in the current stack's process environment.

## Deployment verification before removing remaining aliases

After rolling out the new stack, inspect the effective Swarm service
environment and any separately managed `.env`/operator configuration.
The two legacy URL input names above must stay accepted until that check
is complete. Do not remove v4-v6 checkpoint migration or change stored
numeric/wire identifiers without a separately approved persistence-window
change and fixtures proving historical reads.
