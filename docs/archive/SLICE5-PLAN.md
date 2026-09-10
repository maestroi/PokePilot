# Slice 5 — the first badge: through Viridian Forest to Brock

Status: **in progress.** Plan `237bfde9-f006-4b4e-95e2-41d6dbb1204a`.

Goal: from a fresh boot, reach Pewter Gym and win the Boulder Badge
(`wObtainedBadges` 0xD356, bit 0).

---

## Two gates stand between Pallet Town and Pewter

Slice 2 stalled on the first one. Slice 5 stalled on the second, in exactly
the same way, and for the same reason: a story gate was discovered by walking
into it rather than by reading the scripts first.

### 1. Pallet Town north exit — `EVENT_FOLLOWED_OAK_INTO_LAB`

Closed by slice 3. `GetStarter` opens it.

### 2. Viridian City north exit — `EVENT_GOT_POKEDEX` (MEASURED 2026-08-26)

`scripts/ViridianCity.asm`, `ViridianCityCheckGotPokedexScript`:

```asm
	CheckEvent EVENT_GOT_POKEDEX
	ret nz          ; have it: no gate
	ld a, [wYCoord]
	cp 9
	ret nz
	ld a, [wXCoord]
	cp 19
	ret nz
	ld a, TEXT_VIRIDIANCITY_OLD_MAN_SLEEPY
	ldh [hTextID], a
	call DisplayTextID
	xor a
	ldh [hJoyHeld], a
	call ViridianCityMovePlayerDownScript
```

MEASURED by driving `Travel` from the `viridian_city` fixture to
`skill.Place("pewter city")`:

    Travel -> battles=0 err=Traverse: walk to edge on map 01:
              skill: text box interrupted movement
    map=01 pos=(19,9) frames=22159

(19,9) is the exact tile the script names. The sleepy old man displays a text
box and the script walks the player back south. It is not a pathfinding
failure and not an emulator defect.

**`EVENT_GOT_POKEDEX` comes from the Oak's Parcel errand**: collect the parcel
at Viridian Mart (map $2A), carry it back to Oak. `scripts/OaksLab.asm:1023`
then also gives 5 POKE_BALLs (`CheckAndSetEvent EVENT_GOT_POKEBALLS_FROM_OAK`,
`lb bc, POKE_BALL, 5`, `GiveItem`).

So the errand was moved out of slice 6 and into slice 5, ahead of the Pewter
milestone. Nothing north of Viridian is reachable without it.

---

## The routing repair (S5-3, landed)

The map graph knows which maps **touch**, not which are walkable between.

    FindRoute(0x01, 0x02) -> Viridian --conn--> Route 2 --conn--> Pewter

Route 2 (0x0D) is ONE map, 20x72, split across its full width: rows 22, 36 and
37 are blocked end to end, so no walkable column joins its south end to its
north. The real route leaves through the gate into Viridian Forest (0x33) and
re-enters Route 2 north of the barrier. The collision grid is CORRECT; this
was a routing defect.

Only the tile-level pathfinder can discover this, and only at execution time.
`Traverse` already reported it precisely; `GoTo` had nowhere to put it. Now
both unreachable-leg cases carry `ErrLegUnwalkable`, and `GoTo` bans that edge
and re-plans via `world.FindRouteAvoiding`. Nothing about Route 2 or the forest
is hardcoded — it is a property of the search.

---

## Process note — why S5-3 cost four attempts and no commits

    attempt 1  cancelled  peak_context 142358 / 150000
    attempt 2  stalled    "exceeded max runtime"     peak 118223
    attempt 3  repair     "no file progress"         peak  87806
    attempt 4  cancelled

The task text asked the worker to establish cross-map reachability, which
invites dumping collision grids — Route 2 is 20x72, Viridian Forest 34x48.
Context filled before any code was written. The fix, once diagnosed by hand,
is about twenty lines.

This is the third time this project has burned a runtime cap on a task whose
text pointed at an investigation rather than an edit. The rule that keeps
working: **measure by hand first, put the measurement in the task text, and
leave the worker an edit to make.** A task that says "find out why" is a task
that will spend the whole budget finding out.
