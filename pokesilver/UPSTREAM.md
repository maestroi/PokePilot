# Vendored pokesilver

Pokémon Silver is built from the same pret/pokegold tree as Gold. At
`gs/data.SourceRevision` (`0f087a51e36cbd38f33e5055754614578246ceff`) there is
no separate pret/pokesilver repository: version differences are `_GOLD` /
`_SILVER` ifdefs inside `pokegold/`.

Read `pokegold/` and `docs/POKEGOLD.md`. Silver-only facts (version-exclusive
wild slots, a handful of items) are the `_SILVER` halves of those same files.

The supported USA/Europe rev0 Silver ROM is sha1
`49b163f7e57702bc939d642a18f591de55d92dae`.
