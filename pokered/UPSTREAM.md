# Vendored pokered

The Pokemon Red disassembly, from https://github.com/pret/pokered at commit
`0cd19d3`. This is the source of every fact this project labels DERIVED.

It lives in the repository rather than behind a symlink because agent runs
happen in git worktrees, and a gitignored symlink is absent from every one of
them. Several runs were spent rediscovering that `scripts/*.asm` was not a path
they could open. It is now.

## What is here, and what is not

Vendored: `constants/`, `data/`, `scripts/`, `text/`, `ram/`, `macros/`,
`home/`, `engine/`, and `pokered.sym`. Plain text only, 1521 files.

Not vendored: `gfx/` and `audio/` (7.4M and 1.9M of binary assets), build
output (`*.o`, `pokered.gbc`), and `tools/`. **The built ROM is deliberately
excluded** — this project never commits a `.gb`/`.gbc`/`.sav`/`.state`, and a
disassembly that ships its own ROM would break that rule quietly.

The web World Explorer is the exception at **build time**, not in git:
`web/scripts/sync-gen1-render-assets.mjs` downloads the same pinned upstream
commit and extracts only `maps/*.blk`, `gfx/blocksets/*.bst`, and
`gfx/tilesets/*.png` into the gitignored `web/public/gen1/` directory. Those
files are static render inputs for the explorable map; they are never used to
build or bundle a ROM. Set `POKEPILOT_SKIP_GEN1_TEXTURES=1` for an offline
frontend build, which falls back to the semantic/procedural map renderer.

Nothing here is compiled or executed. It is read as reference, and the ROM at
`roms/pokemon_red.gb` is byte-identical to what this tree builds
(sha1 `ea9bcae617fdf159b045185467ae58b2e4a48b9a`), so it describes the exact
bytes the emulator runs.

## Do not edit

Treat it as read-only upstream. To move to a newer pokered, re-copy those
directories from a fresh checkout and update the commit above — do not patch
files in place, or the tree stops describing the ROM.

`docs/POKERED.md` maps question -> file.
