# Vendored pokeyellow

The Pokemon Yellow disassembly, from https://github.com/pret/pokeyellow. This
is the source of every Yellow fact this project labels DERIVED.

It lives in the repository rather than behind a symlink because agent runs
happen in git worktrees, and a gitignored symlink is absent from every one of
them. See `docs/POKEYELLOW.md` for the question -> file map.

The copy we were given carried no `.git`, so the upstream commit is unknown.
What is known is stronger than a commit: **this tree builds a ROM that is
byte-identical to the one the emulator runs**

    sha1 cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1

which matches `roms/pokemon_yellow.gb` and the `roms.sha1` the upstream project
ships for `pokeyellow.gbc`. So every file below describes the exact bytes the
emulator is running. If you move to a newer pokeyellow, re-copy, rebuild, and
confirm that hash before trusting the new tree.

## What is here, and what is not

Vendored: `constants/`, `data/`, `scripts/`, `text/`, `ram/`, `macros/`,
`home/`, `engine/`, and `pokeyellow.sym`. Plain text only, 1600 files.

Not vendored: `gfx/` and `audio/` (binary assets), build output (`*.o`,
`pokeyellow.gbc`, `pokeyellow.map`), `maps/` (the `.blk` block layouts, which
this project parses out of the running ROM instead), `tools/`, `vc/`, and the
build machinery (`Makefile`, `layout.link`, the top-level `*.asm` include
drivers). **The built ROM is deliberately excluded** — this project never
commits a `.gb`/`.gbc`/`.sav`/`.state`, and a disassembly that ships its own
ROM would break that rule quietly.

`pokeyellow.sym` is generated, not upstream source. Regenerate it with
`rgbasm`/`rgblink` (v1.0.3 or compatible) from a full fresh checkout:

    make pokeyellow.gbc   # produces pokeyellow.sym and pokeyellow.map

then keep `pokeyellow.sym` and delete everything else that the build made,
along with the directories above. It is committed because it is the answer to
"what is the exact address of label X", and regenerating it is not always
possible in a worktree.

Nothing here is compiled or executed. It is read as reference.

## Do not edit

Treat it as read-only upstream. To move to a newer pokeyellow, re-copy those
directories from a fresh checkout, rebuild, verify the sha1 above, and update
this note — do not patch files in place, or the tree stops describing the ROM.
