# Issue 107 validation

ROM-free tests cover the failure mode and roster-planning invariants:

- owned/unlocked Surf with no compatible party member is recognized as not
  preparable by the current roster;
- full-party replacement never chooses the only current Cut user when that
  would strand the required set;
- a hypothetical post-swap party is required to satisfy Cut + Surf + Strength
  together;
- active-box selection prefers a candidate that covers more required field
  moves rather than a hard-coded species.

`TestBillsPCRoundTrip` is opt-in ROM-backed coverage for the gameplay roster
primitive: catch a partner, deposit it through the Pokémon Center PC, withdraw
it again, and verify party/box RAM deltas and species identity.

A full six-member prepared-state swap plus field use remains suitable for the
private qualification suite because public CI deliberately has no commercial
ROM/save fixture. The production primitive used for that path is the same
`DepositPartyMon` / `WithdrawBoxMon` / `EnsureFieldMove` sequence.
