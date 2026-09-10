# Field-move roster recovery

Issue #107 closes the gap between owning an HM/badge and actually carrying a
party that can use the move.

`skill.RepairFieldCapabilities` enforces a required field-move set in three
steps:

1. teach the HM to a compatible member already in the party;
2. otherwise withdraw a compatible Pokémon from the active Bill's PC box,
   depositing a safe party member first when the party is full;
3. otherwise choose a compatible species from ROM wild-encounter tables,
   travel to a known reachable grass map, catch it, and teach the HM.

Roster changes are planned against the *whole* required set. A full-party
swap is rejected if the hypothetical post-swap party cannot still satisfy all
required moves together. For late-game story progression the core invariant is
Cut + Surf + Strength; `RepairOwnedCoreFieldCapabilities` applies the invariant
to whichever of those moves the save has actually unlocked.

There is no hard-coded starter or "HM slave" species table. TM/HM compatibility
comes from the loaded ROM and wild candidates come from `WildGrass`.

Bill's PC primitives (`DepositPartyMon` and `WithdrawBoxMon`) use the hidden PC
at tile (13,3) of ordinary Pokémon Centers and verify party/box deltas from RAM.
They intentionally operate only on the active box. Full-box switching remains
shared follow-up work with #42; a full active box fails explicitly rather than
releasing or overwriting a Pokémon.
