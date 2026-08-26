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
