# The Yellow decomp, and where things are in it

Every Yellow fact this project calls DERIVED comes from the pokeyellow
disassembly, **vendored in this repository at `pokeyellow/`** — so it is
present in a fresh clone and in every agent worktree. See
`pokeyellow/UPSTREAM.md` for provenance and for what was left out.

Read it. Do not guess, and do not search the web: the ROM at
`roms/pokemon_yellow.gb` is byte-identical to the pokeyellow build
(sha1 `cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1`), so every file below
describes the exact bytes the emulator is running.

Paths in the decomp's own documentation are written as `scripts/Foo.asm`; here
they are `pokeyellow/scripts/Foo.asm`. This is the same trap as `pokered/`:
the bare path opens nothing.

## What you are probably looking for

| Question | File |
|---|---|
| What map id is X? | `pokeyellow/constants/map_constants.asm` — `map_const NAME, w, h ; $ID` |
| What does the game do when I stand here? | `pokeyellow/scripts/<MapName>.asm` |
| Which NPCs are on this map, and where? | `pokeyellow/data/maps/objects/<MapName>.asm` |
| Where do this map's warps and connections go? | `pokeyellow/data/maps/headers/<MapName>.asm` |
| What does that text box actually say? | `pokeyellow/text/<MapName>.asm` |
| Which tiles can I walk on? | `pokeyellow/data/tilesets/collision_tile_ids.asm` |
| Which tile is a door, a ledge, a bookshelf? | `pokeyellow/data/tilesets/door_tile_ids.asm`, `ledge_tiles.asm`, `bookshelf_tile_ids.asm` |
| What can I meet in this grass? | `pokeyellow/data/wild/maps/<MapName>.asm` |
| What does a trainer lead with? | `pokeyellow/data/trainers/parties.asm` (Jessie/James have their own entries) |
| When does it evolve, what does it learn? | `pokeyellow/data/pokemon/evos_moves.asm` |
| What does a mart stock, and for how much? | `pokeyellow/data/items/marts.asm` |
| What is at RAM address 0x____? | `pokeyellow/ram/wram.asm` (`wJoyIgnore::`, `wEventFlags::`, `wObtainedBadges::` …) |
| What is the exact address of a label? | `pokeyellow/pokeyellow.sym` |
| Why did input stop working? | `pokeyellow/ram/wram.asm` for `wJoyIgnore`, then grep `pokeyellow/scripts` for what wrote it |
| What is Pikachu doing? | `pokeyellow/constants/pikachu_emotion_constants.asm`, `wPikachu*` in `pokeyellow/ram/wram.asm` |

## Event flags: do not hand-count them

The same rule as Red applies, for the same reason.
`pokeyellow/constants/event_constants.asm` is a `const_def` counter full of
`const_skip N` and `const_next $XX`. Counting `const` lines gives the wrong
index. Yellow's flag layout is **not** Red's: take an index only from that file
(via the RGBDS counter, as `red/state/event_constants_test.go` does for Red) or
from a Yellow-owned `state.Event`. Never from a line number, and never from the
Red table.

## How Yellow differs from Red (measured, not assumed)

These are the facts that make Yellow a port rather than a reskin. All were
read out of the vendored decomp, and the address deltas were confirmed against
`pokeyellow/pokeyellow.sym`.

**RAM moved, almost uniformly by -1.** Do not copy Red's symbol table; every
address must be re-read from `pokeyellow/ram/wram.asm` / `pokeyellow.sym`.

| Symbol | Red | Yellow |
|---|---|---|
| `wJoyIgnore` | 0xCD6B | 0xCD6B (unchanged) |
| `wIsInBattle` | 0xD057 | 0xD056 |
| `wPartyCount` | 0xD163 | 0xD162 |
| `wPlayerMoney` | 0xD347 | 0xD346 |
| `wObtainedBadges` | 0xD356 | 0xD355 |
| `wCurMap` | 0xD35E | 0xD35D |
| `wYCoord` | 0xD361 | 0xD360 |
| `wXCoord` | 0xD362 | 0xD361 |
| `wEventFlags` | 0xD746 | 0xD746 (unchanged) |

**Every decode reads through a per-image address set, not a game's sym
package.** The decoders in `red/state` are the shared Gen I rules (the
party_struct, box_struct, BCD money, badge bitfield, menu registers and
event-flag encoding are byte-identical), but a decode rule is only correct at
the addresses the image actually uses. Decoding a Yellow image with Red's
addresses is a *silent* failure, not a crash: a fresh Yellow boot decodes as a
party of six with 300086 money, because Red's `wPartyCount` lands one byte into
Yellow's `wPartyMon1` and reads a real byte. So:

- `red/state.Addresses` holds every WRAM address a decode reads, embeds
  `BattleAddresses`, and carries the decoders as methods. `RedAddresses()`
  builds it from `red/sym`.
- `yellow/state.YellowAddresses()` builds the same struct from `yellow/sym`,
  and Yellow's decoders are one-line delegations — no decode logic is
  duplicated, so a fix to a shared struct serves both games.
- Gen I constants that genuinely do not move (struct field offsets like
  `MonSpecies`, buffer lengths like `TileMapLen`, fixed counts like
  `PokedexCount`) stay in `red/sym` and the shared decode bodies read them
  directly; they are not address-set fields.
- The skill layer resolves the set once per image
  (`skill.AddressesForROM` / `AddressesFor`, cached with the table set) and
  every read and decode goes through it. `agent` does the same.
- Nine decoder addresses were missing from `yellow/sym` and were added from
  `pokeyellow.sym`: `wCurrentBoxNum` 0xD59F, `wBoxCount` 0xDA7F, `wBoxMon1`
  0xDA95, `wPokedexOwned` 0xD2F6, `wPokedexSeen` 0xD309, `wPlayerCoins`
  0xD5A3, `wTwoOptionMenuID` 0xD12B, `wToggleableObjectFlags` 0xD5A5,
  `wToggleableObjectList` 0xD5CD.

Pinned by `skill.TestYellowDecodeUsesYellowAddresses`, which boots the real
image and asserts the fresh-game decode (money 3000, empty party, map
REDS_HOUSE_2F); it fails with map 0x12 the moment a decode ignores its own
address set.

**ROM table layouts moved.** The map-header pointer and bank tables are not at
Red's offsets. Feeding the Yellow ROM to `red/rom.ParseMap` parses only 33 of
248 map slots (versus 226 on the Red ROM) and fails the rest with out-of-range
header offsets. Every `bank:addr` constant in `red/rom` needs a Yellow
counterpart; the parsing *shapes* are Gen I and reusable, the addresses are
not. Implemented: `red/rom.Tables` parameterizes the parser and `yellow/rom`
supplies Yellow's three table addresses (`MapHeaderPointers` 0x3F:0x41F2,
`MapHeaderBanks` 0x3F:0x43E4, `Tilesets` 0x03:0x4558). `Tables` also carries a
per-game `ValidMapID` and `MaxMapID`, because the valid-map set differs at the
tail (below), and `CollisionListBank` (below). 227 Yellow maps now parse, and
PalletTown's warps/objects match the decomp exactly (`TestParseAllMaps`,
`TestParseTailMaps`, `TestWarpsMatchDecomp`).

**The geometry layer resolves tables per image too.** The skill layer's
readers -- `ParseMap`, the grid builder, the route graph, the wild and tileset
tables -- share Gen I's header/object *formats* but each reads them at that
image's *addresses*. The free `rom.ParseMap` / `world.Build` / `world.BuildGraph`
are Red's addresses by default, and feeding them a Yellow image is again a
silent failure: Red's collision-bank assumption (a pointer below `$4000` means
bank 0) is wrong on Yellow, whose lists were assembled into bank 1, so Pallet
Town decodes ~6 blocked tiles instead of ~139 and every plan walks through
buildings with no error to surface it. The runtime now resolves the table set
once per image (`skill.graphForROM`, cached) and every reader goes through the
`...ForTables` variants. `world.FindPath` is table-agnostic (it operates on an
already-built grid) and needed no change. The generic agent dispatch keeps Red's
tables as the registry fallback for a profile without a registered provider.

Pinned by `skill.TestYellowSkillLayerUsesYellowTables`: Yellow and Red must
resolve different table addresses, and Yellow's Pallet Town grid must be
walkability-identical to Red's (both 139 blocked tiles, 0 diffs) — the failure
is the divergence, and it fails the moment `graphForROM` stops switching.

`skill/probe_test.go` is per-ROM now as well. It is the permanent measurement
tool, and it used to decode `0x12` (Red's `wCurMap`) on a Yellow image and then
fail building the grid. Pointing `POKEMON_RED_ROM` at a Yellow ROM now reports
the live map as `0x0026` and prints Yellow's own warps and grid.

**Yellow's valid-map set is not Red's.** `AGATHAS_ROOM` is `$F7` in *both*
games, and Yellow appends `SUMMER_BEACH_HOUSE` at `$F8`. Red's predicate
rejects `$F8` and above outright, so it cannot be reused: it would drop the
beach house. Copying Red's exclusion list is equally wrong, because a naive
port also excluded `$F7` (a real map). `yellow/rom.validMapID` keeps the
shared unused-slot list and extends the bound to `$F9`; `TestParseTailMaps`
pins both tail maps.

**Agent dispatch is by provider, not by game name.** `agent.run` builds the
travel graph through `buildMapGraph(profile, romData)`, which looks up a
`MapGraphProvider` registered by `GameID` — the same pattern as the knowledge
topology and objective catalog providers. Yellow registers one that calls
`world.BuildGraphForTables(yellowrom.Tables(), romData)`; the profile registry
is the only place game identity is decided. Verified on the real ROM: the
Yellow graph has 227 nodes, versus 33 under Red's tables
(`agent.TestYellowGraphDispatch`).

**The wild-data and tileset engines are byte-identical; only the table
addresses move.** `pokeyellow/engine/overworld/wild_mons.asm` diffs clean
against Red's, and the tileset header layout is the same, so the skill
readers stay format-shared. Yellow's `WildDataPointers` is `03:4B95` (Red
`03:4EEB`) and `Tilesets` is `03:4558` (Red `03:47BE`). `Moves` (`0E:4000`)
and `BaseStats` (`0E:43DE`) are at the *same* addresses in both games.
`skill.tablesForROM` resolves the set from the profile registry and caches it
per ROM image; Route 1 then reads a 25% grass rate over 10 slots matching
`data/wild/maps/Route1.asm` (`skill.TestYellowWildTables`).

**`wCurMapTileset` carries a flag bit Red does not have.** Yellow defines
`BIT_NO_PREVIOUS_MAP` (bit 7) in `wCurMapTileset`
(`pokeyellow/constants/ram_constants.asm:56`), set on save and cleared by
`LoadMapHeader` (`pokeyellow/home/overworld.asm:1805`). A decoder that masks
only the low bits will see tileset 0x84 as 4 after a save. Yellow also has 25
tilesets, not 24: `BEACH_HOUSE` is appended at id 24.

**Yellow relocated the tileset collision lists into bank 1.** This is the
engine diff that silently corrupts the whole walkable grid without erroring.
`_IsTilePassable` (`pokeyellow/engine/gfx/sprite_oam.asm:214`) loads
`wTilesetCollisionPtr` straight into `hl` with **no bank switch**, so the
collision list must already be in the mapped bank. In Red the lists are
assembled into bank 0 (`Overworld_Coll` is `00:1735`) and every pointer is
below `$4000`, which the CPU maps to ROM bank 0 unconditionally — so
resolving "pointer below `$4000` means bank 0" is correct there by accident.
Yellow moved `Overworld_Coll` to `01:4AC2`; its pointer is `≥ $4000`, and the
tileset entry's bank byte is *not* the collision bank (tileset 0 names bank
`0x19`, which is `Overworld_GFX`'s bank). The bank is a build-layout fact the
pointer cannot reveal. `rom.Tables.CollisionListBank` carries it; the grid
uses it for pointers `≥ $4000` and keeps bank 0 below. Without it, Pallet
Town decodes 6 blocked tiles to Red's 139 and every Yellow plan walks through
buildings. Pinned by `yellow/rom.TestPalletGridMatchesRed`, which asserts a
Yellow grid is tile-identical and walkability-identical to Red's for the
shared map, plus `skill.TestSpriteBlockersExcludesFollower`.

**Yellow's Pikachu follower is passable, not a wall.** Yellow dedicates sprite
slot 15 to the follower (`PIKACHU_SPRITE_INDEX = NUM_SPRITESTATA_STRUCTS - 1`
= 15; `wSpritePikachuStateData1` is struct 15 in the shared sprite array).
`IsSpriteInFrontOfPlayer` scans slots 1–15 in *both* games, so the follower is
inside Red's decode range and would be counted as a blocked tile. But
`CollisionCheckOnLand` (`pokeyellow/home/overworld.asm`) matches the sprite
against `PIKACHU_SPRITE_INDEX`, and if `wPikachuOverworldStateFlags` bit 1
(following) is set and either B is held or `wPikachuCollisionCounter`
(`0xD434`) is zero, there is **no collision**: the player bumps the follower a
bounded number of times, then steps onto its tile. So the follower must not be
planned around. Implemented as a per-ROM fact on the table set
(`romTableSet.passableSpriteSlot`, 15 for Yellow, 0 for Red) consumed by
`skill.spriteBlockers`; the slot is excluded on Yellow only, and Red's slot 15
stays solid (`skill.TestSpriteBlockersExcludesFollower`).

**The map header is populated lazily.** `wCurMapHeader` is copied from ROM by
`LoadMapHeader` during the map-load sequence, not when `wCurMap` is set. A
state captured the instant `wCurMap` becomes 0x26 can legitimately have
`wCurMapHeight`/`wCurMapWidth` still zero; the header goes live roughly 100
input iterations later. Any controllability or world decode that reads the
header must wait for it, not just for `wCurMap`.

**`BattleMonLevel` is the one battle field that escaped the address struct.**
`DecodeBattleAt` took a `BattleAddresses` so a game could supply its own
addresses, but read the active monster's *level* straight from this package's
`sym` constants — so Yellow's `DecodeBattle` reported Red's `0xD022` instead
of Yellow's `0xD021`, silently, because the value at the wrong address still
looked like a level. Every battle field now resolves through
`BattleAddresses.BattleMonLevel`, and `TestDecodeBattleAtReadsActiveLevelFrom
Addresses` pins it by supplying an address that differs from Red's and
asserting the decoder reads only there.

**Map ids are mostly shared.** This is the good news. The `map_const` sequence
is identical to Red's through almost the whole table; the only differences are
`CERULEAN_TRADE_HOUSE` → `CERULEAN_MELANIES_HOUSE` (same slot) and one extra
map, `SUMMER_BEACH_HOUSE`, appended at `$F8`. So the map vocabulary ports
nearly unchanged — it is the ROM *tables* and the *contents* (headers,
objects, scripts, wilds) that differ. Implemented: `yellow/state.MapName`
covers all 249 ids.

**The starter is not a choice.** Oak gives Pikachu; the rival takes Eevee.
There is no ball-selection scene, so `skill/boot.go`'s starter handling and
`skill/starter_request.go` do not apply. The three original starters are gifts
obtained later, each with its own script:

- Bulbasaur — Melanie, `pokeyellow/scripts/CeruleanMelaniesHouse.asm`
- Charmander — `pokeyellow/scripts/Route24.asm`
- Squirtle — the officer, `pokeyellow/scripts/VermilionCity_2.asm`

**Pikachu follows you.** A trailing overworld sprite with its own state:
`wPikachuOverworldStateFlags`, `wPikachuSpawnState`, `wPikachuFollowCommandBuffer`,
`wPikachuMovementXOffset`/`wPikachuMovementYOffset` and friends in
`pokeyellow/ram/wram.asm`. Any sprite/blocker model that assumes the player is
the only moving object on the tile grid must account for it.

**Other deltas to plan for:** rebalanced gym-leader levels and trainer parties,
Jessie/James encounters, Yellow-specific wild tables and mart stock, the
name-menu presets differ from Red's ASH/GARY defaults, and the save layout
differs. The CGB header flag means banked WRAM is in play — carry the bank in
`game.MemorySymbol` and advertise `game.FeatureBankedMemory` from the Yellow
profile.

## How far the skill layer drives Yellow (measured)

Boot-to-Pallet-Town is the exercise that makes the three migrations above
observable at once, because a broken decode, a broken grid, or a broken
movement cadence each strand the player somewhere different. Measured on the
real image, through the public skill entry points only:

| stage | result |
|---|---|
| power-on → controllable overworld | map 0x26 (REDS_HOUSE_2F), (3,6), money 3000, empty party |
| `BootToOverworld` name presets | selects ASH / GARY (index 2 of NEW NAME, YELLOW, ASH, JACK and NEW NAME, BLUE, GARY, JOHN) |
| bedroom walk to the stairs warp | (3,6)→(7,1) in 9 steps, warp fires |
| REDS_HOUSE_1F walk to the exit | (7,1)→(2,7) in 11 steps |
| exit warp → Pallet Town | map 0x00, (5,8), controllable |

Each row decodes through the Yellow address set and builds geometry from the
Yellow table set; a regression in either shows up as a stuck player or a
garbage decode, not an error. The Oak cutscene past Pallet Town still seizes
input (the `ctrl=false → ctrl=true` dialogue pattern), which is the remaining
skill-layer work, not a decode defect.

## The Yellow opening, driven end to end

The three migrations above (WRAM reads, decoders, geometry) are each verified
in isolation, but the opening sequence is what makes them observable *together*:
a broken decode strands the player in a misread map, a broken grid refuses to
pathfind, and a broken movement cadence never reaches the trigger tile. Driven
on the real image through the public skill entry points only:

| stage | result |
|---|---|
| power-on → controllable overworld | map 0x26 (REDS_HOUSE_2F), (3,6), money 3000 |
| `BootToOverworld` name presets | ASH / GARY |
| bedroom → stairs warp → 1F | (3,6)→(7,1) warp, then (7,1)→(2,7) |
| 1F exit → Pallet Town | map 0x00, (5,6) |
| pathfind north to the Route 1 exit | (5,6)→(10,1), then (10,0) |
| Oak cutscene | `wPalletTownCurScript` 1→7; `wJoyIgnore` \$FC/\$FF seizes input |
| wild Pikachu + Oak leads to lab | map 0x28 (OaksLab) |
| lab script: rival fed up, choose mon | `wOaksLabCurScript` 5→6, control returns |
| walk to the Eevee ball at (7,4), face up, A | script 9 (CHOSE_STARTER) |
| rival takes a ball, speeches | script 0xB (PLAYER_RECEIVES_PIKACHU) |
| **starter awarded** | **species 84 (PIKACHU), level 5, 18/18 HP, party=1** |

Two facts about that drive are worth keeping, because they are the shape of
the remaining work rather than defects:

- **The Oak sequence is dialogue, not geometry.** Every stall after Pallet Town
  is `wJoyIgnore` nonzero with a text box open. The fix is paging (A) with the
  script variables (`wPalletTownCurScript` 0xD5F0, `wOaksLabCurScript` 0xD5EF)
  as the progress signal, not more pathfinding.
- **`wPartyCount` is written before the mon struct.** Between "received a
  PIKACHU" and the party data landing, `wPartyCount` is 1 while the 44-byte
  mon region at `wPartyMon1` (0xD16A) is still zero. A decoder that keys off
  the count alone will hand back a level-0 species-0 mon. The fixture pins the
  *settled* state, which is why it reads species 84 / level 5.

**The fixture is the regression.** `skill/failure/yellow_starter.state` is the
settled post-starter RAM, and `skill.TestYellowStarterFixture` decodes it
through the Yellow address set and asserts the lead is Pikachu lvl 5 with a
live HP pool. It also asserts the *Red* decode of the same bytes disagrees —
that is what makes the fixture discriminating rather than a tautology. A
regression in any of the three migrations fails it: the wrong address set reads
the species one byte into the party struct and gets a different mon.

## First battle on Yellow (rival), and the follower it leaves behind

The rival fight is the first subsystem a decode test cannot cover: `Battle()`
must read the Yellow battle block (enemy species, HP, move list) to choose and
observe moves, and a Red-addressed read of that block on Yellow RAM picks up
plausible-looking garbage -- a wrong species, a wrong HP, moves that do not
exist. So it is driven as a battle, not asserted as bytes.

Load the settled post-starter fixture, page Oak's tutorial chain (A), walk
south to y=6 (the rival challenge triggers on that tile), page the challenge
text, then hand control to `Battle(e, StatAwareMove(romData))`. Measured:

| fact | value |
|---|---|
| battle entered | `wIsInBattle` set, enemy species 84 (the rival's Pikachu) |
| `Battle()` result | `ResultWon` (0) |
| determinism | 3 consecutive runs, identical outcome |
| post-battle script | `wOaksLabCurScript` 0x16 (NOOP -- the opening is over) |
| follower left behind | `wSpritePikachuStateData1` (0xC1F0) = picture id 0x49 |

The follower is the second Yellow-only mechanic the battle produces, and it is
pinned against the same live RAM (`skill.TestYellowFollowerLive`): the Pikachu
occupies sprite slot 15 (`PIKACHU_SPRITE_INDEX = NUM_SPRITESTATEDATA_STRUCTS -
1`), and `CollisionCheckOnLand` lets the player bump it a bounded number of
times and then walk through, so planning around its tile strands the player.
`tablesForROM` marks slot 15 passable for a Yellow image and leaves it 0 (solid)
for Red, whose slot 15 is an ordinary NPC. The test asserts both, and asserts
the follower byte is nonzero in the first place -- a model naming the wrong
slot is inert against a synthetic RAM that never wrote it.

Pinned by `skill.TestYellowRivalBattle` (the fight) and
`skill.TestYellowFollowerLive` (the follower it leaves), both against
`skill/failure/yellow_starter.state` / `yellow_post_rival.state`.

## Grepping it without drowning

The decomp is ~1,600 files, and dumping one into a worker's context is how
several attempts on `pokered/` died. Scope the search:

    grep -rn EVENT_GOT_POKEDEX pokeyellow/scripts
    sed -n '1,80p' pokeyellow/scripts/VermilionCity_2.asm   # a slice, not the file

`pokeyellow/engine/` is the game's own logic (battle, overworld, menus,
Pikachu emotion). It answers "how does the game decide X", which is rarely the
question — the map, script, and data files above almost always are.
