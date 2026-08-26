# The decomp, and where things are in it

Every fact this project calls DERIVED came from the pokered disassembly, and it
is **vendored in this repository at `pokered/`** — so it is present in a fresh
clone and in every agent worktree. See `pokered/UPSTREAM.md` for provenance and
for what was left out.

Read it. Do not guess, and do not search the web: the ROM at
`roms/pokemon_red.gb` is byte-identical to the pokered build
(sha1 `ea9bcae617fdf159b045185467ae58b2e4a48b9a`), so every file below
describes the exact bytes the emulator is running.

Paths in the decomp's own documentation are written as `scripts/Foo.asm`; here
they are `pokered/scripts/Foo.asm`. Several runs were spent discovering that the
bare path opened nothing.

## What you are probably looking for

| Question | File |
|---|---|
| What map id is X? | `pokered/constants/map_constants.asm` — `map_const NAME, w, h ; $ID` |
| What does the game do when I stand here? | `pokered/scripts/<MapName>.asm` |
| Which NPCs are on this map, and where? | `pokered/data/maps/objects/<MapName>.asm` |
| Where do this map's warps and connections go? | `pokered/data/maps/headers/<MapName>.asm` |
| What does that text box actually say? | `pokered/text/<MapName>.asm` |
| Which tiles can I walk on? | `pokered/data/tilesets/collision_tile_ids.asm` |
| Which tile is a door, a ledge, a bookshelf? | `pokered/data/tilesets/door_tile_ids.asm`, `ledge_tiles.asm`, `bookshelf_tile_ids.asm` |
| What can I meet in this grass? | `pokered/data/wild/maps/<MapName>.asm` |
| What does a gym leader lead with? | `pokered/data/trainers/parties.asm` (`BrockData:` at line 643) |
| When does it evolve, what does it learn? | `pokered/data/pokemon/evos_moves.asm` |
| What does a mart stock, and for how much? | `pokered/data/items/marts.asm` |
| What is at RAM address 0x____? | `pokered/ram/wram.asm` (`wJoyIgnore::`, `wEventFlags::`, `wObtainedBadges::` …) |
| What is the exact address of a label? | `pokered/pokered.sym` |
| Why did input stop working? | `pokered/ram/wram.asm:890` for `wJoyIgnore`, then grep `pokered/scripts` for what wrote it |

## Event flags: do not hand-count them

`pokered/constants/event_constants.asm` is a `const_def` counter, and it is full
of `const_skip N` and `const_next $XX`. Counting `const` lines gives the wrong
index — that is how S3-1 shipped bit 38 for an event that lives at 39, and it
went unnoticed for a slice.

The file is already vendored at `red/state/testdata/event_constants.asm`, and
`red/state/event_constants_test.go` replays the RGBDS counter over it properly.
Take an index from there, or from `state.Event`. Never from a line number.

## Grepping it without drowning

The decomp is ~1,500 files, and dumping one into a worker's context is how
several attempts here have died. Scope the search:

    grep -rn EVENT_GOT_POKEDEX pokered/scripts
    sed -n '1,80p' pokered/scripts/ViridianCity.asm   # a slice, not the file

`pokered/engine/` is the game's own logic (battle, overworld, menus). It answers
"how does the game decide X", which is rarely the question — the map, script,
and data files above almost always are.
