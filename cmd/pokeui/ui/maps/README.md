# Semantic map assets

This directory is embedded into `pokeui` and is populated from a local Pokemon Red ROM.
The ROM itself is never committed or served.

Generate assets with:

```sh
POKEMON_RED_ROM=roms/pokemon_red.gb go run ./cmd/mapassets -o cmd/pokeui/ui/maps
```

Each `{id}.json` file contains the `world.Build` walkability grid plus warp and connection metadata used by the live watch map.
