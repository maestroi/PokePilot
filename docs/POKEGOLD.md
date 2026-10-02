# The Gold/Silver decomp, and where things are in it

Every Gold/Silver fact this project calls DERIVED comes from the pret/pokegold
disassembly, **vendored in this repository at `pokegold/`** — so it is present
in a fresh clone and in every agent worktree. See `pokegold/UPSTREAM.md` for
provenance and for what was left out. Silver shares that tree; see
`pokesilver/UPSTREAM.md`.

Read it. Do not guess, and do not search the web: the supported USA/Europe rev0
ROMs are byte-identical to the pokegold build
(Gold sha1 `d8b8a3600a465308c9953dfa04f0081c05bdcb94`, Silver sha1
`49b163f7e57702bc939d642a18f591de55d92dae`), so every file below describes the
exact bytes the emulator is running.

Paths in the decomp's own documentation are written as `maps/Foo.asm`; here
they are `pokegold/maps/Foo.asm`. This is the same trap as `pokered/`: the
bare path opens nothing. Gold stores map scripts and events under `maps/`,
not `scripts/`.

## What you are probably looking for

| Question | File |
|---|---|
| What map is (group, number)? | `pokegold/constants/map_constants.asm` — `map_const NAME, w, h` |
| What does the game do when I stand here? | `pokegold/maps/<MapName>.asm` |
| Which NPCs are on this map, and where? | `pokegold/maps/<MapName>.asm` (`def_object_events`) |
| Where do this map's warps and connections go? | `pokegold/maps/<MapName>.asm` (warps), `pokegold/data/maps/attributes.asm` (connections) |
| Map header / second header layout | `pokegold/data/maps/maps.asm` (`MACRO map`), `pokegold/data/maps/attributes.asm`, `pokegold/macros/scripts/maps.asm` |
| What can I meet in this grass, by time of day? | `pokegold/data/wild/johto_grass.asm`, `kanto_grass.asm`, `*_water.asm` |
| When does it evolve, what does it learn? | `pokegold/data/pokemon/evos_attacks.asm` |
| What does a mart stock? | `pokegold/data/items/marts.asm` |
| Item prices / held-item flags | `pokegold/data/items/attributes.asm` |
| Move bytes (incl. Dark/Steel) | `pokegold/data/moves/moves.asm` |
| TM/HM list (HM06 Whirlpool, HM07 Waterfall) | `pokegold/data/moves/tmhm_moves.asm` |
| Character encoding | `pokegold/constants/charmap.asm` |
| What is at RAM address 0x____? | `pokegold/ram/wram.asm` |

`gs/rom` parses these tables out of the ROM image. Assertions that are a
function of ROM bytes belong in `gs/rom` tests, never in a journey test.
