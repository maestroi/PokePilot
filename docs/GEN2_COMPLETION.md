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
| Elite Four Will → Karen, Lance, Hall of Fame | implemented | league stage replay workflow never run in CI |
| S.S. Ticket, S.S. Aqua first landing | implemented | Kanto capture/replay harness exists |
| Kanto: Surge, Sabrina, Erika, Janine | implemented | |
| Power Plant: manager, Cerulean Rocket, Machine Part, restore | verified-replay | #2945 |
| Copycat, Lost Doll, Magnet Train Pass, Misty date, Cascade | verified-replay | #2945 |
| Radio Card, EXPN Card, Snorlax, Brock, Blaine, Blue invite, Earth | verified-replay | #2947 |
| Oak's Mt. Silver permit, Red | verified-replay | #2947; Red is proven by the disappearance event |

No unimplemented mandatory story milestone is known. **No full fresh-save →
Red run has been observed.** Do not call Gen 2 verified until one has.

## Systems

| System | Status | Where |
| --- | --- | --- |
| RAM decode: party, bag (all pockets), badges (Johto+Kanto), money, dex, map, RTC/time of day | implemented, used live | `gs/profile`, `gs/sym` |
| Event flags / story facts | implemented | `gs/profile/progression.go` |
| Native world graph, warps, connections, collision (incl. ice, pits, whirlpool, waterfall) | implemented | `gs/rom`, `web/src/shared/gen2WorldManifest.generated.ts` |
| Field moves Cut/Surf/Strength/Flash/Fly/Whirlpool/Waterfall/Rock Smash + teaching carrier | implemented | `agent/gs_field_carrier.go`, `gs_surf.go`, `gs_pit_puzzles.go` |
| Healing (nearest native Center), lead training after a combat loss | implemented | `gs_party_recovery.go`, `gs_combat_recovery.go` |
| Trainer/gym/scripted battles | implemented | shared battle driver + Gen 2 decoders |
| League items: Indigo Mart purchase, in-League medicine/PP | implemented, unqualified | `gs_league_resources.go`, `gs_league_stock.go` |
| PC storage | implemented | `gs_pc.go` |
| Clock / day-night | implemented | `gs_clock_executor.go` |
| Pokégear radio (Radio Card quiz, EXPN Card, Poké Flute channel) | implemented | `gs_radio_card_quiz.go`, `gs_kanto_radio_snorlax.go` |
| Phone calls | partial | Elm's call handled in-story; phone rematches not a capability (#2660) |
| Blackout recovery, checkpoint resume | implemented | shared runtime |
| Full Items pocket blocking a story gift (Dragon Fang) | gap | typed diagnostic only; no free-a-slot repair |
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
| Full Production E2E mode | gap: gated to Red (`web/src/operator/newRunMode.ts`, media pipeline expects a Champion/Hall of Fame proof) |
| Autofixer / triage | shared: Gen 2 failures go through the same typed failure + checkpoint path; nothing Gen 2 specific has been audited |

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

## Next tasks (in priority order)

1. Run a fresh Gold full-game run (default goal) on the farm; triage the
   first failure from its checkpoint.
2. Fix the coastal runner inputs and run the League and Kanto stage replays for Gold and Silver.
3. Full Production E2E for Gen 2: accept a Red-defeat proof and Gen 2
   media metadata, not only Red's Hall of Fame.
4. Free a bag slot (toss or deposit) before a story gift when the Items pocket is full.
5. Phone rematches as a capability (#2660). Optional for the story.

## Session log

- 2026-11-24: audit; this tracker; Gen 2 goals (Kanto badges, 16 badges,
  full-game default), Gen 2 milestones, postgame spectator route line.
