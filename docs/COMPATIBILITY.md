# Compatibility baseline

Last reviewed: 2026-09-27 for issue #1543.

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
| `world.BuildGraph([]byte)` / ROM provider-factory bridge | A material part of the runtime world call graph still starts with ROM bytes instead of an adapter-owned `MapHeaderProvider`. Removing it in isolation would move game detection back into generic callers. | Every production world consumer receives an explicit provider from the selected cartridge/game adapter, and raw-ROM construction remains only inside adapter packages. |
| Checkpoint files without persisted game/revision identity | Older supported checkpoint versions predate profile identity metadata. They are still read, while files that do carry identity are rejected on mismatch. | The support window excludes every checkpoint version that predates game/revision identity. |

## Not protected as compatibility

The retired vanilla operator bundle under `cmd/pokeui/ui` is **not** part of
the persistence or external API baseline. Vue already owns production root
routing; remaining directly served legacy assets are cleanup work in #1543, not
a compatibility promise.

Likewise, rollout aliases in `cmd/pokewall` should only survive when a current
mixed-version deployment or persisted wall record demonstrably needs them. Their
removal is a separate #1543 slice and should document the specific deployment
window before changing the wire/storage contract.
