# Issue #111 — Elite Four sequence resources

Pokémon Red's Elite Four is a five-battle no-Center sequence. Local battle
medicine is not enough by itself: an unattended run must decide what to repair
between fights without consuming every finite resource after Lorelei.

## Model

`skill.PlanLeagueResources` observes:

- every party member's live/fainted state
- HP and status
- current move PP
- ROM-derived base maximum PP for each known move
- current bag quantities
- how many chained encounters remain

The policy is deliberately party-agnostic. It does not require a canonical
starter, six-member team, or exact shopping list. A configurable viability
floor says how many party members should remain live, the minimum HP fraction,
and the minimum aggregate PP fraction.

## Free healing before entry

When `FreeCenterAvailable` is true, any HP/status/PP deficit produces exactly
one `LeagueUseCenter` action. No finite medicine, Revive, Ether, or Elixer is
allocated while a free Center recovery is available. This is the intended
Indigo Plateau pre-entry behavior.

## Fair-share reservation

Between battles, each resource category gets at most:

```
ceil(current stock / encounters remaining)
```

uses in that preparation window. For example, four Ethers with four fights
left permit one Ether now and reserve three for later. The planner reports the
projected remaining quantities in `LeagueResourcePlan.Reserved` so the
allocation is explainable and testable.

The planner spends only enough to restore the configured viability floor:

1. revive deterministic high-value fainted members until the live-party floor
   is reached
2. clear status where a matching cure exists
3. heal the lowest-HP live members to the HP floor
4. restore PP to the lowest-PP live member until aggregate PP reaches the floor

Regular Revive is preferred over Max Revive. Specific status cures are
preferred over Full Heal/Full Restore. HP medicine uses the existing
smallest-sufficient healing order. Ether/Elixer selection prefers the ordinary
variant when it can address the observed deficit.

`PrepareLeagueResources` executes one such bounded plan and then re-observes.
It intentionally does **not** repeatedly request a new fair share in the same
between-battle window, because doing so would defeat the reserve invariant.
Failure to reach the configured floor returns `ErrLeagueResourcesInsufficient`
with the successful actions and final assessment available to the caller.

## Verified item actions

`RevivePartyMember` and `RestorePartyPP` reuse `UseFieldItem`, so normal game
menus are used. Success requires both:

- the target RAM state changed positively (revived HP / increased PP)
- the selected bag quantity fell by exactly one

Max Revive additionally verifies that the revived member reaches full HP.
No party or inventory RAM is mutated directly.

## Sequence completion

`LeagueSequenceProgress` may record intermediate wins for resume/diagnostics,
but `Complete()` is false until all five fights have been won **and** the
observable `EventBeatChampionRival` flag is present. A `Battle()` win against
Lorelei, Bruno, Agatha, or Lance can therefore never mark the overall League
objective complete.

## Validation

ROM-free tests cover:

- Center-before-consumables behavior
- fair-share HP/PP reservation
- bounded Revive allocation
- insufficient-resource diagnostics
- no early sequence completion
- revival as a positive field-item effect

`TestLeagueResourceItemsRealROM` is an opt-in prepared-state qualification for
real Revive/Max Revive and Ether/Elixer menu execution. Set
`POKEPILOT_LEAGUE_RESOURCE_TEST_STATE` in local/private ROM-backed CI. The
external state must be controllable, include one fainted member plus a revive,
and include one live member with a PP deficit plus a PP-restoration item.

The full Indigo Plateau -> Champion room/battle journey belongs to progression
issue #38; #111 provides the reusable resource and checkpoint layer that flow
can call before entry and between each leader.
