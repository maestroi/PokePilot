# Gen 1 combat scoring boundary

Issue: #49

This slice makes one reusable combat model the source of truth for ordinary damaging-move ordering.

## Included

- ROM-derived move power, type, accuracy and max PP
- live current PP and disabled-move filtering from battle state
- Gen 1 type effectiveness and immunity from the ROM TypeEffects table
- STAB
- Gen 1 type-based physical/special split (`type < 0x14` is physical)
- live current Attack/Defense/Special values used by Red's own damage routine
- integer approximation of Red's ordinary damage formula for deterministic ordering
- accuracy-weighted expected-turn score
- PP as a tie-break only, so resource conservation cannot override materially better expected damage
- reusable `red/combat.Combatant` / `EvaluateMove` API for later party switching and move-teaching work
- compact ZBAT explanation of the chosen move's factors

## Intentionally not invented

The scorer does not pretend every Gen 1 move is an ordinary damage-formula move. Critical-hit probability, fixed-damage moves, multi-hit distributions, Counter/Bide, Reflect/Light Screen and richer status-move utility remain explicit mechanics to add when a policy needs them. Zero-power moves receive no fake damage score; the existing bounded Tail Whip setup rule remains separate.

This keeps #49's shared matchup layer factual and reusable while avoiding a false "exact simulator" claim.
