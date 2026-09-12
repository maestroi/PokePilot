# Game profiles

PokePilot separates portable planning/runtime concepts from ROM- and revision-specific knowledge through `game.GameProfile`.

A profile represents one exact supported game revision. It owns the semantic RAM symbol table, ROM detection rule, baseline observation decoder, ROM-name/species parser and optional feature set. Generic code consumes semantic values such as `player.x`, `party.count`, `battle.mode`, `badges` and `money`; it should not learn a concrete game's WRAM addresses.

## Runtime selection

`profiles.Detect` fingerprints the complete ROM and asks every registered profile whether it owns that identity. Selection has three deliberate properties:

- revisions are matched by an exact ROM fingerprint rather than title alone;
- zero matches return an `UnsupportedROMError` containing title, SHA-1, SHA-256, version and ROM size;
- multiple matches are rejected as ambiguous rather than relying on registration order.

Pokémon Red is the first built-in profile:

- profile id: `pokemon-red`
- revision: `en-us-rev0`
- ROM title: `POKEMON RED`
- SHA-1: `ea9bcae617fdf159b045185467ae58b2e4a48b9a`

That hash is already the repository's canonical supported Red ROM identity. A different revision with the same title is intentionally unsupported until it has its own tested profile.

## Contract

The core contract lives in `game/profile.go`:

```go
type GameProfile interface {
    ID() GameID
    Revision() RevisionID
    Detect(ROMInfo) bool
    Symbols() SymbolTable
    Features() ProfileFeatures
    ROMParser() ROMParser
    DecodeObservation(MemoryReader, []byte) (ProfileObservation, error)
}
```

`MemoryReader` is side-effect-free and is satisfied by `emu.Emu`. A profile therefore observes memory without depending on controller input or mutating emulation.

Every profile must expose the baseline semantic symbols checked by `game.ValidateProfileContract`:

- `player.map`
- `player.x`
- `player.y`
- `player.direction`
- `party.count`
- `party.members`
- `battle.mode`
- `badges`
- `bag`
- `money`

Profiles may add more symbols such as `respawn.map` and `story.events`. Banked games should include the bank in `MemorySymbol` and advertise `FeatureBankedMemory`.

`ProfileObservation` is deliberately semantic. Species use `game.SpeciesID`, locations use `game.PlaceID`, and story state uses `game.ProgressState`. `NativeMapID` exists as a runtime-only migration handle and is omitted from planner JSON.

## Feature flags

Optional behavior is represented by `ProfileFeatures`, not game-name branches. Current feature ids include map parsing, inventory, story progress, battles, field moves, trainer flags, banked memory and semantic species mapping.

A game that lacks a mechanic simply leaves the feature disabled. Generic code should ask for the capability it needs and return an actionable unsupported-capability error instead of checking `profile.ID()`.

## Pokémon Red profile

`red/profile` is the first concrete adapter. It owns:

- the exact Red revision fingerprint;
- semantic-to-WRAM symbol mapping;
- map/species parsing;
- normal player, party, badge, event, inventory and story observation;
- Red progression projection into semantic `ProgressID` values.

Raw Red species/item byte mappings live in `red/data` so profile decoding and Red-owned objective execution share one source of truth.

The existing battle, route, map-object and field-move controllers are still Red-owned code. `agent.ObserveChecked` first obtains the portable baseline from the selected profile, then adds those Red execution enrichments. This preserves current Red behavior while establishing the profile boundary that subsequent controllers can migrate behind feature-specific interfaces.

## Adding another game or revision

Use this checklist for every new profile. A pull request adding a profile should be reviewable without changing generic planner policy.

1. **Fingerprint the exact ROM.** Record title, SHA-1/SHA-256 and relevant header metadata. Never accept a revision by title alone.
2. **Discover memory semantics.** Use `cmd/romprobe` from `docs/ROM_REVERSE_ENGINEERING.md` to capture/diff repeated transitions and export labelled symbols. Keep the symbol evidence tied to the ROM hash.
3. **Create a game-owned data package.** Put raw species/item/map encodings and other revision vocabulary under that game's package, not `agent` or `game`.
4. **Implement `GameProfile`.** Give it stable `ID`/`Revision` values, exact `Detect`, semantic `Symbols`, `ROMParser`, `Features` and `DecodeObservation`.
5. **Implement required baseline symbols.** `game.ValidateProfileContract` must pass. For GBC/banked layouts, include bank identity and enable `FeatureBankedMemory`.
6. **Decode semantic observations.** Return semantic location/species/progress values. Do not expose raw WRAM addresses or raw species/item bytes to planner-facing structs.
7. **Declare optional capabilities honestly.** Do not advertise battles, field moves, map parsing, etc. until the profile has an implementation for them.
8. **Add contract tests.** Call `game.ValidateProfileContract(profile)` and test the profile's known fingerprint plus rejection of an unknown revision with the same title.
9. **Add observation fixtures.** At minimum cover position/direction, party, badge/progress and one game-specific edge case. Prefer deterministic save-state fixtures when available.
10. **Register it once.** Add the profile to `profiles.Builtin`; do not add game-name conditionals throughout generic runtime code.
11. **Run qualification.** Exercise the reusable short tests and, where the ROM is available, a representative `pokequal`/run checkpoint through the selected profile.
12. **Keep revision differences explicit.** If another revision moves even one relevant structure, add a separate revision profile or a clearly tested revision-owned layout object rather than weakening detection.

A minimal shape is:

```go
type Profile struct{}

func (*Profile) ID() game.GameID              { return "pokemon-yellow" }
func (*Profile) Revision() game.RevisionID     { return "en-us-rev0" }
func (*Profile) Detect(info game.ROMInfo) bool { return info.SHA1 == knownSHA1 }
func (*Profile) Symbols() game.SymbolTable     { /* semantic addresses */ }
func (*Profile) Features() game.ProfileFeatures { /* implemented capabilities */ }
func (*Profile) ROMParser() game.ROMParser     { return parser{} }
func (*Profile) DecodeObservation(mem game.MemoryReader, rom []byte) (game.ProfileObservation, error) {
    // decode this revision into semantic state
}
```

Yellow is a useful second Gen I validation because much of the architecture is similar while layout/mechanics differ. Crystal is a stronger later validation because its GBC banking forces bank identity and optional capability handling to be correct.

## Relationship to `romprobe`

Issue #55's reverse-engineering tools discover and verify symbols; `GameProfile` is where confirmed symbols become runtime behavior. The handoff is intentionally data-driven:

1. `romprobe experiment`/`compare` identifies candidates;
2. labels export semantic names with the ROM fingerprint;
3. the profile maps those confirmed semantics into its `SymbolTable` and decoder;
4. profile tests make the mapping executable and regression-safe.

This keeps emulator introspection game-agnostic, profile knowledge game-owned, and planner behavior semantic.
