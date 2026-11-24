# Gen 2 (Gold/Silver) completion tracker

Working checklist for finishing Gen 2. Update it in the same PR as the
change. Detailed per-slice evidence and replay commands live in
`docs/QUALIFICATION.md`; this file is the index and resume point.

Supported games: **Pokémon Gold, Pokémon Silver** (`pokemon-gold`,
`pokemon-silver`). Crystal is not supported (no profile).

Status legend:

- **verified-live**: a real farm run reached it on a retail ROM.
- **verified-replay**: a retail checkpoint replay passed (private ROM runner/local).
- **implemented**: controller + ROM-free tests exist; waiting on live/replay proof.
- **gap**: missing or known broken.

## Story chain

Every step is a `gs/profile` progress fact proven by a cartridge event, badge
bit or key item, and an `agent/gs_*.go` controller. Offers live in
`gsObjectiveAdapter.ProgressionObjectives` and do not depend on the run goal.

| Segment | Status | Notes |
| --- | --- | --- |
| New game, starter, Mystery Egg, Pokédex, rival | verified-live | `gs_opening.go` |
| Badges 1–6 (Zephyr → Mineral), Sprout Tower, Slowpoke Well, Ilex, Sudowoodo, Burned Tower, Lighthouse/Cianwood | verified-live | Operator ran 8-badge runs; coastal replay infra broken (see below) |
| Lake of Rage, Mahogany, Rocket Hideout, Glacier (7) | verified-live | operator-confirmed 8-badge runs |
| Radio Tower takeover, Underground, Ice Path, Clair, Dragon's Den, Rising (8) | verified-live | operator-confirmed 8-badge runs |
| Victory Road → Indigo Plateau | verified-live (once) | run-3om7ptfumukt9aaxwusxdq9sj, after a blackout retry |
| Elite Four Will → Karen, Lance, Hall of Fame | verified-replay (local, Gold) | 2026-11-24: from run-zzrclw1r60a8g's real Radio Tower checkpoint (#2928), the League reset, Will, Koga, Bruno, Karen, Lance and the Hall of Fame all passed on retail Gold; ends in New Bark Town. Needed in-battle medicine and the Champion aftermath fix (see Systems). Silver not replayed |
| S.S. Ticket, S.S. Aqua first landing | verified-replay (local, Gold) | 2026-11-24 chain from the post-Hall-of-Fame state; needed the ROM routing below plus ship fixes (boarding prompt, B1F east ladder, ROM-checked stand tiles) |
| Kanto: Surge, Sabrina, Erika, Janine | verified-replay (local, Gold) | Same chain. Needed Saffron Gym teleporter proofs, the Cycling Road bicycle barrier, off-screen NPC blockers and Janine's stand tile (1,9) |
| Power Plant: manager, Cerulean Rocket, Machine Part, restore | verified-replay (local, Gold) | Needs Surf via the Route 9 lake channel (`gsKantoSurfToPowerPlant`). Leaving the strip by Surf once failed to settle at Route 10 North (15,10) and passed on retry |
| Copycat, Lost Doll, Magnet Train Pass, Misty date, Cascade | verified-replay (local, Gold) | Fan Club stand tile was a table (fixed to (1,3)) |
| Radio Card, EXPN Card, Snorlax, Brock, Blaine, Blue invite, Earth | verified-replay (local, Gold) | Radio Card came from the real run. Snorlax needed the 2x2 big-object blocker and seam re-entry |
| Oak's Mt. Silver permit, Red | permit verified-replay (local, Gold); **Red: gap** | The route to Red and the battle trigger work. Red beat a solo L84 Meganium three times. Gen 2 combat recovery only trains the lead (capped at L60, Johto grind sites), so after a Red loss no executable preparation exists. See Next tasks |

No unimplemented mandatory story milestone is known. **No full fresh-save →
Red run has been observed.** Do not call Gen 2 verified until one has.

## Systems

| System | Status | Where |
| --- | --- | --- |
| RAM decode: party, bag (all pockets), badges (Johto+Kanto), money, dex, map, RTC/time of day | implemented, used live | `gs/profile`, `gs/sym` |
| Event flags / story facts | implemented | `gs/profile/progression.go` |
| Native world graph, warps, connections, collision (incl. ice, pits, whirlpool, waterfall) | implemented | `gs/rom`, `web/src/shared/gen2WorldManifest.generated.ts` |
| Post-League routing topology | verified-replay (local, Gold) to Red's room | `gs/rom/world_runtime.go`: hand slice + ROM headers/collision for Kanto, ports, Fast Ship, Magnet Train, Route 28, Mt. Silver. Before 2026-11-24 routing had no Kanto maps at all |
| Routing blockers | verified-replay (local, Gold) | stationary NPCs (incl. off-screen `wMapObjects`) and profile script barriers (Cycling Road gates without a BICYCLE) are durable; same-map teleporter maps get reachability proofs |
| Field moves Cut/Surf/Strength/Flash/Fly/Whirlpool/Waterfall/Rock Smash + teaching carrier | implemented | `agent/gs_field_carrier.go`, `gs_surf.go`, `gs_pit_puzzles.go` |
| Healing (nearest native Center), lead training after a combat loss | implemented | `gs_party_recovery.go`, `gs_combat_recovery.go` |
| Trainer/gym/scripted battles | implemented | shared battle driver + Gen 2 decoders |
| League items: Indigo Mart purchase, between-room medicine/PP | verified-replay (local, Gold) | `gs_league_resources.go`, `gs_league_stock.go` |
| In-battle medicine (HP/status/Full Restore, bench sacrifice heal) | verified-replay (local, Gold) | profile-owned `game.Medicine` table (`gs/profile/medicine.go`, `gen1/medicine.go`); Gen 2 had no in-battle items before this. Battle PACK USE/QUIT box decoded in `gs/profile/field_item.go` |
| PC storage | implemented | `gs_pc.go` |
| Clock / day-night | implemented | `gs_clock_executor.go` |
| Pokégear radio (Radio Card quiz, EXPN Card, Poké Flute channel) | implemented | `gs_radio_card_quiz.go`, `gs_kanto_radio_snorlax.go` |
| Phone calls | partial | Elm's call handled in-story; phone rematches not a capability (#2660) |
| Blackout recovery, checkpoint resume | implemented | shared runtime |
| Full Items pocket blocking a story gift (Dragon Fang) | implemented | stores a non-medicine stack in the player's PC (`gs_item_storage.go`); deposit menu ROM-tested on Gold+Silver, full-bag Dragon's Den path not yet run |
| Catching / team building beyond story needs | partial | not required for the main story |

## Goals, UI, telemetry

| Item | Status |
| --- | --- |
| New Run goals: 1–8 Johto badges, Indigo, Hall of Fame, S.S. Ticket, first landing, all 8 individual Kanto badges, Magnet Train Pass, 16 badges, full game (Red) | done |
| Default Gen 2 goal (UI and MCP) = full game `progress:gs_mt_silver_red_defeated`; Hall of Fame is not completion | done |
| `gs_supported_frontier` preset | legacy, still stops at Mineral; the coastal qualification relies on that meaning |
| Player milestones (Pokédex, HMs, Rocket, Radio Tower, Champion, Hall of Fame, Kanto beats, Red) | done (`gs/profile/milestones.go`) |
| Spectator route line: Johto → Hall of Fame, then Kanto in controller order → Mt. Silver · Red | done |
| Spectator goal meter for full game: N/16 badges, then Red | done |
| Full Production E2E mode (New Run + Media tab) | done in UI: Gold/Silver target Red and require a `goal_complete` or `Red` milestone proof. The server pipeline was already game-agnostic. Not exercised on a real Gen 2 run |
| Autofixer / triage | works for Gen 2: issue #2928 (Gold League checkpoint) was filed, fixed (#2933) and replays offline with `pokerepro -bundle … -verify` using `POKEMON_GOLD_ROM`. Portable repro selects the ROM env by game |

## External blockers

- **Gen2 Coastal ROM Qualification has failed every day (40/40)**: the
  `pokepilot-rom` runner lacks the private Gold ROM or the
  `gs-coastal/gold/post-cure/start.state` checkpoint
  (`Missing private gold ROM or post-cure checkpoint`). An operator has to fix
  the runner. This is not a code failure.
- The League and Kanto stage replay workflows have never run in Actions. Run
  them once the qualification corpus is mounted on the runner.
- The prod wall could not reach Postgres on 2026-11-24, so live run history
  was not checked for this audit.

## Local League replay without a corpus

When only a farm failure checkpoint exists (no first-clear corpus), a
throwaway test in `agent` can load the `.state`, then repeatedly run
`ExecuteWithAdapter` on the first unmet League step (Will → … → Hall of Fame).
After a League reset, start again from Will. Do not commit such harnesses or
state files.

## Next tasks (in priority order)

1. **Red needs a stronger team (gap).** Gen 2 combat recovery trains only
   the lead, capped at `gsCombatRecoveryMaxLevel` = 60, on Johto grind sites.
   It needs (a) post-League grind sites (Route 28 / Silver Cave) and a higher
   cap, and (b) bench training: the readiness model already assigns the
   deficit past the ceiling to the bench. Also heal at the Silver Cave Center
   before climbing. Repro: the local Kanto chain from the Mt. Silver permit
   state (see "Local chain replays").
2. Merge #2952 (in-battle medicine), then the stacked Kanto routing PR.
3. Run a fresh Gold full-game run (default goal) on the farm once Postgres is
   back (`pokefarm_postgres` was 0/1 on 2026-11-24); triage the first failure.
4. Replay the same chains on Silver.
5. Check that Gen 2 episode/Short renders (theme `pokegold-gen2`) look right
   on the first finished Gen 2 full-game run.
6. Phone rematches as a capability (#2660). Optional for the story.

## Local chain replays

Throwaway harnesses (never commit them or the `.state` files) drove every
step above on retail Gold. Each loads a real checkpoint and repeatedly runs
`ExecuteWithAdapter` on the first unmet milestone of an ordered chain,
saving a state after each pass:

- League: from run-zzrclw1r60a8g's Radio Tower checkpoint (issue #2928),
  chain Will → Lance → Hall of Fame (after a reset, restart from Will).
- Kanto: from the post-credits New Bark state, chain S.S. Ticket → first
  landing → Kanto gyms and story in `gsKantoReplaySpecs` order → Red.

There is no planner here: no heal/train objectives and no LLM, so combat
losses in a chain are not proof that a live run would lose.

## Session log

- 2026-11-24: audit; this tracker; Gen 2 goals (Kanto badges, 16 badges,
  full-game default), Gen 2 milestones, postgame spectator route line,
  Full E2E for Gold/Silver (PR #2950, merged). Dragon Fang full-bag deposit (PR #2951).
- 2026-11-24 (later): the farm Postgres was down (`pokefarm_postgres` 0/1), so
  no farm runs. Replayed the League locally from a real Gold checkpoint and
  found two blockers: Gen 2 never used items in battle (Gen 1 ids were
  hardcoded in the generic policy), and `skill.Battle` settlement pressed
  through Lance's ending and credits. Both fixed; Gold reaches the Hall of
  Fame.
- 2026-11-24 (Kanto): routing had no Kanto maps. Added the ROM-backed
  post-League topology and fixed every blocker the Gold chain hit, from the
  S.S. Aqua to Red's room (stationary and off-screen NPCs, big sprites,
  teleporter proofs, seam re-entry, bicycle gates, the Power Plant channel,
  wrong stand tiles). Red is the remaining gap (team strength).
