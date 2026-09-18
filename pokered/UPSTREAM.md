# Vendored pokered

The Pokemon Red disassembly, from https://github.com/pret/pokered at commit
`0cd19d3`. This is the source of every fact this project labels DERIVED.

It lives in the repository rather than behind a symlink because agent runs
happen in git worktrees, and a gitignored symlink is absent from every one of
them. Several runs were spent rediscovering that `scripts/*.asm` was not a path
they could open. It is now.

## What is here, and what is not

Vendored: `constants/`, `data/`, `scripts/`, `text/`, `ram/`, `macros/`,
`home/`, `engine/`, `gfx/`, and `pokered.sym`. 2233 files, including the 711
source graphics under `gfx/` (png, bst, tilemap, rle, and the small assembler
tables that index them).

Not vendored: `audio/` (binary assets), build output (`*.o`, `pokered.gbc`),
and `tools/`. Compiled graphics (`*.1bpp`, `*.2bpp`, `*.pic`) are also left
out — they are Makefile products, not source. **The built ROM is deliberately
excluded** — this project never commits a `.gb`/`.gbc`/`.sav`/`.state`, and a
disassembly that ships its own ROM would break that rule quietly.

The web World Explorer still needs `maps/*.blk` at **build time**, which is
not vendored here: `web/scripts/sync-gen1-render-assets.mjs` downloads the
same pinned upstream commit and extracts `maps/*.blk` plus the already-vendored
`gfx/blocksets/*.bst` and `gfx/tilesets/*.png` into the gitignored
`web/public/gen1/` directory. Those files are static render inputs for the
explorable map; they are never used to build or bundle a ROM. Set
`POKEPILOT_SKIP_GEN1_TEXTURES=1` for an offline frontend build, which falls
back to the semantic/procedural map renderer.

Nothing here is compiled or executed. It is read as reference, and the ROM at
`roms/pokemon_red.gb` is byte-identical to what this tree builds
(sha1 `ea9bcae617fdf159b045185467ae58b2e4a48b9a`), so it describes the exact
bytes the emulator runs.

## Do not edit

Treat it as read-only upstream. To move to a newer pokered, re-copy those
directories from a fresh checkout and update the commit above — do not patch
files in place, or the tree stops describing the ROM.

`docs/POKERED.md` maps question -> file.
