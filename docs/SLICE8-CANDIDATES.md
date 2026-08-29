# Slice 8 candidates — raised 2026-08-29

Raised while slice 7 was still finishing (S7-8 running, S7-9 pending, nothing
merged to `main`). Recorded so the next planner does not re-derive the ROM
facts. Not yet a plan.

Every ROM citation below was read from `~/.cache/pokered` on 2026-08-29 and is
labelled DERIVED. Nothing here has been driven on the emulator; the items that
need MEASURING before they can be scoped say so explicitly.

---

## The proposed goal: the Cascade Badge

Slice 6's goal ("badge 1 with any starter") worked because it was the smallest
target that forced a whole family of mechanisms into existence — errand, shop,
bag, catching, party. **Earn the Cascade Badge** is the next such target. It
forces trainers, a multi-floor dungeon, item use outside battle, and a route
long enough that fleeing matters.

DERIVED — the destination exists and looks like Pewter Gym did:

    constants/map_constants.asm:109   map_const CERULEAN_GYM, 5, 7   ; $41
    data/maps/objects/CeruleanGym.asm
        object_event 4,  2, SPRITE_BRUNETTE_GIRL, ... OPP_MISTY, 1
        object_event 2,  3, SPRITE_COOLTRAINER_F, ... OPP_JR_TRAINER_F, 1
        object_event 8,  7, SPRITE_SWIMMER,       ... OPP_SWIMMER, 1
        object_event 7, 10, SPRITE_GYM_GUIDE,     ... TEXT_CERULEANGYM_GYM_GUIDE

Same shape as Pewter: a guide at (7,10) near the (4,13)/(5,13) entrance warps,
and **two** trainers before the leader. `skill.Gym` was written for Brock; it
will need to stop being Pewter-specific.

**As of today the goal is not even sayable.** `skill/goto.go:196-254` names 13
places and the last is `pewter gym`. `-goal "Earn the Cascade Badge."` gives
the planner a task it has no destination for.

---

## 0. The planner still cannot hear an NPC — `StepFrames` skips the hook

**This is now the top item, and it was MEASURED by S7-8, not guessed.** From
its RUNNOTES:

> `Emu.StepFrames` (emu/emu.go) batch-steps via `m.e.StepFrames(n)` and does
> NOT call onFrame; only single-frame `StepFrame` does. skill.Talk pages its
> whole conversation with `StepFrames(talkSettle)`, so a conversation Talk
> drives is invisible to every per-frame sampler — agent.Run's dialogue tape
> included.

So S7-7 offers the planner a `KindTalk` at the gym guide, `Execute` walks over
and talks to him, and **the reply never reaches `Observation.RecentDialogue`**.
The talk happens; the planner learns nothing. S7-8 only proved the guide's line
at all by driving its own talk loop, one `StepFrame` at a time, instead of
using `skill.Talk`.

Every NPC-derivation payoff slice 7 was built for is therefore still closed in
production. It also means: anything asserted from a per-frame sampler about a
Talk-driven conversation is asserting on frames that were never sampled — a
whole class of past readings is suspect, including the earlier "PrintText never
sets wFontLoaded" conclusion, which S7-8 identified as an artifact of exactly
this blindness.

Fix at the seam: make `StepFrames` step through `onFrame`, or give `Talk` an
observable pacing. Small, and it unblocks the thing slice 7 spent four tasks
building toward.

There is a second note attached: the vendored decomp shows only
`DisplayTextIDInit` setting `BIT_FONT_LOADED`
(`display_text_id_init.asm:34`), but the ROM empirically sets `wFontLoaded`
throughout PrintText boxes. A decomp/ROM discrepancy worth its own note before
anyone "simplifies" `DecodeDialogue`'s gate off it.

---

## 1. Trainers — and the ambush comes first

No trainer verb exists. S7-7 deliberately *reports* trainers in `MapObjects`
and does not offer them, because offering an objective with no verb behind it
manufactures a guaranteed failure every round. That was the right call and it
leaves the gap open.

**PART OF THIS IS NOW MEASURED.** A Gen 1 trainer engages by LINE OF SIGHT
while you walk past — `home/trainers.asm:123 CheckFightingMapTrainers` then
`TrainerWalkUpToPlayer_Bank0` (`home/trainers.asm:150`). S7-8 hit this for
real and its RUNNOTES record the mechanism precisely:

> PEWTERGYM_COOLTRAINER_M sits at (3,6) facing right (sight line x=4..8,
> engage distance 5), and his defeat flag is set ONLY by Brock's victory
> script (PewterGym.asm:79 .gymVictory) — his own end-battle text sets no
> event. Every crossing of row 6 on his sight line is a fresh two-Pokemon
> fight (Diglett L11 + Sandshrew L11).

That is the ambush, and it **re-arms on every crossing**. S7-8 routed around
him through the x=1 side corridor rather than paying the tax repeatedly. So
the answer to "does Travel survive an ambush" is: it survives one, but a
trainer whose flag never gets set is a repeating toll, and a small party
cannot pay it in, out for a heal, and back in.

The open question for slice 8 is narrower than I first wrote it: not "can we
survive a trainer" but **"can the router see a sight line?"** Route 3's seven
trainers cannot all be side-stepped the way one gym trainer was. Sight lines
are derivable — facing plus range, both already in `rom.Object` after S7-5.

DERIVED — Route 3 is not a route with some trainers on it, it is a corridor OF
trainers. `data/maps/objects/Route3.asm` has **eight** objects, seven of them
trainers, all `STAY` with a facing:

    object_event 10, 6, ... OPP_BUG_CATCHER, 4     object_event 14, 4, ... OPP_YOUNGSTER, 1
    object_event 16, 9, ... OPP_LASS, 1            object_event 19, 5, ... OPP_BUG_CATCHER, 5
    object_event 23, 4, ... OPP_LASS, 2            object_event 22, 9, ... OPP_YOUNGSTER, 2
    object_event 24, 6, ... OPP_BUG_CATCHER, 6     object_event 33, 10, ... OPP_LASS, 3

Plus the one plain NPC at (57,11). Mt. Moon 1F carries seven more
(`MtMoon1F.asm:30-36`), and Mt. Moon B2F is four Rockets and a Super Nerd
(`MtMoonB2F.asm:24-28`).

There is also `PEWTERGYM_COOLTRAINER_M` at (3,6) — already flagged in the slice
7 candidates, still unaccounted for by any test. S7-8 was told to REPORT it if
walking to the gym guide tripped his line of sight. S7-8's RUNNOTES now cover him in
full — see the quote above.

---

## 2. Item use outside battle

`skill.UseItem` (`skill/bag.go:115`) hard-requires a battle:

    skill/bag.go:120  "skill: UseItem: no battle in progress on map %02x at (%d,%d)"

So S7-6's `skill.Pickup` now collects Potions and Antidotes the agent
**cannot use**. Healing still means walking to a Pokemon Center, which is what
S6-0f measured as a real cost in the Butterfree/Brock line.

DERIVED — these are ordinary field medicines, not battle-only items:

    engine/items/item_effects.asm:30-34
        dw ItemUseMedicine   ; ANTIDOTE
        dw ItemUseMedicine   ; BURN_HEAL
        dw ItemUseMedicine   ; ICE_HEAL
        dw ItemUseMedicine   ; AWAKENING
        dw ItemUseMedicine   ; PARLYZ_HEAL

**UNMEASURED:** the start-menu path (START -> ITEM -> pick -> pick a party mon)
has never been driven. `skill/bag.go`'s existing menu machinery is the
battle-menu path and reuses `selectBagEntry`; how much of it transfers is a
measurement, not an assumption. Do not size this item until someone has driven
the start menu once.

Menus are step-and-verify: press, assert `wCurrentMenuItem` reached the wanted
index, then A. Never a press count.

---

## 3. The "forget a move?" prompt is an open bug, not a gap

This one is already written down as broken, in our own code:

    skill/train.go:57-59
      "The one case it does NOT fully handle is the 'forget a move?' prompt
       (a mon offered a new move with four already known) ... but dismisses
       it, so the move is not learned."

DERIVED — `engine/pokemon/learn_move.asm`: with four moves known,
`TryingToLearn` (:98) prints the prompt and a NO answer jumps to
`AbandonLearning` (:76). It is an ordinary two-option menu, so
`state.DecodeTwoOptionMenu` sees it and `SelectMenuItem` can answer it.

A yes/no answered by reflex has now cost this project three times: S6-3 lost an
already-caught Caterpie to a blind A on the nickname prompt, S6-4 measured
Battle silently answering NO to this exact offer, and `train.go` still does.
Fix it before it corrupts another measurement — a run that silently drops a
move looks like a weak planner in the scoreboard.

The policy question ("which move should it forget?") is real but is NOT a
reason to defer the fix. Answering deliberately with a stated rule beats
answering NO by accident.

---

## 4. Fleeing

`skill/battle.go` names the RUN option only in comments (`:52`, `:60`, `:219`,
`:235`). There is no escape path, so every wild encounter is fought to the end.

DERIVED — the mechanism: `engine/battle/core.asm:1078` jumps to
`TryRunningFromBattle` (:1496), which reads and increments `wNumRunAttempts`
(`ram/wram.asm:1606`) — so escape odds IMPROVE with repeated attempts within
one battle, and `end_of_battle.asm:55` zeroes it afterwards. A failed flee is
not a wall; it is a retry with better odds. `CantEscapeText` (:1573) is the
trapped case.

Mt. Moon is where this stops being a nicety. Three floors (`MT_MOON_1F` $3B,
`MT_MOON_B1F` $3C, `MT_MOON_B2F` $3D), a cave full of Zubat, and eleven
trainers who must be fought. Fighting every wild encounter on top of that burns
HP, the frame budget, and the failure budget.

---

## 5. Places through Cerulean

DERIVED, from `constants/map_constants.asm`:

    ROUTE_3            $0E     MT_MOON_1F        $3B
    ROUTE_4            $0F     MT_MOON_B1F       $3C
    CERULEAN_CITY      $03     MT_MOON_B2F       $3D
    CERULEAN_POKECENTER $40    MT_MOON_POKECENTER $44
    CERULEAN_GYM       $41     CERULEAN_MART     $43

Coordinates come from `skill.Place` or the ROM, never literals — the project
has already paid for a hand-written literal that named a counter tile no player
could stand on. Every new destination gets its coordinate measured, and the
Mt. Moon floors connect by LADDERS, which is a warp shape no current
destination exercises.

---

## 6. Party size and the PC — smaller than it looks now

Nothing in `agent/` or `skill/` reads `wPartyCount` (`ram/wram.asm:1722`). A
party of six makes catching fail with no diagnosis.

S7-9 is landing the correction that makes this cheap: the PC is a
`hidden_event` (`OpenPokemonCenterPC`) at tile **(13,3)** of every Pokemon
Center, activated by standing on it and facing UP — it is NOT an NPC sprite,
which is why S6-4's search of the map object files found nothing and concluded,
wrongly, that Centers have no PC.

Stretch goal. Only needed once the agent catches more than it can carry.

---

## 7. Status conditions

Poison walks the party into a blackout: `skill/travel.go:29` types it honestly
as an outcome, so poison is *survivable* but never *curable*. Viridian Forest
has a free ANTIDOTE at (25,11) that S7-6's Pickup can now collect and item 2
above would make usable. Falls out of item 2; not its own item.

---

## What the items above are worth

The Mt. Moon 1F ground items alone (`MtMoon1F.asm:37-42`) are six balls, which
is more than the whole game has offered so far:

    (2,20) POTION      (2,2)  MOON_STONE    (35,31) RARE_CANDY
    (36,23) ESCAPE_ROPE (20,33) POTION      (5,32)  TM_WATER_GUN

`Route4.asm:22` adds TM_WHIRLWIND at (57,3). S6-3 measured that five Poke Balls
were not enough for one Caterpie; a dungeon floor with six free items is a
different order of resource. It also makes the Pickup verb's decision layer —
"which items are worth a detour" — a real question rather than a hypothetical.

---

## Suggested sequencing

1. **Fix the `StepFrames` sampling seam (0).** Slice 7 built an NPC-derivation
   path the planner cannot hear. Everything else is worth less until it lands,
   and it invalidates readings taken through per-frame samplers.
2. **Fix the move-forget prompt (3).** It is a bug, it is small, and every
   further measurement is taken through it.
3. **Sight lines for the router (1).** The Pewter gym trainer is measured;
   Route 3's seven cannot be side-stepped one at a time. Facing and range are
   already in `rom.Object` after S7-5.
4. **Item use outside battle (2)**, which also closes status conditions (7).
5. **Flee (4) and places (5) together** — they are what make Mt. Moon
   affordable and the Cerulean goal sayable.
6. Party/PC (6) only when the agent actually fills a party.

Explicitly OUT: HM field moves (Cut is not needed until Vermilion), hidden
items (need ITEMFINDER, `engine/items/itemfinder.asm`), the bike, trading, and
the Nugget Bridge rival fight.

---

## WARNING: slice 7 is not merged

Slice 7 finished 9/9 at 16:21 on 2026-08-29 (tip `16c0ab7`), but every task
lives on `agent-plan/76ac5d0c-2903-4264-905e-6231652e145c` and NOT on `main`.
Nothing in this file can be planned against `main` as it stands.

Note also that S7-8's milestone run **lost to Brock** (lead L12, 36/36 HP, no
status, `outcome=ResultLost`, no badge). Both affordances were proved from RAM
before the fight, so the test logs the loss as a finding and passes — a game
outcome is not a defect, and the rDIV-seeded RNG makes it cycle-dependent. But
it means the badge has not actually been re-earned since the goal landed.

Separately, the `main` working tree carried an uncommitted `max_tokens: 32` in
`agent/llm.go`. S7-3 (on the plan branch, `agent/llm.go:193`) rejects any reply
whose `finish_reason` is not `"stop"`, and S7-4 turns three rejections into a
terminal `StopError`. Merged together, a 32-token cap truncates the reply,
which reports `"length"`, which kills every run on the first round — and the
scoreboard would read like a model failure. **Fixed 2026-08-29** to a named
`maxReplyTokens = 512`, above a `<think>` block plus `{"choice": N}`; still
uncommitted, so keep 512 when resolving the merge.
