package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ItemOaksParcel is the bag item ID of OAK's PARCEL, from the decomp
// (pokered/constants/item_constants.asm: `const OAKS_PARCEL ; $46`).
const ItemOaksParcel = 0x46

// ItemPokeBall is the bag item ID of POKE_BALL, the item Oak later gives
// the player (5x, pokered/scripts/OaksLab.asm .give_poke_balls - a talk
// reached only after the Route 22 rival battle, not the parcel hand-over
// chain). It is the 0x04 entry in the decomp's item table
// (pokered/constants/item_constants.asm: `const POKE_BALL ; $04`).
const ItemPokeBall = 0x04

// parcelRouteMaxBattles bounds the wild encounters Travel may fight on the
// post-starter -> Viridian Mart route. The only grass on that route is
// Route 1's, which throws one Pidgey; 20 is headroom, the same constant the
// fixture builders use.
const parcelRouteMaxBattles = 20

// parcelCutsceneBudget bounds the mart's entry cutscene: the greeting box,
// the three-tile force-walk, and the parcel box with its fanfare. It is a
// budget, not a prediction: exceeding it is an error carrying the map,
// position, wJoyIgnore and wFontLoaded diagnostics from Cutscene.
const parcelCutsceneBudget = 2000

// deliveryBudget bounds the hand-over chain run by advanceUntil: the parcel
// dialogue, the rival's arrival, the Pokedex-giving text run, and the rival's
// exit. It is a budget, not a prediction; exhausting it is an error carrying
// the map, position, wJoyIgnore and wFontLoaded diagnostics.
const deliveryBudget = 40000

// labEntryBudget bounds the lab's entry force-walk (the door is not a plain
// warp; the lab script slides the player on entry). A Travel that fails while
// the player is on the lab is the normal entry and is resumed with a
// Cutscene on this budget.
const labEntryBudget = 2000

// route22CutsceneBudget bounds the pre-battle cutscene fired by stepping
// onto the Route 22 trigger tile (Route22DefaultScript, Route22.asm:58):
// the emotion bubble, MUSIC_MEET_RIVAL, the rival's walk from his home tile
// (25,5), and the intro box that starts the battle when dismissed.
const route22CutsceneBudget = 10000

// route22AfterBattleBudget bounds Route22Rival1AfterBattleScript
// (Route22.asm:167), the event's only setter: its text box, the music
// change, and the rival's exit walk, which holds wJoyIgnore until it ends.
const route22AfterBattleBudget = 10000

// route22MaxBattles bounds the wild encounters Travel may fight on each leg
// of the balls journey (lab -> Route 22 and back): Route 1's grass and
// Route 22's own. 40 is headroom, in the style of parcelRouteMaxBattles.
const route22MaxBattles = 40

// pokeballTalkBudget bounds the .give_poke_balls talk: CheckAndSetEvent,
// GiveItem, and the two-box explanation (OaksLab.asm:1022-1029). It is a
// budget, not a prediction; exhausting it is an error carrying the map,
// position, wJoyIgnore and wFontLoaded diagnostics.
const pokeballTalkBudget = 8000

// GetParcel collects Oak's parcel from the Viridian Mart. From the
// post-starter state it Travels to the mart, lets the entry cutscene run to
// completion, and returns once the parcel flag is set and the parcel is in
// the bag. It does not deliver the parcel to Oak; that is a later task.
//
// The parcel comes from the map script, not from an NPC dialogue. Entering
// the mart (pokered/scripts/ViridianMart.asm, ViridianMartDefaultScript)
// shows a greeting box and force-walks the player from the door tile (3,7)
// to (2,5); once that walk finishes, ViridianMartOaksParcelScript shows the
// parcel box, and the item is given and EVENT_GOT_OAKS_PARCEL set when the
// box is closed (DisplayTextID blocks until A). There is no Talk to issue.
//
// Travel can reach the destination while the entry cutscene is still in
// flight: the force-walk leaves controllable gaps, so a walk recovered from
// the greeting box can land on (2,5) before ViridianMartOaksParcelScript
// runs, and Travel reports success. "On the mart without the flag" therefore
// means the cutscene is still ahead, not that the journey failed, and the
// cutscene is run to completion whether Travel failed or succeeded. Any
// Travel failure with the player off the mart is returned as-is.
func GetParcel(m *emu.Emu, romData []byte, policy MovePolicy) error {
	dest, ok := Place("viridian mart")
	if !ok {
		return fmt.Errorf("skill: GetParcel: Place \"viridian mart\" not found")
	}

	if _, err := Travel(m, romData, dest, policy, parcelRouteMaxBattles); err != nil {
		if m.Peek8(sym.CurMap) != dest.Map {
			return fmt.Errorf("skill: GetParcel: %w", err)
		}
	}

	// On the mart: the entry cutscene is in flight, or already done (then
	// the predicate holds on the first snapshot and Cutscene returns
	// without input). It runs to completion until the parcel event is set
	// and the player is controllable again.
	if err := Cutscene(m, parcelCutsceneBudget, func(mem *state.Mem) bool {
		return state.HasEvent(mem, state.EventGotOaksParcel)
	}); err != nil {
		return fmt.Errorf("skill: GetParcel: %w", err)
	}

	// The cutscene can end with the player off the destination tile (a
	// retried walk that overshot the force-walk's terminus), so land the
	// player back on dest before the postcondition. A no-op when the walk
	// ended where the cutscene left it.
	if err := GoTo(m, romData, dest); err != nil {
		return fmt.Errorf("skill: GetParcel: %w", err)
	}

	// Positive postcondition, both halves: the story flag is set AND the
	// item is in the bag. Either alone could be a half-finished cutscene.
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventGotOaksParcel) {
		return fmt.Errorf("skill: GetParcel: parcel flag not set (map %#04x at (%d,%d))",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == ItemOaksParcel {
			return nil
		}
	}
	return fmt.Errorf("skill: GetParcel: parcel flag set but no OAK's PARCEL in the bag")
}

// OaksParcel delivers Oak's parcel to Professor Oak in his lab (map 0x28).
// From the post-starter state it first collects the parcel from the Viridian
// Mart (GetParcel), then Travels back to the lab and hands the parcel to Oak,
// letting the hand-over chain run to completion: Oak thanks the player, the
// rival arrives at Oak's request, Oak hands over the Pokedex, and the rival
// leaves for Route 22. It returns once EVENT_GOT_POKEDEX is set and the
// player is controllable again, in the lab.
//
// The hand-over is a Talk, not a map script: OaksLabOak1Text (
// pokered/scripts/OaksLab.asm) checks the parcel in .check_got_parcel
// (1004) and, when it is present, shows the dialogue, removes the parcel
// from the bag, and switches wOaksLabCurScript to
// RIVAL_ARRIVES_AT_OAKS_REQUEST (1015) to start the chain. advanceUntil
// drives that chain: it taps A as each box appears and runs until the
// Pokedex event is set and control returns to the player.
//
// What the chain does and does not do (decomp, measured 2026-08-26):
//
//	.got_parcel (1011) -> OaksLabRivalArrivesAtOaksRequestScript (510)
//	-> OaksLabOakGivesPokedexScript (554: sets EVENT_GOT_POKEDEX and
//	EVENT_OAK_GOT_PARCEL, hides the Pokedex props, walks the rival down)
//	-> OaksLabRivalLeavesWithPokedexScript (628: hides the rival, sets
//	EVENT_1ST_ROUTE22_RIVAL_BATTLE and EVENT_ROUTE22_RIVAL_WANTS_BATTLE,
//	spawns him on Route 22, clears wJoyIgnore) -> NoopScript.
//
// The chain ends with the Pokedex handed over and the rival waiting on
// Route 22 for his second battle. It does NOT give the 5x POKE_BALL and
// does NOT set EVENT_GOT_POKEBALLS_FROM_OAK: .give_poke_balls (1022-1029)
// is a separate, later talk with Oak, reached only when
// EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE is set (OaksLab.asm:988-989). That
// event has exactly one setter in the decomp, Route22.asm:167, inside
// Route22Rival1AfterBattleScript - the after-battle script of the Route 22
// rival battle, a later story beat this task does not play.
//
// Re-entering the lab through the Pallet door is not a plain warp: the lab
// script force-walks the player on entry. A Travel that fails while the
// player is on the lab is the normal entry and is resumed with a Cutscene
// that waits until the player is controllable again (the same pattern
// GetParcel uses for the mart's entry), then re-runs Travel to the approach
// tile. Any other Travel failure is returned as-is.
func OaksParcel(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if err := GetParcel(m, romData, policy); err != nil {
		return fmt.Errorf("skill: OaksParcel: %w", err)
	}

	// Place("oak's lab") resolves to (5,3), the open floor tile directly
	// below Oak (5,2); it is also the tile the player stands on after the
	// lab's entry sequence in GetStarter, so it is a known-good approach
	// tile.
	dest, ok := Place("oak's lab")
	if !ok {
		return fmt.Errorf("skill: OaksParcel: Place \"oak's lab\" not found")
	}
	if _, err := Travel(m, romData, dest, policy, parcelRouteMaxBattles); err != nil {
		if m.Peek8(sym.CurMap) != dest.Map {
			return fmt.Errorf("skill: OaksParcel: %w", err)
		}
		// On the lab: the entry force-walk is in flight. Run it to
		// completion, then resume the walk to the approach tile.
		if err := Cutscene(m, labEntryBudget, state.Controllable); err != nil {
			return fmt.Errorf("skill: OaksParcel: %w", err)
		}
		if _, err := Travel(m, romData, dest, policy, parcelRouteMaxBattles); err != nil {
			return fmt.Errorf("skill: OaksParcel: %w", err)
		}
	}

	// Face Oak and open the parcel dialogue; the first A press hands over
	// the parcel and starts the hand-over chain.
	if err := Face(m, 5, 2); err != nil {
		return fmt.Errorf("skill: OaksParcel: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	mem := advanceUntil(m, deliveryBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, state.EventGotPokedex) && state.Controllable(mm)
	})
	if !state.HasEvent(&mem, state.EventGotPokedex) {
		return fmt.Errorf("skill: OaksParcel: %s not set after the hand-over: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			state.EventGotPokedex, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: OaksParcel: not controllable after the hand-over: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	return nil
}

// Route22RivalLeadLevel is the lead level at which the Route 22 rival
// battle is a reliable win. His party (pokered/data/trainers/parties.asm
// Rival1Data, Route 22 section) is Pidgey Lv9 plus the starter line the
// player did NOT pick at Lv8 — for the Squirtle line that is BULBASAUR
// Lv8. The post-parcel lead (level 7) loses it; 15 beats both mons with
// room for the RNG. GetPokeBalls does not train: callers that need it to
// succeed should level the lead to this first, in the gym-test pattern.
const Route22RivalLeadLevel = 15

// ErrLostRoute22RivalBattle reports that the Route 22 rival battle was lost:
// the blackout relocated the player to the respawn spot, so GetPokeBalls
// stops and reports it rather than pressing on from a state it did not
// verify. Losing is a typed outcome, not a panic.
var ErrLostRoute22RivalBattle = errors.New("skill: GetPokeBalls: lost the Route 22 rival battle; the blackout relocated the player")

// GetPokeBalls gets the five POKE_BALLs Oak gives once the Route 22 rival
// battle is won. From the post-parcel state it fights the Route 22 rival if
// that battle has not been won yet, then Travels back to Oak's lab and runs
// the talk. It returns once EVENT_GOT_POKEBALLS_FROM_OAK is set AND the bag
// holds 5x POKE_BALL.
//
// The dispatch order in OaksLabOak1Text (pokered/scripts/OaksLab.asm
// .check_for_poke_balls) is the spec of this skill:
//
//	IsItemInBag POKE_BALL            -> holding even ONE ball gets small talk,
//	                                       never the five
//	EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE -> .give_poke_balls
//	EVENT_GOT_POKEDEX                 -> .mon_around_the_world (small talk)
//
// Two consequences that shape the skill:
//
//   - Already-done is success. If EVENT_GOT_POKEBALLS_FROM_OAK is set, the
//     script will never give the balls again (CheckAndSetEvent's nz branch
//     and the IsItemInBag guard both fall to small talk), so GetPokeBalls
//     returns nil immediately instead of retrying a talk that can never
//     fire. A run resumed from a checkpoint must not wedge here.
//   - The battle must actually be fought. Without the Route 22 event the
//     same Oak talk falls through to .mon_around_the_world and looks exactly
//     like success from the outside: a box appeared, it was dismissed,
//     control returned. A predicate that checks "the talk completed" would
//     pass forever with an empty bag, so the postcondition asserts the BAG,
//     not just the event.
//
// The Route 22 battle is a coordinate trigger, not an NPC to Talk to:
// Route22DefaultScript (pokered/scripts/Route22.asm:58) fires when
// EVENT_ROUTE22_RIVAL_WANTS_BATTLE is set (the parcel hand-over chain sets
// it) and the player stands on (29,4) or (29,5). Stepping onto the trigger
// tile holds wJoyIgnore (PAD_CTRL_PAD) through the emotion bubble and music,
// clears it in Route22Rival1StartBattleScript, and shows the intro box that
// starts the battle when dismissed. The after-battle script
// (Route22.asm:167) is the event's only setter; it re-holds wJoyIgnore for
// its own text and the rival's exit walk before clearing it, so settling
// waits for controllable rather than fighting the input lock.
func GetPokeBalls(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)

	if state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		return nil
	}

	// The battle. EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE has exactly one setter
	// in the decomp (Route22.asm:167), so if it is not set the battle must
	// be fought and won; there is no other way to reach .give_poke_balls.
	if !state.HasEvent(&mem, state.EventBeatRoute22Rival1stBattle) {
		dest, ok := Place("route 22")
		if !ok {
			return fmt.Errorf("skill: GetPokeBalls: Place \"route 22\" not found")
		}
		if _, err := Travel(m, romData, dest, policy, route22MaxBattles); err != nil {
			if m.Peek8(sym.CurMap) != dest.Map {
				return fmt.Errorf("skill: GetPokeBalls: %w", err)
			}
			// On Route 22: stepping onto the trigger tile set wJoyIgnore and
			// can abort the walk, which is the normal arrival. The Cutscene
			// below picks the cutscene up either way.
		}

		// Let the pre-battle cutscene run until the battle is up: the script
		// drives the bubble, the music and the rival's walk, taps A on the
		// intro box, and the battle starts. wJoyIgnore is held for most of it
		// and cleared by the start-battle script; wait for control rather
		// than fighting it.
		if err := Cutscene(m, route22CutsceneBudget, func(mm *state.Mem) bool {
			return state.DecodeBattle(mm) != nil
		}); err != nil {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}

		outcome, err := Battle(m, policy)
		if err != nil {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}
		if outcome == state.ResultLost {
			state.Snapshot(m, &mem)
			return fmt.Errorf("skill: GetPokeBalls: map=%#04x at (%d,%d): %w",
				mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord), ErrLostRoute22RivalBattle)
		}

		// Battle settles on controllable, which holds in the gap between the
		// battle ending and Route22Rival1AfterBattleScript running — so the
		// win can be reported before the event's only setter has executed.
		// Run that script to completion: its text box (A), the rival's exit
		// walk, and the wJoyIgnore it holds until the exit script clears it.
		if err := Cutscene(m, route22AfterBattleBudget, func(mm *state.Mem) bool {
			return state.HasEvent(mm, state.EventBeatRoute22Rival1stBattle)
		}); err != nil {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}

		state.Snapshot(m, &mem)
		if !state.HasEvent(&mem, state.EventBeatRoute22Rival1stBattle) {
			return fmt.Errorf("skill: GetPokeBalls: battle won but %s not set: map=%#04x at (%d,%d)",
				state.EventBeatRoute22Rival1stBattle, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord))
		}
	}

	// Back to the lab. Re-entering through the Pallet door is not a plain
	// warp: the lab script force-walks the player on entry, so a Travel that
	// fails while on the lab is the normal entry (the OaksParcel pattern).
	dest, ok := Place("oak's lab")
	if !ok {
		return fmt.Errorf("skill: GetPokeBalls: Place \"oak's lab\" not found")
	}
	if _, err := Travel(m, romData, dest, policy, route22MaxBattles); err != nil {
		if m.Peek8(sym.CurMap) != dest.Map {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}
		if err := Cutscene(m, labEntryBudget, state.Controllable); err != nil {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}
		if _, err := Travel(m, romData, dest, policy, route22MaxBattles); err != nil {
			return fmt.Errorf("skill: GetPokeBalls: %w", err)
		}
	}

	// Face Oak and open the talk. With EVENT_BEAT_ROUTE22_RIVAL_1ST_BATTLE
	// set and no ball in the bag, OaksLabOak1Text falls to .give_poke_balls:
	// CheckAndSetEvent, GiveItem POKE_BALL 5, and the explanation text.
	if err := Face(m, 5, 2); err != nil {
		return fmt.Errorf("skill: GetPokeBalls: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	mem = advanceUntil(m, pokeballTalkBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, state.EventGotPokeballsFromOak) && state.Controllable(mm)
	})

	// Positive postcondition, both halves: the story flag is set AND the bag
	// holds 5x POKE_BALL. CheckAndSetEvent runs BEFORE GiveItem, so the flag
	// alone can be a half-finished talk; asserting only it would pass with
	// an empty bag.
	if !state.HasEvent(&mem, state.EventGotPokeballsFromOak) {
		return fmt.Errorf("skill: GetPokeBalls: %s not set after the talk: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x",
			state.EventGotPokeballsFromOak, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
			mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded))
	}
	balls := 0
	for _, it := range state.DecodeInventory(&mem).Items {
		if it.ID == ItemPokeBall {
			balls += int(it.Quantity)
		}
	}
	if balls != 5 {
		return fmt.Errorf("skill: GetPokeBalls: %s set but the bag holds %d POKE_BALL, want 5", state.EventGotPokeballsFromOak, balls)
	}
	return nil
}
