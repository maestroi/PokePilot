# Player progress in the live UI

Approved 2026-09-14. Operator and spectator already show money, badges, and
party. Bag space, Pokédex counts, and major story milestones are already
decoded from RAM on every heartbeat; this work puts them on the same
`farm.Player` snapshot and renders them.

This is telemetry and display. It does not change planner, skill, or recovery
policy. `docs/ARCHITECTURE.md` still applies: generic layers carry portable
counts and labels; Red item bytes, event bits, and bag/Dex limits stay in the
adapter.

## Goal

An operator or spectator can see bag fill, Dex completion, and major story
beats without opening the in-game menus.

## Approach

Extend the existing live player snapshot (option B). Lists ride the same
dashboard poll that already carries party. Spectator may show the same game
state; there is no extra secret in bag names or milestone labels.

## Data contract

`farm.Player` gains portable fields only:

| Field | Meaning |
|---|---|
| `bag_used` / `bag_capacity` | Occupied slots vs adapter capacity |
| `bag[]` | `{name, quantity}` semantic item names |
| `dex_owned` / `dex_seen` / `dex_total` | Owned vs seen vs National Dex size |
| `milestones[]` | Completed major story labels, story order |

Missing fields (older runners) hide the new meters. They must not render as
`0/0`. New runners always send capacity and Dex total.

Item names use the existing adapter name table, same pattern as party
species. Unknown IDs become `item 0xNN`. Capacity is Red's 20-slot bag.
Dex total is 151. Both values are supplied by the adapter, not hardcoded in
Vue.

## Milestones

Completed only. Badges stay on their own row.

Pokédex, Mt. Moon fossil, S.S. Ticket, HM01, Silph Scope, Poké Flute, Card
Key, Silph rescue, Secret Key, Champion, Hall of Fame.

## Surfaces

**Operator Live, Game state.** Header: `₽1,840 · bag 14/20 · dex 12/151 ·
Boulder`. Under the party grid, three always-visible rows for bag items,
Dex owned/seen, and completed milestones. Empty bag or no milestones get a
one-line empty state.

**Spectator.** The 4-up strip becomes 6-up (Badges, Party, Bag, Dex, Money,
Speed). The Party panel footer keeps badge chips and adds the same three
inline rows. The activity feed notes a new owned Dex count or a newly
completed milestone.

## Tests

- `playerSnapshot` names bag items, reports used/capacity, Dex counts, and
  milestone labels; unknown items stay displayable; empty bag still sends
  capacity.
- Spectator public JSON includes the new player fields and still strips
  private run metadata.
- Shared meter helpers hide when capacity/total is absent.
