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
| What does a trainer lead with? | `pokeyellow/data/trainers/parties.asm` (`BrockData:` at line 654; Jessie/James have their own entries) |
| When does it evolve, what does it learn? | `pokeyellow/data/pokemon/evos_moves.asm` |
| What does a mart stock, and for how much? | `pokeyellow/data/items/marts.asm` |
| What is at RAM address 0x____? | `pokeyellow/ram/wram.asm` (`wJoyIgnore::`, `wEventFlags::`, `wObtainedBadges::` …) |
| What is the exact address of a label? | `pokeyellow/pokeyellow.sym` |
| Why did input stop working? | `pokeyellow/ram/wram.asm:1068` for `wJoyIgnore`, then grep `pokeyellow/scripts` for what wrote it |
| What is Pikachu doing? | `pokeyellow/constants/pikachu_emotion_constants.asm`, `wPikachu*` in `pokeyellow/ram/wram.asm` |

## Event flags: do not hand-count them

The same rule as Red applies, for the same reason.
`pokeyellow/constants/event_constants.asm` is a `const_def` counter full of
`const_skip N` and `const_next $XX`. Counting `const` lines gives the wrong
index. Yellow's flag layout is **not** Red's: never reuse a Red bit index, and
never take one from a line number.

## Do not treat Yellow as a Red reskin

Map ids are mostly shared (Yellow appends `SUMMER_BEACH_HOUSE` at `$F8`), but
WRAM symbols, ROM table addresses, and scripts are not. Oak gives Pikachu; the
rival takes Eevee. There is no ball-selection scene. The three original
starters are later gifts:

- Bulbasaur — Melanie, `pokeyellow/scripts/CeruleanMelaniesHouse.asm`
- Charmander — `pokeyellow/scripts/Route24.asm`
- Squirtle — the officer, `pokeyellow/scripts/VermilionCity_2.asm`

Pikachu follows the player as overworld sprite slot 15. Any sprite/blocker
model that assumes the player is the only moving object on the tile grid must
account for it.

## Grepping it without drowning

The decomp is ~1,600 files, and dumping one into a worker's context is how
several attempts on `pokered/` died. Scope the search:

    grep -rn EVENT_GOT_POKEDEX pokeyellow/scripts
    sed -n '1,80p' pokeyellow/scripts/VermilionCity_2.asm   # a slice, not the file

`pokeyellow/engine/` is the game's own logic (battle, overworld, menus,
Pikachu emotion). It answers "how does the game decide X", which is rarely the
question — the map, script, and data files above almost always are.
