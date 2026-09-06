# Issue 107 validation

ROM-free tests cover the failure mode and roster-planning invariants:

- owned/unlocked Surf with no compatible party member is recognized as not
  preparable by the current roster;
- field-move coverage uses a bounded assignment search, so a flexible mon can
  be reserved for Surf while a more constrained mon takes Cut;
- full-party replacement never chooses the only current Cut user when that
  would strand the required set;
- a hypothetical post-swap party is required to satisfy Cut + Surf + Strength
  together;
- active-box selection prefers a candidate that covers more required field
  moves rather than a hard-coded species.

`TestBillsPCRoundTrip` is opt-in ROM-backed coverage for the gameplay roster
primitive: catch a partner, deposit it through the Pokémon Center PC, withdraw
it again, and verify party/box RAM deltas and species identity.

`TestRepairFieldCapabilitiesFullPartySurfRealROM` is the full #107 qualification
path. Set `POKEPILOT_FIELD_ROSTER_TEST_STATE` to an external real-ROM state with
six party members, HM03 + Soul Badge, no current Surf-compatible member, a
compatible mon in the non-full active Bill's PC box, and surfable water directly
north of the player's starting tile. The test performs the full deposit /
withdraw / teach repair, verifies the party changed and Surf became usable,
returns to the prepared shoreline, and executes Surf successfully from the real
game state.

The prepared state is intentionally not committed because public CI has no
commercial ROM/save fixture. Public CI still compiles the qualification path;
local/private qualification supplies the external state.
