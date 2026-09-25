package skill

import (
	"errors"
	"fmt"
	"math/bits"
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
	ErrFieldRosterNoBalls      = errors.New("skill: catch recovery needs a POKE BALL but none is available")
)

// missingFieldRosterPrerequisite is a stable gameplay blockage: the badge or
// HM that would make a field move preparable is not owned yet. Wrap both
// sentinels so roster diagnostics stay specific while the agent replans
// through the existing field-move prerequisite policy.
func missingFieldRosterPrerequisite(cap FieldCapability) error {
	return fmt.Errorf("%w: %w: %s badge=%v HM=%v", ErrFieldMovePrerequisite, ErrFieldRosterPrerequisite, cap.Name, cap.BadgeOwned, cap.HMOwned)
}

// CoreProgressionFieldMoves is the field-move invariant late-game story
// progression must preserve. Fly and Flash are useful, but Cut/Surf/Strength
// are the moves that can make the remaining mandatory path physically
// impossible when a roster change drops their only user.
func CoreProgressionFieldMoves() []FieldMove {
	return []FieldMove{FieldCut, FieldSurf, FieldStrength}
}

// OwnedCoreProgressionFieldMoves returns the owned core traversal moves the
// current party can actually retain: badge and HM owned, and the whole
// returned set holdable by the party at once. The retention contract exists
// to stop a roster change from dropping the last carrier of an unlocked
// move; a move no party member can learn (a randomized compatibility table,
// a wiped roster) has no carrier to retain, and demanding it would make
// every roster repair report no recovery. The result is the largest jointly
// satisfiable subset, so the final-party invariant stays achievable.
func OwnedCoreProgressionFieldMoves(romData []byte, mem *state.Mem) []FieldMove {
	owned := ownedCoreProgressionFieldMoves(mem)
	mons := state.DecodeParty(mem).Mons
	for size := len(owned); size >= 1; size-- {
		for mask := 1; mask < 1<<len(owned); mask++ {
			if bits.OnesCount8(uint8(mask)) != size {
				continue
			}
			subset := make([]FieldMove, 0, size)
			for i, move := range owned {
				if mask&(1<<uint(i)) != 0 {
					subset = append(subset, move)
				}
			}
			if ok, err := partyCanSatisfyFieldMoves(romData, mons, subset); err == nil && ok {
				return subset
			}
		}
	}
	return nil
}

// ownedCoreProgressionFieldMoves returns the core moves whose HM and badge
// are already owned, before the retainability filter.
func ownedCoreProgressionFieldMoves(mem *state.Mem) []FieldMove {
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
// hold the entire required set at once. It is an assignment search, not a
// greedy first-compatible choice: a flexible mon may need to be reserved for
// Surf while a different mon takes Cut. Each branch writes the planned HM into
// a copy before assigning the next requirement, so four move slots and HM
// permanence are both respected. At most five field moves across six party
// members keeps this search tiny and deterministic.
func partyCanSatisfyFieldMoves(romData []byte, mons []state.Mon, required []FieldMove) (bool, error) {
	planned := append([]state.Mon(nil), mons...)
	var assign func(int, []state.Mon) (bool, error)
	assign = func(next int, current []state.Mon) (bool, error) {
		if next >= len(required) {
			return true, nil
		}
		move := required[next]
		for _, mon := range current {
			if monKnowsRequiredMove(mon, move) {
				return assign(next+1, current)
			}
		}
		for i := range current {
			branch := append([]state.Mon(nil), current...)
			placed, err := placeFieldMoveHypothetically(romData, &branch[i], move)
			if err != nil {
				return false, err
			}
			if !placed {
				continue
			}
			ok, err := assign(next+1, branch)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	return assign(0, planned)
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

// currentMapGrassDestination is a standing tile on the player's map whose
// walkable component contains tall grass. The player's own tile qualifies
// when they already share that component. Otherwise a warp-adjacent tile on
// another component qualifies only when the live route plan can reach it, so
// a gate between two pockets is a habitat and a sealed pocket is not.
func currentMapGrassDestination(romData []byte, planner *RoutePlanner) (Destination, bool, error) {
	if planner == nil {
		return Destination{}, false, nil
	}
	mapID := planner.cur
	grass, grid, err := grassCells(romData, mapID)
	if err != nil || grid == nil || len(grass) == 0 {
		return Destination{}, false, err
	}
	px, py := int(planner.x), int(planner.y)
	if len(grassInPlayerComponent(grass, grid, px, py)) > 0 {
		return Destination{Map: mapID, X: planner.x, Y: planner.y}, true, nil
	}
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return Destination{}, false, err
	}
	best := Destination{}
	bestLen := int(^uint(0) >> 1)
	found := false
	seen := map[[2]int]bool{}
	for _, w := range h.Warps {
		for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			x, y := int(w.X)+d[0], int(w.Y)+d[1]
			key := [2]int{x, y}
			if seen[key] || x < 0 || y < 0 || x > 255 || y > 255 || !grid.Walkable(x, y) {
				continue
			}
			seen[key] = true
			if len(grassInPlayerComponent(grass, grid, x, y)) == 0 {
				continue
			}
			plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
				planner.graph, mapID, mapID, px, py, x, y, nil, planner.prereqs,
			)
			if err != nil {
				continue
			}
			n := len(plan)
			if n == 0 {
				n = 1
			}
			if !found || n < bestLen || (n == bestLen && (y < int(best.Y) || (y == int(best.Y) && x < int(best.X)))) {
				found = true
				bestLen = n
				best = Destination{Map: mapID, X: uint8(x), Y: uint8(y)}
			}
		}
	}
	return best, found, nil
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
	routePlanner, err := NewRoutePlanner(m, romData)
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
		habitat := dest
		if dest.Map == cur {
			// The map's encounter table is not a habitat. Route 16's west
			// Fly-house landing and its Doduo grass are different components;
			// accepting the standing tile made a carrier "reachable" that
			// Catch cannot hunt. A same-map gate hop onto the grass component
			// is a real route, the same one GoTo already plans.
			tile, ok, err := currentMapGrassDestination(romData, routePlanner)
			if err != nil {
				return wildFieldCandidate{}, false, err
			}
			if !ok {
				continue
			}
			habitat = tile
		} else {
			// Plain graph connectivity is not enough here. A Surf repair must not
			// choose a Surf-compatible species whose habitat is itself behind Surf
			// (likewise for Cut/Strength/story gates). Use the same live capability-
			// aware reachability contract as GoTo before using static route length
			// only as a ranking signal.
			if err := routePlanner.Reachability(dest); err != nil {
				continue
			}
		}
		route, err := world.FindRoute(g, cur, habitat.Map)
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
			candidate := wildFieldCandidate{Destination: habitat, Map: habitat.Map, Species: species.ID, Slots: species.Slots, RouteLen: len(route)}
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

// findGiftFieldCandidate returns a registered gift whose story prerequisites
// hold, which this save has not already claimed, and which can carry target
// without stranding another required move.
func findGiftFieldCandidate(mem *state.Mem, romData []byte, target FieldMove, required []FieldMove) (fieldCarrierGift, bool, error) {
	facts := state.DecodeStoryFacts(mem, state.DecodeInventory(mem))
	party := state.DecodeParty(mem)
	for _, gift := range fieldCarrierGifts {
		if !gift.Ready(facts) || giftPokemonAlreadyOwned(mem, romData, gift.Species) {
			continue
		}
		incoming := state.Mon{Species: gift.Species}
		canTarget, err := monCanPlaceFieldMove(romData, incoming, target)
		if err != nil {
			return fieldCarrierGift{}, false, err
		}
		if !canTarget {
			continue
		}
		_, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
		if err != nil {
			return fieldCarrierGift{}, false, err
		}
		if legal {
			return gift, true, nil
		}
	}
	return fieldCarrierGift{}, false, nil
}

// RepairFieldCapabilities makes every required move usable by the current
// party. It first teaches within the existing roster, then tries the active PC
// box, then acquires a ROM-compatible wild species from a reachable known
// grass map, and finally claims a ready registered gift carrier. Every roster mutation goes through the real PC/catch UI and
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
			return missingFieldRosterPrerequisite(cap)
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
			// A single-door room hides every outdoor habitat from component
			// routing. Take that door once and search from the landing; the
			// door is the only legal first action, the same rule Center
			// recovery already uses.
			g, gerr := cachedRouteGraph(romData)
			if gerr != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: find wild %s carrier: %w", target, gerr)
			}
			left, leaveErr := leaveMandatoryWarpRoom(m, romData, g)
			if leaveErr != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: leave room to seek %s carrier: %w", target, leaveErr)
			}
			if left {
				candidate, wildOK, err = findWildFieldCandidate(m, romData, target, required)
				if err != nil {
					return fmt.Errorf("skill: RepairFieldCapabilities: find wild %s carrier: %w", target, err)
				}
			}
		}
		if !wildOK {
			state.Snapshot(m, &mem)
			gift, giftOK, err := findGiftFieldCandidate(&mem, romData, target, required)
			if err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: find gift %s carrier: %w", target, err)
			}
			if !giftOK {
				return fmt.Errorf("%w: %s has no compatible current-party member, active-box member, reachable known grass species, or ready gift", ErrFieldRosterNoRecovery, target)
			}
			party = state.DecodeParty(&mem)
			// The gift lands in the box when the party is full, so make room first.
			if party.Count >= gen1PartyCapacity {
				depositSlot, _, err := chooseDepositSlotForIncoming(romData, party, state.Mon{Species: gift.Species}, required)
				if err != nil {
					return fmt.Errorf("skill: RepairFieldCapabilities: plan party room for gift species %#02x: %w", gift.Species, err)
				}
				if err := DepositPartyMon(m, romData, policy, depositSlot); err != nil {
					return fmt.Errorf("skill: RepairFieldCapabilities: make room for gift species %#02x: %w", gift.Species, err)
				}
			}
			result, err := gift.Receive(m, romData, policy)
			if err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: receive gift species %#02x for %s: %w", gift.Species, target, err)
			}
			if result.Outcome != OutcomeCaught || result.Species != gift.Species {
				return fmt.Errorf("%w: gift species %#02x for %s ended with outcome %d", ErrFieldRosterNoRecovery, gift.Species, target, result.Outcome)
			}
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairFieldCapabilities: teach %s after receiving gift species %#02x: %w", target, gift.Species, err)
			}
			continue
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
		balls := wildBallCount(&mem)
		if balls <= 0 {
			return fmt.Errorf("%w: compatible wild species %#02x exists on map %#04x but no POKE BALL is available", ErrFieldRosterNoBalls, candidate.Species, candidate.Map)
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
	return RepairFieldCapabilities(m, romData, policy, OwnedCoreProgressionFieldMoves(romData, &mem))
}
