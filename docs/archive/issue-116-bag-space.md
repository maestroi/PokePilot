# Safe bag-space recovery

Pokémon Red's bag stores at most 20 distinct item stacks. A story reward that
uses `GiveItem` fails when all 20 slots are occupied and the awarded item does
not already have a stack.

`skill.EnsureBagSpaceFor` and `skill.EnsureBagFreeSlots` make that failure
recoverable without guessing which inventory is expendable:

- the candidate set is an explicit whitelist of replenishable consumables;
- the whole stack is discarded, because partial tosses do not create a slot;
- candidates are ranked by replacement cost of the whole stack;
- Master Ball, key/story items, TMs/HMs, evolution items, rare/scarce resources
  and unknown IDs are protected by omission;
- the real START -> ITEM -> TOSS flow is driven through live menu state;
- success is accepted only after RAM proves the stack disappeared, the player
  is controllable again, and a bag slot is free.

Generic ground-item pickup preflights capacity immediately before interacting
with the item ball. Fuchsia progression additionally preflights HM03 and the
resumed-Warden HM04 case. When Gold Teeth are still in the bag, the Warden
removes them before awarding HM04, so no needless sacrifice is made.
