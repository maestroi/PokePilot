# Compatibility baseline

Last reviewed: 2026-10-13 for issue #1543.

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
| `POKEPILOT_SPECTATOR_URL` / `POKEPILOT_RUN_BASE_URL` (`sites.EnvSpectatorURL`, `sites.EnvRunBaseURL`) | `deploy/postgres.yml` and `deploy/farm.yml` still set the old names, and `cmd/pokeissues` reads `POKEPILOT_RUN_BASE_URL` directly. Dropping them is a production stack env change, not a code cleanup. | The deploy stacks and `pokeissues` use `POKEPILOT_PUBLIC_BASE_URL` / `POKEPILOT_ADMIN_BASE_URL`, and the swarm env no longer sets the old names. |
| Checkpoint files without persisted game/revision identity | Older supported checkpoint versions predate profile identity metadata. They are still read, while files that do carry identity are rejected on mismatch. | The support window excludes every checkpoint version that predates game/revision identity. |

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

## Still open

Rollout aliases in `cmd/pokewall` should only survive when a current
mixed-version deployment or persisted wall record demonstrably needs them. Their
removal is a separate #1543 slice and should document the specific deployment
window before changing the wire/storage contract.
