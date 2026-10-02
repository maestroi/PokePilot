# Vendored pokegold

The Pokémon Gold/Silver disassembly, from https://github.com/pret/pokegold at
commit `0f087a51e36cbd38f33e5055754614578246ceff`. This is the source of every
Gold/Silver fact this project labels DERIVED, and it is the same revision
`gs/data.SourceRevision` was generated from.

It lives in the repository rather than behind a symlink because agent runs
happen in git worktrees, and a gitignored symlink is absent from every one of
them. See `docs/POKEGOLD.md` for the question -> file map.

pret/pokegold is one tree that builds both Gold and Silver (`_GOLD` / `_SILVER`
ifdefs). There is no separate pret/pokesilver checkout at this revision;
`pokesilver/UPSTREAM.md` records that.

## What is here, and what is not

Vendored: `constants/`, `data/` (plain text), `maps/*.asm`, `ram/`, `macros/`,
`home/`, and `engine/`. 1741 files.

Not vendored: `gfx/` and `audio/` (binary assets), `maps/*.blk` (block layouts;
this project parses those out of the running ROM), build output (`*.o`,
`pokegold.gbc`, `pokesilver.gbc`, `*.sym`, `*.map`), `tools/`, `vc/`, and the
build machinery. **The built ROM is deliberately excluded** — this project
never commits a `.gb`/`.gbc`/`.sav`/`.state`.

Table addresses are located from ROM structure rather than a committed `.sym`,
so a worktree does not need `rgbasm` to answer "where is MapGroupPointers".

Nothing here is compiled or executed. It is read as reference. The supported
USA/Europe rev0 ROMs are byte-identical to what this tree builds:

- Gold sha1 `d8b8a3600a465308c9953dfa04f0081c05bdcb94`
- Silver sha1 `49b163f7e57702bc939d642a18f591de55d92dae`

## Do not edit

Treat it as read-only upstream. To move to a newer pokegold, re-copy those
directories from a fresh checkout and update the commit above — do not patch
files in place, or the tree stops describing the ROM.

`docs/POKEGOLD.md` maps question -> file.
