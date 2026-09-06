package skill

import (
	"errors"
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

var (
	ErrFieldRosterPrerequisite = errors.New("skill: field roster prerequisite is missing")
	ErrFieldRosterNoRecovery   = errors.New("skill: no roster change can restore the required field capability")
	ErrFieldRosterCatch        = errors.New("skill: failed to acquire a compatible field-move Pokemon")
)

// CoreProgressionFieldMoves is the field-move invariant late-game story
// progression must preserve. Fly and Flash are useful, but Cut/Surf/Strength
// are the moves that can make the remaining mandatory path physically
// impossible when a roster change drops their only user.
func CoreProgressionFieldMoves() []FieldMove {
	return []FieldMove{FieldCut, FieldSurf, FieldStrength}
}

// OwnedCoreProgressionFieldMoves returns the core moves whose HM and badge are
// already owned. It lets a story slice preserve everything the save has
// actually unlocked without assuming a canonical badge order or starter.
func OwnedCoreProgressionFieldMoves(mem *state.Mem) []FieldMove {
	var out []FieldMove
	for _, move := range CoreProgressionFieldMoves() {
		cap := FieldCapabilityFor(mem, move)
		if cap.HMOwned && cap.BadgeOwned {
			out = append(out, move)
		}
	}
	return out
}

func normalizeRequiredFieldMoves(required []FieldMove) ([]FieldMove, error) {
	want := map[FieldMove]bool{}
	for _, move := range required {
		if _, ok := FieldMoveSpecFor(move); !ok {
			return nil, fmt.Errorf("skill: unknown required field move %d", move)
		}
		want[move] = true
	}
	var out []FieldMove
	for _, move := range ProgressionFieldMoves() {
		if want[move] {
			out = append(out, move)
			delete(want, move)
		}
	}
	return out, nil
}

func monKnowsRequiredMove(mon state.Mon, move FieldMove) bool {
	spec, ok := FieldMoveSpecFor(move)
	return ok && monKnowsMove(mon, spec.MoveID)
}

// monCanPlaceFieldMove reports whether mon can end up knowing move without
// violating Gen I's permanent-HM rule. This mirrors DecideTMHM's legality but
// deliberately ignores battle-score preferences: a required traversal move
// may justify replacing any non-HM move.
func monCanPlaceFieldMove(romData []byte, mon state.Mon, move FieldMove) (bool, error) {
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return false, fmt.Errorf("unknown field move %d", move)
	}
	if monKnowsMove(mon, spec.MoveID) {
		return true, nil
	}
	compatible, err := rom.CanLearnTMHM(romData, mon.Species, spec.HMItem)
	if err != nil {
		return false, err
	}
	if !compatible {
		return false, nil
	}
	for _, known := range mon.Moves {
		if known == 0 {
			return true, nil
		}
		isHM, err := rom.IsHMMove(romData, known)
		if err != nil {
			return false, err
		}
		if !isHM {
			return true, nil
		}
	}
	return false, nil
}

// placeFieldMoveHypothetically mutates only the planner copy of mon. It uses
// the same legal placement rule as TeachTMHM: empty slot first, otherwise the
// first non-HM slot. The exact replacement score does not matter for coverage;
// the real teaching call will choose the best legal slot later.
func placeFieldMoveHypothetically(romData []byte, mon *state.Mon, move FieldMove) (bool, error) {
	spec, ok := FieldMoveSpecFor(move)
	if !ok {
		return false, fmt.Errorf("unknown field move %d", move)
	}
	if monKnowsMove(*mon, spec.MoveID) {
		return true, nil
	}
	compatible, err := rom.CanLearnTMHM(romData, mon.Species, spec.HMItem)
	if err != nil || !compatible {
		return false, err
	}
	for i, known := range mon.Moves {
		if known == 0 {
			mon.Moves[i] = spec.MoveID
			return true, nil
		}
	}
	for i, known := range mon.Moves {
		isHM, err := rom.IsHMMove(romData, known)
		if err != nil {
			return false, err
		}
		if !isHM {
			mon.Moves[i] = spec.MoveID
			return true, nil
		}
	}
	return false, nil
}

// partyCanSatisfyFieldMoves proves that a hypothetical party can eventually
// hold the entire required set at once. Each planned HM is written into a copy
// before the next requirement is considered, so one four-move Pokémon cannot
// be counted as infinite future capacity.
func partyCanSatisfyFieldMoves(romData []byte, mons []state.Mon, required []FieldMove) (bool, error) {
	planned := append([]state.Mon(nil), mons...)
	for _, move := range required {
		already := false
		for _, mon := range planned {
			if monKnowsRequiredMove(mon, move) {
				already = true
				break
			}
		}
		if already {
			continue
		}
		placed := false
		for i := range planned {
			ok, err := placeFieldMoveHypothetically(romData, &planned[i], move)
			if err != nil {
				return false, err
			}
			if ok {
				placed = true
				break
			}
		}
		if !placed {
			return false, nil
		}
	}
	return true, nil
}

func requiredMovesKnownBy(mon state.Mon, required []FieldMove) int {
	n := 0
	for _, move := range required {
		if monKnowsRequiredMove(mon, move) {
			n++
		}
	}
	return n
}

// chooseDepositSlotForIncoming returns the safest party member to store before
// adding incoming. A candidate is considered only if the resulting party can
// still satisfy the whole required field set. Among legal choices it first
// avoids current required-HM users, then stores the lowest-level/weakest bench
// member, preserving the lead on exact ties.
func chooseDepositSlotForIncoming(romData []byte, party state.PartyState, incoming state.Mon, required []FieldMove) (int, bool, error) {
	if party.Count < gen1PartyCapacity {
		mons := append(append([]state.Mon(nil), party.Mons...), incoming)
		ok, err := partyCanSatisfyFieldMoves(romData, mons, required)
		return -1, ok, err
	}

	best := -1
	bestRequired := int(^uint(0) >> 1)
	bestLevel := int(^uint(0) >> 1)
	bestStats := int(^uint(0) >> 1)
	for slot, mon := range party.Mons {
		mons := make([]state.Mon, 0, len(party.Mons))
		mons = append(mons, party.Mons[:slot]...)
		mons = append(mons, party.Mons[slot+1:]...)
		mons = append(mons, incoming)
		ok, err := partyCanSatisfyFieldMoves(romData, mons, required)
		if err != nil {
			return -1, false, err
		}
		if !ok {
			continue
		}
		requiredKnown := requiredMovesKnownBy(mon, required)
		stats := int(mon.MaxHP) + int(mon.Attack) + int(mon.Defense) + int(mon.Speed) + int(mon.Special)
		if best < 0 || requiredKnown < bestRequired ||
			(requiredKnown == bestRequired && int(mon.Level) < bestLevel) ||
			(requiredKnown == bestRequired && int(mon.Level) == bestLevel && stats < bestStats) ||
			(requiredKnown == bestRequired && int(mon.Level) == bestLevel && stats == bestStats && slot > best) {
			best, bestRequired, bestLevel, bestStats = slot, requiredKnown, int(mon.Level), stats
		}
	}
	return best, best >= 0, nil
}

func boxMonAsPartyMon(mon state.BoxMon) state.Mon {
	return state.Mon{Species: mon.Species, Moves: mon.Moves}
}

func fieldMovePotential(romData []byte, mon state.Mon, required []FieldMove) (int, error) {
	n := 0
	for _, move := range required {
		ok, err := monCanPlaceFieldMove(romData, mon, move)
		if err != nil {
			return 0, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

// chooseCompatibleBoxMon searches the active box for a target-compatible mon
// whose addition leaves the complete required set satisfiable. It prefers a
// mon useful for more of the required set, then the earlier box position.
func chooseCompatibleBoxMon(romData []byte, party state.PartyState, box state.BoxState, target FieldMove, required []FieldMove) (boxIndex, depositSlot int, ok bool, err error) {
	bestPotential := -1
	bestBox, bestDeposit := -1, -1
	for i, boxed := range box.Mons {
		incoming := boxMonAsPartyMon(boxed)
		canTarget, err := monCanPlaceFieldMove(romData, incoming, target)
		if err != nil {
			return -1, -1, false, err
		}
		if !canTarget {
			continue
		}
		deposit, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
		if err != nil {
			return -1, -1, false, err
		}
		if !legal {
			continue
		}
		potential, err := fieldMovePotential(romData, incoming, required)
		if err != nil {
			return -1, -1, false, err
		}
		if bestBox < 0 || potential > bestPotential {
			bestPotential, bestBox, bestDeposit = potential, i, deposit
		}
	}
	return bestBox, bestDeposit, bestBox >= 0, nil
}

type wildFieldCandidate struct {
	Destination Destination
	Map         uint8
	Species     uint8
	Slots       int
	RouteLen    int
}

func knownGrassDestinations() []Destination {
	seen := map[uint8]bool{}
	var out []Destination
	for _, name := range PlaceNames() {
		d, ok := Place(name)
		if !ok || seen[d.Map] {
			continue
		}
		seen[d.Map] = true
		out = append(out, d)
	}
	return out
}

// findWildFieldCandidate uses only ROM encounter tables and HM compatibility.
// It contains no species-specific "HM slave" table: any reachable known grass
// map may supply the missing capability. Nearer maps win; within the same map,
// species occupying more encounter slots win.
func findWildFieldCandidate(m *emu.Emu, romData []byte, target FieldMove, required []FieldMove) (wildFieldCandidate, bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	cur := mem.U8(sym.CurMap)
	g, err := world.BuildGraph(romData)
	if err != nil {
		return wildFieldCandidate{}, false, err
	}

	dests := knownGrassDestinations()
	x, y := playerXY(m)
	dests = append(dests, Destination{Map: cur, X: x, Y: y})
	sort.SliceStable(dests, func(i, j int) bool {
		if dests[i].Map != dests[j].Map {
			return dests[i].Map < dests[j].Map
		}
		if dests[i].X != dests[j].X {
			return dests[i].X < dests[j].X
		}
		return dests[i].Y < dests[j].Y
	})

	seenMap := map[uint8]bool{}
	best := wildFieldCandidate{RouteLen: int(^uint(0) >> 1)}
	found := false
	for _, dest := range dests {
		if seenMap[dest.Map] {
			continue
		}
		seenMap[dest.Map] = true
		route, err := world.FindRoute(g, cur, dest.Map)
		if err != nil {
			continue
		}
		wild, err := WildGrass(romData, dest.Map)
		if err != nil {
			return wildFieldCandidate{}, false, err
		}
		for _, species := range wild {
			incoming := state.Mon{Species: species.ID}
			canTarget, err := monCanPlaceFieldMove(romData, incoming, target)
			if err != nil {
				return wildFieldCandidate{}, false, err
			}
			if !canTarget {
				continue
			}
			_, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
			if err != nil {
				return wildFieldCandidate{}, false, err
			}
			if !legal {
				continue
			}
			candidate := wildFieldCandidate{Destination: dest, Map: dest.Map, Species: species.ID, Slots: species.Slots, RouteLen: len(route)}
			if !found || candidate.RouteLen < best.RouteLen ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots > best.Slots) ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots == best.Slots && candidate.Map < best.Map) ||
				(candidate.RouteLen == best.RouteLen && candidate.Slots == best.Slots && candidate.Map == best.Map && candidate.Species < best.Species) {
				best, found = candidate, true
			}
		}
	}
	return best, found, nil
}

// RepairFieldCapabilities makes every required move usable by the current
// party. It first teaches within the existing roster, then tries the active PC
// box, and finally acquires a ROM-compatible wild species from a reachable
// known grass map. Every roster mutation goes through the real PC/catch UI and
// every learned move is verified by EnsureFieldMove from party RAM.
func RepairFieldCapabilities(m *emu.Emu, romData []byte, policy MovePolicy, required []FieldMove) error {
	if policy == nil {
		return fmt.Errorf("skill: RepairFieldCapabilities: nil move policy")
	}
	required, err := normalizeRequiredFieldMoves(required)
	if err != nil {
		return err
	}
	if len(required) == 0 {
		return nil
	}

	for _, target := range required {
		var mem state.Mem
		state.Snapshot(m, &mem)
		cap := FieldCapabilityFor(&mem, target)
		if !cap.BadgeOwned || !cap.HMOwned {
			return fmt.Errorf("%w: %s badge=%v HM=%v", ErrFieldRosterPrerequisite, cap.Name, cap.BadgeOwned, cap.HMOwned)
		}
		if cap.Usable {
			continue
		}
		if CanPrepareFieldMove(romData, &mem, target) {
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: prepare %s in current party: %w", target, err)
			}
			continue
		}

		party := state.DecodeParty(&mem)
		box := state.DecodeBox(&mem)
		boxIndex, depositSlot, boxOK, err := chooseCompatibleBoxMon(romData, party, box, target, required)
		if err != nil {
			return fmt.Errorf("skill: RepairFieldCapabilities: plan PC recovery for %s: %w", target, err)
		}
		if boxOK {
			if party.Count >= gen1PartyCapacity {
				if err := DepositPartyMon(m, romData, policy, depositSlot); err != nil {
					return fmt.Errorf("skill: RepairFieldCapabilities: make party room for boxed %s user: %w", target, err)
				}
			}
			if err := WithdrawBoxMon(m, romData, policy, boxIndex); err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: withdraw %s-compatible Pokemon from box index %d: %w", target, boxIndex, err)
			}
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: teach %s after withdraw: %w", target, err)
			}
			continue
		}

		candidate, wildOK, err := findWildFieldCandidate(m, romData, target, required)
		if err != nil {
			return fmt.Errorf("skill: RepairFieldCapabilities: find wild %s carrier: %w", target, err)
		}
		if !wildOK {
			return fmt.Errorf("%w: %s has no compatible current-party member, active-box member, or reachable known grass species", ErrFieldRosterNoRecovery, target)
		}

		state.Snapshot(m, &mem)
		party = state.DecodeParty(&mem)
		incoming := state.Mon{Species: candidate.Species}
		depositSlot, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
		if err != nil {
			return fmt.Errorf("skill: RepairFieldCapabilities: plan party room for wild species %#02x: %w", candidate.Species, err)
		}
		if !legal {
			return fmt.Errorf("%w: adding wild species %#02x would strand another required field move", ErrFieldRosterNoRecovery, candidate.Species)
		}
		if party.Count >= gen1PartyCapacity {
			if err := DepositPartyMon(m, romData, policy, depositSlot); err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: make room for wild species %#02x: %w", candidate.Species, err)
			}
		}

		state.Snapshot(m, &mem)
		_, balls := bagEntry(&mem, ItemPokeBall)
		if balls <= 0 {
			return fmt.Errorf("%w: compatible wild species %#02x exists on map %#04x but no POKE BALL is available", ErrFieldRosterCatch, candidate.Species, candidate.Map)
		}
		if _, err := TravelFlee(m, romData, candidate.Destination, policy, pcTravelBattles); err != nil {
			return fmt.Errorf("%w: reach map %#04x for species %#02x: %v", ErrFieldRosterCatch, candidate.Map, candidate.Species, err)
		}
		result, err := Catch(m, romData, []uint8{candidate.Species}, policy, minInt(balls, 10))
		if err != nil {
			return fmt.Errorf("%w: catch species %#02x for %s: %v", ErrFieldRosterCatch, candidate.Species, target, err)
		}
		if result.Outcome != OutcomeCaught || result.Species != candidate.Species {
			return fmt.Errorf("%w: species %#02x for %s ended with outcome %d after %d balls", ErrFieldRosterCatch, candidate.Species, target, result.Outcome, result.BallsThrown)
		}
		if _, err := EnsureFieldMove(m, target); err != nil {
			return fmt.Errorf("skill: RepairFieldCapabilities: teach %s after catching species %#02x: %w", target, candidate.Species, err)
		}
	}

	var after state.Mem
	state.Snapshot(m, &after)
	for _, move := range required {
		cap := FieldCapabilityFor(&after, move)
		if !cap.Usable {
			return fmt.Errorf("skill: RepairFieldCapabilities: final invariant failed for %s: badge=%v HM=%v learned=%v slot=%d", cap.Name, cap.BadgeOwned, cap.HMOwned, cap.Learned, cap.PartySlot)
		}
	}
	return nil
}

// RepairOwnedCoreFieldCapabilities is the story-facing convenience entrypoint:
// preserve every Cut/Surf/Strength capability this save has already unlocked.
func RepairOwnedCoreFieldCapabilities(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return RepairFieldCapabilities(m, romData, policy, OwnedCoreProgressionFieldMoves(&mem))
}
