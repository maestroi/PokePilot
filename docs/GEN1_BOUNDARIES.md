# Generation I portability boundaries

Issue #1041 starts extracting mechanics that are genuinely shared by Red, Blue,
and Yellow without moving cartridge data into generic packages.

## Shared now

- **Fresh-game boot policy** lives in `skill/boot.go`. It consumes
  `game.BootState` only: ready/control state, semantic menu position, live
  preset names, and final player/rival names.
- **English Gen-I text/name encoding** lives in `gen1/`. Profiles provide the
  bytes and own the addresses.
- **ROM identity and RAM layout** remain profile-owned. Red/Blue and Yellow may
  implement the same semantic contract with different addresses.
- **Ready map identity** remains profile-owned. The shared boot driver never
  checks a native map id.

## Red/Blue sharing versus Yellow

Blue currently delegates most of its profile implementation to Red because the
supported English Red/Blue revisions use the same runtime layout. Yellow does
not join that delegation: its WRAM layout differs and its later story/mechanics
need their own adapters.

The `gen1` package is deliberately smaller than a "base game" class. It may
contain only formats/mechanics demonstrated to be shared. It must not contain
ROM offsets, RAM addresses, native map ids, event/script ids, encounter tables,
or version-specific species/item data.

## Remaining Red-owned dependencies

The remaining `red/*` imports under `skill/` fall into three groups:

1. **World/map execution** — routing, blockers, elevators, grass/water access,
   map objects, and other helpers coupled to Red ROM map parsing. Yellow needs
   its own world/parser provider first; tracked by #1042.
2. **Story/campaign scripts** — gyms, Mt. Moon, S.S. Anne, Silph, gifts, route
   gates, and other objectives whose event/script ids are version data. These
   should stay game-owned and are adapted during #1043/#1044.
3. **Battle/inventory mechanics** — some Gen-I behavior is probably shareable,
   but the current APIs still accept Red state/ROM structures. Extract those
   only when Yellow's battle/inventory decoders exist, rather than freezing Red
   byte layouts into a premature shared interface.

This is intentional: #1041 establishes semantic seams, while later Yellow
phases fill those seams with Yellow data. Gen II (#968) is free to provide
different implementations where its formats differ.
