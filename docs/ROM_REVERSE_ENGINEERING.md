# ROM reverse-engineering workflow

`romprobe` is the PokePilot-side workflow for discovering RAM symbols while onboarding a new Pokémon ROM or revision. It builds on GomeBoy's side-effect-free `Peek*`/snapshot API and deterministic `memprobe` experiments; Pokémon meaning stays in PokePilot as labels such as `player.x` or `battle.mode`.

The tool never writes to emulated memory while capturing a snapshot. Snapshot files are JSON and include the exact ROM SHA-256, title/model, frame/cycle, active WRAM/VRAM bank registers, and base64-encoded named memory regions. The defaults are WRAM (`C000-DFFF`) and HRAM (`FF80-FFFE`).

## Commands

Build or run it with Go:

```sh
go run ./cmd/romprobe help
```

Capture a named checkpoint from a ROM, optionally after restoring a raw or checked GomeBoy save state:

```sh
go run ./cmd/romprobe capture \
  -rom roms/game.gb \
  -state investigation.state \
  -name baseline \
  -out baseline.json
```

Diff two captures. `-region`, `-start`, `-end`, `-before`, `-after`, and `-delta` can be combined:

```sh
go run ./cmd/romprobe diff \
  -region wram -start 0xD000 -end 0xDFFF -delta 1 \
  baseline.json moved.json
```

Use `-json` for machine-readable results. Each changed byte includes region, address, old/new value, signed delta, and bank where the address is banked.

Compare repeated transitions by passing snapshot pairs. Candidate addresses are ranked by how often they changed; addresses with a stable delta across repeated experiments receive a small consistency bonus. A no-input pair can be subtracted as ordinary time-driven noise:

```sh
go run ./cmd/romprobe compare \
  -control-pair 1 \
  baseline.json waited.json \
  baseline.json moved-right.json \
  baseline.json moved-down.json
```

For input-driven investigations, `experiment` is usually faster. Every action starts from the same emulator checkpoint, so the baseline is identical and the emulator is restored when the experiment completes. `control` is automatically used as a noise set:

```sh
go run ./cmd/romprobe experiment \
  -rom roms/game.gb \
  -state overworld.state \
  -actions control,right,down \
  -hold 1 -settle 20
```

`-repeat N` repeats non-control actions from the same baseline. It is useful for checking that an address/delta is stable, not for sampling unrelated RNG trajectories: the baseline checkpoint is deterministic.

## Labels and profile symbols

Once a candidate is understood, attach a semantic label with `-label`. Labels are validated against the current candidate set and appear in JSON/text output. `-symbols` additionally saves them as a small profile-friendly JSON file:

```sh
go run ./cmd/romprobe experiment \
  -rom roms/game.gb \
  -state overworld.state \
  -actions control,right,down \
  -hold 1 -settle 20 \
  -label 'wram@0xD362=player.x:overworld tile X coordinate' \
  -label 'wram@0xD361=player.y:overworld tile Y coordinate' \
  -symbols coordinates.symbols.json
```

Label syntax is `[REGION@]ADDRESS=NAME[:NOTE]`. Naming the region is recommended, and is required if overlapping captured regions make an address ambiguous. The exported format is intentionally data-only rather than generated Go, so the future `GameProfile` work can consume it without coupling this tool to an interface that has not landed yet.

Example symbol output:

```json
{
  "schema_version": 1,
  "rom": {
    "title": "POKEMON RED",
    "sha256": "...",
    "model": "..."
  },
  "symbols": [
    {
      "name": "player.x",
      "region": "wram",
      "address": 54114,
      "address_hex": "0xD362",
      "bank": 1,
      "note": "overworld tile X coordinate"
    }
  ]
}
```

Do not copy symbols between revisions merely because the title matches. The SHA-256 in every capture/symbol file is the revision boundary.

## Worked Pokémon Red example

Pokémon Red is the currently supported reference game, so it is useful for validating the discovery workflow without teaching the tool any Red-specific addresses.

Start from a checked/raw state where the player is controllable and both the tile to the right and the tile below are walkable. Run:

```sh
go run ./cmd/romprobe experiment \
  -rom "$POKEMON_RED_ROM" \
  -state /path/to/overworld.state \
  -actions control,right,down \
  -hold 1 -settle 20 \
  -delta 1 \
  -json > red-coordinates.json
```

The control action removes bytes that change simply because frames elapsed. Among the remaining `+1` candidates, the right transition isolates the X-coordinate byte and the down transition isolates the Y-coordinate byte. On the supported Red revision these resolve to `0xD362` (`wXCoord`) and `0xD361` (`wYCoord`), matching the vendored `pokered` decomposition and `docs/DESIGN.md`. Confirm them and export:

```sh
go run ./cmd/romprobe experiment \
  -rom "$POKEMON_RED_ROM" \
  -state /path/to/overworld.state \
  -actions control,right,down \
  -hold 1 -settle 20 -delta 1 \
  -label 'wram@0xD362=player.x:confirmed against wXCoord' \
  -label 'wram@0xD361=player.y:confirmed against wYCoord' \
  -symbols pokemon-red.coordinates.json
```

The same process scales to less obvious state:

1. take/restore a baseline before the behavior of interest;
2. add a no-input control when time-driven bytes are noisy;
3. change one thing at a time (direction, menu count, battle entry/exit, item quantity);
4. narrow with region/range/value/delta filters;
5. repeat or compare independent transitions and rank candidates;
6. label only after confirming the address by another experiment or the game's decomp/disassembly;
7. keep the exported symbol file beside the profile work as reviewable evidence.

For battle/party/bag structures, prefer multiple small transitions over one large "before game / after game" diff. A byte that tracks the intended semantic change and stays quiet in controls is much stronger evidence than a byte that merely changed once.

## Bank notes

The snapshot records raw `SVBK` (`FF70`) and `VBK` (`FF4F`) plus their effective CGB bank values. Diff rows report bank 0 for fixed WRAM/HRAM and the captured active bank for `D000-DFFF`/VRAM. On DMG-only titles such as Red, this context is mostly diagnostic; on GBC profiles it must be treated as part of an address's identity.

`romprobe` deliberately does not infer Pokémon semantics from addresses. GomeBoy owns safe observation and emulator-level bank behavior; this layer owns investigation, ranking, labels, and profile-oriented export.
