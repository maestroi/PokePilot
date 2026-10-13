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
| `world.BuildGraphForROM([]byte)` registry lookup | `world.BuildGraph` is typed on `MapHeaderProvider`; callers that only hold ROM bytes (mostly `skill`, `agent`, `cmd/worldverify`, `cmd/pokerepro`) resolve the provider through the registered adapters with this explicit, separately named entry point. Removing it in isolation would move game detection back into generic callers. | Every production world consumer receives an explicit provider from the selected cartridge/game adapter, and raw-ROM construction remains only inside adapter packages. |
| Checkpoint files without persisted game/revision identity | Older supported checkpoint versions predate profile identity metadata. They are still read, while files that do carry identity are rejected on mismatch. | The support window excludes every checkpoint version that predates game/revision identity. |

## Retired

- The vanilla operator console (`cmd/pokeui/ui/index.html`, `ui.js`, and its
  injected scripts/styles) and the vanilla spectator page (`watch.html`,
  `watch.js`) are deleted. Vue owns every operator and spectator browser
  route; `/`, `/ui.js`, `/watch.js` and the other legacy asset paths now 404
  behind the API handlers. `cmd/pokeui/ui/maps` stays: the Vue map views read
  `/maps/{id}.json`.
- `world.BuildGraph(source any)` no longer type-switches on its argument. It
  takes a `MapHeaderProvider`; the raw-ROM path is the explicit
  `BuildGraphForROM` listed above.
- `agent.NewKnowledge` takes `*KnowledgeTopology`; the historical `any` and
  byte-map topology forms are gone.

## Still open

Rollout aliases in `cmd/pokewall` should only survive when a current
mixed-version deployment or persisted wall record demonstrably needs them. Their
removal is a separate #1543 slice and should document the specific deployment
window before changing the wire/storage contract.
