package skill

import (
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

// GetParcel collects Oak's parcel from the Viridian Mart. From the
// post-starter state it Travels to the mart, lets the entry cutscene run to
// completion, and returns once the parcel flag is set and the parcel is in
// the bag. It does not deliver the parcel to Oak; that is a later task.
//
// The parcel comes from the map script, not from an NPC dialogue. Entering
// the mart (pokered/scripts/ViridianMart.asm, ViridianMartDefaultScript)
// shows a greeting box and force-walks the player from the door tile (3,7)
// to (2,5); once that walk finishes, ViridianMartOaksParcelScript shows the
// parcel box and gives the item the frame the box opens, setting
// EVENT_GOT_OAKS_PARCEL. There is no Talk to issue.
//
// Travel cannot finish a leg into the mart: the greeting box keeps the
// player uncontrollable, so a Travel that fails while the player is on the
// mart is the normal entry and is resumed with Cutscene. Any other Travel
// failure is returned as-is.
func GetParcel(m *emu.Emu, romData []byte, policy MovePolicy) error {
	dest, ok := Place("viridian mart")
	if !ok {
		return fmt.Errorf("skill: GetParcel: Place \"viridian mart\" not found")
	}

	if _, err := Travel(m, romData, dest, policy, parcelRouteMaxBattles); err != nil {
		if m.Peek8(sym.CurMap) != dest.Map {
			return fmt.Errorf("skill: GetParcel: %w", err)
		}
		// On the mart: the entry cutscene is in flight or already done.
		// Run it to completion; the parcel event is set the frame the
		// parcel box opens, and Cutscene keeps going until the box is
		// closed and the player is controllable again.
		if err := Cutscene(m, parcelCutsceneBudget, func(mem *state.Mem) bool {
			return state.HasEvent(mem, state.EventGotOaksParcel)
		}); err != nil {
			return fmt.Errorf("skill: GetParcel: %w", err)
		}
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
