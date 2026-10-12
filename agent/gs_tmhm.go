package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsFirstHM = 51 // HM01's machine number; HM moves can never be forgotten.
	gsLastHM  = 57
	// gsMachineMinGain keeps marginal reshuffles off the menu.
	gsMachineMinGain = 10
)

type gsMachineDecision struct {
	Machine       int // TM01..TM50 => 1..50, HM01..HM07 => 51..57
	Item          uint8
	Move          uint8
	Slot, Replace int // Replace -1 = empty move slot
	Before, After int
}

// gsMoveSetScore values one move list for a mon of types t1/t2: the best
// accuracy-weighted, STAB-adjusted power per damaging type plus a coverage
// bonus per distinct type, and one utility move is worth keeping.
// ponytail: power-only model; fixed-damage and multi-hit moves are
// undervalued. Use the battle strategy's move evaluation if that misleads.
func gsMoveSetScore(romData []byte, t1, t2 uint8, moves [4]uint16) int {
	best := map[uint8]int{}
	status := false
	for _, id := range moves {
		if id == 0 || id > 0xff {
			continue
		}
		mv, err := gsrom.LookupMove(romData, uint8(id))
		if err != nil {
			continue
		}
		if mv.Power == 0 {
			status = true
			continue
		}
		v := int(mv.Power) * int(mv.Accuracy) / 255
		if mv.Type == t1 || mv.Type == t2 {
			v = v * 3 / 2
		}
		if v > best[mv.Type] {
			best[mv.Type] = v
		}
	}
	score := 0
	for _, v := range best {
		score += v + 25
	}
	if status {
		score += 20
	}
	if len(best) == 0 {
		score -= 200
	}
	return score
}

func gsHMMoves(romData []byte) map[uint16]bool {
	out := map[uint16]bool{}
	for n := gsFirstHM; n <= gsLastHM; n++ {
		if mv, err := gsrom.TMHMMove(romData, n); err == nil {
			out[uint16(mv)] = true
		}
	}
	return out
}

// gsDecideMachine picks the party slot and move slot where machine raises the
// move-set score most. onlySlot >= 0 restricts the carrier. HMs only fill an
// empty slot because Gen II never lets the player forget them.
func gsDecideMachine(romData []byte, machine int, species []uint8, eligible []bool, moves [][4]uint16, onlySlot int) (gsMachineDecision, bool) {
	move, err := gsrom.TMHMMove(romData, machine)
	if err != nil || machine < 1 || machine > len(gsdata.MachineItems) {
		return gsMachineDecision{}, false
	}
	base, err := gsrom.LocateBaseData(romData)
	if err != nil {
		return gsMachineDecision{}, false
	}
	hm := machine >= gsFirstHM
	permanent := gsHMMoves(romData)
	best, found := gsMachineDecision{}, false
	for slot := range species {
		if (onlySlot >= 0 && slot != onlySlot) || slot >= len(moves) || !eligible[slot] {
			continue
		}
		if ok, err := gsrom.CanLearnTMHMAt(romData, base, species[slot], machine); err != nil || !ok {
			continue
		}
		known := moves[slot]
		already := false
		for _, id := range known {
			already = already || id == uint16(move)
		}
		if already {
			continue
		}
		t1, t2, err := gsrom.SpeciesTypesAt(romData, base, species[slot])
		if err != nil {
			continue
		}
		before := gsMoveSetScore(romData, t1, t2, known)
		for _, replace := range gsMachinePlacements(known, hm, permanent) {
			next, at := known, replace
			if replace < 0 {
				at = gsFirstEmptySlot(known)
			}
			next[at] = uint16(move)
			after := gsMoveSetScore(romData, t1, t2, next)
			if after-before < gsMachineMinGain || (found && after-before <= best.After-best.Before) {
				continue
			}
			best, found = gsMachineDecision{
				Machine: machine, Item: gsdata.MachineItems[machine-1], Move: move,
				Slot: slot, Replace: replace, Before: before, After: after,
			}, true
		}
	}
	return best, found
}

// gsMachinePlacements lists legal move slots for a new move: only the first
// empty slot when one exists (-1), otherwise every slot holding a forgettable
// move. An HM never displaces a move.
func gsMachinePlacements(known [4]uint16, hm bool, permanent map[uint16]bool) []int {
	if gsFirstEmptySlot(known) >= 0 {
		return []int{-1}
	}
	if hm {
		return nil
	}
	var out []int
	for i, id := range known {
		if !permanent[id] {
			out = append(out, i)
		}
	}
	return out
}

func gsFirstEmptySlot(known [4]uint16) int {
	for i, id := range known {
		if id == 0 {
			return i
		}
	}
	return -1
}

type gsTeachingState struct {
	owned    []int
	species  []uint8
	eligible []bool
	moves    [][4]uint16
}

func readGSTeachingState(m *emu.Emu, romData []byte) (gsTeachingState, error) {
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return gsTeachingState{}, err
	}
	species, eligible := profile.PartyNativeSpecies(m)
	return gsTeachingState{
		owned:    profile.OwnedMachineNumbers(m),
		species:  species,
		eligible: eligible,
		moves:    profile.DecodeBattleExecution(m).PartyMoves,
	}, nil
}

// gsMachineObjectives offers each owned TM/HM whose best placement improves a
// party member's move set.
func gsMachineObjectives(m *emu.Emu, romData []byte) []Objective {
	if m == nil {
		return nil
	}
	st, err := readGSTeachingState(m, romData)
	if err != nil {
		return nil
	}
	var out []Objective
	for _, n := range st.owned {
		d, ok := gsDecideMachine(romData, n, st.species, st.eligible, st.moves, -1)
		if !ok {
			continue
		}
		item, ok := gsdata.Item(d.Item)
		if !ok {
			continue
		}
		placement := "empty move slot"
		if d.Replace >= 0 {
			placement = fmt.Sprintf("replace move slot %d", d.Replace)
		}
		semantics := "TMs are single-use"
		if n >= gsFirstHM {
			semantics = "reusable; a learned HM cannot be forgotten"
		}
		out = append(out, Objective{
			Kind: KindUseItem, Item: ItemID(item), Slot: d.Slot,
			Note: fmt.Sprintf("(teaches move %d; %s; party slot %d move-set score %d->%d; %s)",
				d.Move, semantics, d.Slot, d.Before, d.After, placement),
		})
	}
	return out
}

func gsMachineNumberForItem(id ItemID) (int, bool) {
	for i, raw := range gsdata.MachineItems {
		if item, ok := gsdata.Item(raw); ok && ItemID(item) == id {
			return i + 1, true
		}
	}
	return 0, false
}

// executeGSTeachMachine re-decides the placement from live state for the
// offered carrier, then drives the native PACK teaching transaction, which
// proves the carrier learned the move.
func executeGSTeachMachine(m *emu.Emu, romData []byte, o Objective) error {
	machine, ok := gsMachineNumberForItem(o.Item)
	if !ok {
		return fmt.Errorf("%w: %s is not a TM/HM", errGSControllerUnavailable, o.Item)
	}
	st, err := readGSTeachingState(m, romData)
	if err != nil {
		return err
	}
	owned := false
	for _, n := range st.owned {
		owned = owned || n == machine
	}
	if !owned {
		return fmt.Errorf("%w: %s is not in the TM/HM pocket", errGSControllerUnavailable, o.Item)
	}
	d, ok := gsDecideMachine(romData, machine, st.species, st.eligible, st.moves, o.Slot)
	if !ok {
		return fmt.Errorf("%w: %s no longer improves party slot %d", errGSControllerUnavailable, o.Item, o.Slot)
	}
	if err := skill.TeachNativeMachine(m, game.NativeFieldMove{MachineItemID: uint16(d.Item), MoveID: uint16(d.Move)}, d.Slot, d.Replace); err != nil {
		return err
	}
	// The START menu's closing frames still own the overworld; wait (no
	// input) so the objective ends at a controllable boundary.
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	for waited := 0; waited < gsTeachSettleFrames; waited += 10 {
		if profile.DecodeOverworld(m).Controllable {
			return nil
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("%w: control did not return %d frames after teaching %s", errGSBoundaryUnsafe, gsTeachSettleFrames, o.Item)
}

const gsTeachSettleFrames = 600
