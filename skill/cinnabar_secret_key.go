package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	cinnabarIslandMap        uint8 = 0x08
	pokemonMansion1FMap      uint8 = 0xa5
	cinnabarPokemonCenterMap uint8 = 0xab
	pokemonMansion2FMap      uint8 = 0xd6
	pokemonMansion3FMap      uint8 = 0xd7
	pokemonMansionB1FMap     uint8 = 0xd8

	mansionSecretKeyItem uint8 = 0x2b
	mansionSecretKeyX    uint8 = 5
	mansionSecretKeyY    uint8 = 13

	mansionTravelBattles       = 180
	mansionSwitchDriveBudget   = 5000
	mansionSwitchStandAttempts = 10
	mansionDropBudget          = 1200
)

type mansionSwitchSpec struct {
	Map              uint8
	TargetX, TargetY uint8
	StandX, StandY   uint8
}

var (
	mansion1FSwitch    = mansionSwitchSpec{Map: pokemonMansion1FMap, TargetX: 5, TargetY: 2, StandX: 5, StandY: 3}
	mansion2FSwitch    = mansionSwitchSpec{Map: pokemonMansion2FMap, TargetX: 11, TargetY: 2, StandX: 11, StandY: 3}
	mansion3FSwitch    = mansionSwitchSpec{Map: pokemonMansion3FMap, TargetX: 5, TargetY: 10, StandX: 5, StandY: 11}
	mansionB1FSwitches = []mansionSwitchSpec{
		{Map: pokemonMansionB1FMap, TargetX: 3, TargetY: 20, StandX: 3, StandY: 21},
		{Map: pokemonMansionB1FMap, TargetX: 25, TargetY: 18, StandX: 25, StandY: 19},
	}
	cinnabarToMansionWarp = world.Edge{Kind: world.EdgeWarp, From: cinnabarIslandMap, To: pokemonMansion1FMap, WarpX: 6, WarpY: 3}
	mansion1FTo2FWarp     = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion1FMap, To: pokemonMansion2FMap, WarpX: 5, WarpY: 10}
	mansion2FTo3FWarp     = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion2FMap, To: pokemonMansion3FMap, WarpX: 7, WarpY: 10}
	mansion1FToB1FWarp    = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion1FMap, To: pokemonMansionB1FMap, WarpX: 21, WarpY: 23}
	mansionDropHoles      = [][2]uint8{{16, 14}, {17, 14}}
)

func init() {
	places["cinnabar island"] = Destination{Map: cinnabarIslandMap, X: 11, Y: 12}
	places["pokemon mansion"] = Destination{Map: pokemonMansion1FMap, X: 5, Y: 26}
	places["cinnabar pokemon center"] = Destination{Map: cinnabarPokemonCenterMap, X: 3, Y: 4}
}

func CinnabarSecretKeyOwned(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).SecretKeyOwned
}

func CinnabarSecretKeyReady(mem *state.Mem) bool {
	facts := state.DecodeStoryFacts(mem, state.DecodeInventory(mem))
	progress := state.DecodeProgress(mem)
	return facts.FuchsiaProgressionComplete && facts.SilphRescueComplete && progress.Has(state.BadgeMarsh)
}

func AcquireCinnabarSecretKey(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: nil policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if CinnabarSecretKeyOwned(&mem) {
		return nil
	}
	if !CinnabarSecretKeyReady(&mem) {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: post-Saffron/Surf handoff is not satisfied")
	}

	if !onCinnabarSecretKeySlice(m.Peek8(sym.CurMap)) {
		// The supported story corridor is Pallet -> Route 21 -> Cinnabar.
		// Route 20 is split by Seafoam Islands; treating its two Surf seams as
		// one ordinary cross-map journey makes Secret Key accidentally absorb
		// the unsupported Seafoam traversal/puzzle and stranded #1595 on 0x1f.
		//
		// Earlier #1448 temporarily removed the Pallet waypoint because the
		// semantic router could not cross Surf-only ports. #1551/#1591 fixed
		// those port/band defects, so restore the original #190 contract rather
		// than teaching this milestone to solve Seafoam.
		//
		// Make Fly usable first so a resumed late-game save can deterministically
		// return to Pallet instead of asking the walking graph to choose between
		// Diglett's Cave and the same Seafoam detour. By this stage Thunder,
		// Poké Flute and Cut are already progression prerequisites, so Fly setup
		// is an idempotent repair of an intended speedrun capability.
		state.Snapshot(m, &mem)
		if !FieldCapabilityFor(&mem, FieldFly).Usable {
			if err := PrepareFlyFastTravel(m, romData, policy); err != nil {
				return fmt.Errorf("skill: AcquireCinnabarSecretKey: prepare Fly for Route 21 approach: %w", err)
			}
		}
		if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldSurf, FieldFly}); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: prepare Surf/Fly carriers: %w", err)
		}

		pallet := Destination{Map: semanticPalletTownMap, X: 5, Y: 6}
		if _, err := TravelFlee(m, romData, pallet, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: reach Pallet for Route 21: %w", err)
		}
		if _, err := TravelFlee(m, romData, Destination{Map: cinnabarIslandMap, X: 11, Y: 12}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: Surf Route 21 to Cinnabar: %w", err)
		}
	}

	if !isPokemonMansionMap(m.Peek8(sym.CurMap)) {
		if err := enterPokemonMansion(m, romData, policy); err != nil {
			return err
		}
	}

	if m.Peek8(sym.CurMap) != pokemonMansionB1FMap {
		if err := reachMansionBasement(m, romData, policy); err != nil {
			return err
		}
	}
	if err := solveMansionBasementForKey(m, romData, policy); err != nil {
		return err
	}

	state.Snapshot(m, &mem)
	if !CinnabarSecretKeyOwned(&mem) {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: pickup completed without secret_key_owned semantic postcondition")
	}
	return nil
}

func runMansionFleeRecovery(m *emu.Emu, policy MovePolicy, action func() error) error {
	_, err := travel(
		m,
		policy,
		mansionTravelBattles,
		action,
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fleeThenFight(m, policy, guaranteedWildFleeAttempts),
	)
	return err
}

func mansionInterruptionForTravel(err error) error {
	if errors.Is(err, ErrBattleInterrupted) {
		return ErrBattle
	}
	return err
}

func enterPokemonMansion(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != cinnabarIslandMap {
		// Keep the story handoff local once Cinnabar is reached. A generic
		// cross-map route to Mansion 1F can consider unrelated boundaries on
		// the island (the locked Gym or the southern Surf seam) before the
		// actual Mansion door. That produced repeated secret_key_owned
		// terminal failures while the run was already on map 0x08 (#1618,
		// #1629). Route to a stable island tile first, then traverse the ROM's
		// known Mansion entrance warp directly.
		if _, err := TravelFlee(m, romData, Destination{Map: cinnabarIslandMap, X: 11, Y: 12}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: return to Cinnabar Island for Mansion entrance: %w", err)
		}
	}
	if got := m.Peek8(sym.CurMap); got != cinnabarIslandMap {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: Mansion entrance staging ended on map %#04x, want Cinnabar Island", got)
	}
	// #1634 deliberately replaced generic cross-map routing with the exact
	// Cinnabar Mansion warp. Keep that deterministic story edge, but recover
	// the same incidental dialogue/battle interruptions that TravelFlee used
	// to own. Farm #1636/#1637 showed that a raw Traverse bubbles an
	// ErrDialogueInterrupted straight to the objective budget.
	if err := runMansionFleeRecovery(m, policy, func() error {
		switch got := m.Peek8(sym.CurMap); got {
		case pokemonMansion1FMap:
			// A recovered interruption may let an already-started warp finish
			// before the retry. Treat the positively observed landing as done.
			return nil
		case cinnabarIslandMap:
			return Traverse(m, romData, cinnabarToMansionWarp)
		default:
			return fmt.Errorf("Mansion entrance retry on map %#04x, want Cinnabar Island", got)
		}
	}); err != nil {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: enter Pokemon Mansion through Cinnabar door: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != pokemonMansion1FMap {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: Mansion door landed on map %#04x, want Mansion 1F", got)
	}
	return nil
}

func onCinnabarSecretKeySlice(mapID uint8) bool {
	return mapID == cinnabarIslandMap || mapID == cinnabarPokemonCenterMap || isPokemonMansionMap(mapID)
}

func isPokemonMansionMap(mapID uint8) bool {
	switch mapID {
	case pokemonMansion1FMap, pokemonMansion2FMap, pokemonMansion3FMap, pokemonMansionB1FMap:
		return true
	default:
		return false
	}
}

func currentMansionSwitchOn(m *emu.Emu) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).MansionSwitchOn
}

func reachMansionBasement(m *emu.Emu, romData []byte, policy MovePolicy) error {
	switch m.Peek8(sym.CurMap) {
	case pokemonMansionB1FMap:
		return nil
	case pokemonMansion1FMap:
		if err := ensureMansionWarpReachable(m, romData, mansion1FTo2FWarp, mansion1FSwitch, policy); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: open route to Mansion 2F: %w", err)
		}
		if _, err := TravelFlee(m, romData, Destination{Map: pokemonMansion2FMap, X: 5, Y: 11}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: reach Mansion 2F: %w", err)
		}
		fallthrough
	case pokemonMansion2FMap:
		if err := ensureMansionWarpReachable(m, romData, mansion2FTo3FWarp, mansion2FSwitch, policy); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: open route to Mansion 3F: %w", err)
		}
		if _, err := TravelFlee(m, romData, Destination{Map: pokemonMansion3FMap, X: 7, Y: 11}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: reach Mansion 3F: %w", err)
		}
		fallthrough
	case pokemonMansion3FMap:
		if err := dropMansion3FTo1F(m, romData, policy); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: take Mansion 3F fall: %w", err)
		}
	default:
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: unsupported Mansion resume map %#04x", m.Peek8(sym.CurMap))
	}

	if m.Peek8(sym.CurMap) != pokemonMansion1FMap {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: 3F fall landed on %#04x, want Mansion 1F", m.Peek8(sym.CurMap))
	}
	if err := ensureMansionWarpReachable(m, romData, mansion1FToB1FWarp, mansion1FSwitch, policy); err != nil {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: open route to Mansion B1F: %w", err)
	}
	if _, err := TravelFlee(m, romData, Destination{Map: pokemonMansionB1FMap, X: 23, Y: 21}, policy, mansionTravelBattles); err != nil {
		return fmt.Errorf("skill: AcquireCinnabarSecretKey: reach Mansion B1F: %w", err)
	}
	return nil
}

func ensureMansionWarpReachable(m *emu.Emu, romData []byte, edge world.Edge, sw mansionSwitchSpec, policy MovePolicy) error {
	goalReachable := func() bool {
		return mansionTileReachable(m, romData, edge.WarpX, edge.WarpY)
	}
	if goalReachable() {
		return nil
	}
	if m.Peek8(sym.CurMap) != sw.Map {
		return fmt.Errorf("switch for map %#04x requested while on %#04x", sw.Map, m.Peek8(sym.CurMap))
	}
	if !mansionTileReachable(m, romData, sw.StandX, sw.StandY) {
		return fmt.Errorf("neither warp (%d,%d) nor switch stand (%d,%d) is reachable on map %#04x", edge.WarpX, edge.WarpY, sw.StandX, sw.StandY, sw.Map)
	}

	want := !currentMansionSwitchOn(m)
	// The exact selected warp is the route goal. This prevents a generic
	// "toggle a reachable statue" fallback from mutating unrelated Mansion
	// doors after a path failure.
	_, err := executeTopologyInteraction(m, romData, policy, topologyInteraction{
		Name:          fmt.Sprintf("Mansion switch %02x(%d,%d)", sw.Map, sw.TargetX, sw.TargetY),
		Approach:      Destination{Map: sw.Map, X: sw.StandX, Y: sw.StandY},
		TargetX:       sw.TargetX,
		TargetY:       sw.TargetY,
		MaxBattles:    mansionTravelBattles,
		Budget:        mansionSwitchDriveBudget,
		RouteGoal:     fmt.Sprintf("Mansion warp (%d,%d)", edge.WarpX, edge.WarpY),
		GoalReachable: goalReachable,
		Complete: func(mem *state.Mem) bool {
			facts := state.DecodeStoryFacts(mem, state.DecodeInventory(mem))
			return facts.MansionSwitchOn == want
		},
		Interact: func() error {
			return driveMansionSwitchInteraction(m, sw, want)
		},
	})
	if err != nil {
		return err
	}
	if !goalReachable() {
		return fmt.Errorf("warp (%d,%d) remains unreachable after verified Mansion switch action", edge.WarpX, edge.WarpY)
	}
	return nil
}

func dropMansion3FTo1F(m *emu.Emu, romData []byte, policy MovePolicy) error {
	// The final step onto a dungeon hole is outside TravelFlee: a wild
	// encounter or trainer sightline can steal control after the adjacent-tile
	// approach but before the map script processes the fall. The old loop just
	// stepped 1200 frames and reported a bare "did not change map", which the
	// agent normalized as unknown_failure (#1635, with later recurrences).
	//
	// Run the whole resumable drop attempt through Travel's interruption
	// resolver. After a recovered battle/dialogue we recompute live reachability
	// and approach the hole again; if recovery itself let the pending dungeon
	// warp finish, observing Mansion 1F is already the positive postcondition.
	if err := runMansionFleeRecovery(m, policy, func() error {
		switch got := m.Peek8(sym.CurMap); got {
		case pokemonMansion1FMap:
			return nil
		case pokemonMansion3FMap:
			return dropMansion3FTo1FOnce(m, romData, policy)
		default:
			return fmt.Errorf("Mansion 3F drop retry on map %#04x, want 3F or 1F", got)
		}
	}); err != nil {
		return err
	}
	if got := m.Peek8(sym.CurMap); got != pokemonMansion1FMap {
		return fmt.Errorf("Mansion 3F drop recovery ended on map %#04x, want 1F", got)
	}
	return nil
}

func dropMansion3FTo1FOnce(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != pokemonMansion3FMap {
		return fmt.Errorf("drop requested on map %#04x, want Mansion 3F", m.Peek8(sym.CurMap))
	}
	findHole := func() (uint8, uint8, bool) {
		for _, hole := range mansionDropHoles {
			if mansionTileReachable(m, romData, hole[0], hole[1]) {
				return hole[0], hole[1], true
			}
		}
		return 0, 0, false
	}
	hx, hy, ok := findHole()
	if !ok {
		if !mansionTileReachable(m, romData, mansion3FSwitch.StandX, mansion3FSwitch.StandY) {
			return fmt.Errorf("no 1F drop hole or 3F switch stand is reachable")
		}
		if err := setMansionSwitch(m, romData, mansion3FSwitch, !currentMansionSwitchOn(m), policy); err != nil {
			return err
		}
		hx, hy, ok = findHole()
		if !ok {
			return fmt.Errorf("no 1F drop hole became reachable after Mansion switch toggle")
		}
	}

	dest, move, err := besideDestination(m, romData, hx, hy)
	if err != nil {
		return fmt.Errorf("find side of drop hole (%d,%d): %w", hx, hy, err)
	}
	if move {
		if _, err := TravelFlee(m, romData, dest, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("approach drop hole (%d,%d): %w", hx, hy, err)
		}
	}
	px, py := playerXY(m)
	step, ok := directionTo(px, py, hx, hy)
	if !ok {
		return fmt.Errorf("drop hole (%d,%d) is not adjacent after approach from (%d,%d)", hx, hy, px, py)
	}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("invalid step %s into Mansion drop hole", step)
	}

	m.Press(btn)
	defer m.Release(btn)
	crossed := false
	for i := 0; i < mansionDropBudget; i++ {
		if m.Peek8(sym.CurMap) != pokemonMansion3FMap {
			crossed = true
			break
		}
		if err := mansionInterruptionForTravel(movementInterruption(m)); err != nil {
			return err
		}
		m.StepFrame()
	}
	if !crossed {
		return fmt.Errorf("Mansion drop at (%d,%d) did not change map", hx, hy)
	}
	if m.Peek8(sym.CurMap) != pokemonMansion1FMap {
		return fmt.Errorf("Mansion drop at (%d,%d) landed on map %#04x, want 1F", hx, hy, m.Peek8(sym.CurMap))
	}
	var mem state.Mem
	if _, err := m.StepUntil(mansionDropBudget, func(e *emu.Emu) bool {
		state.Snapshot(e, &mem)
		return state.Controllable(&mem)
	}); err != nil {
		return fmt.Errorf("Mansion 1F landing did not return control")
	}
	return nil
}

func solveMansionBasementForKey(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != pokemonMansionB1FMap {
		return fmt.Errorf("basement solver on map %#04x, want Mansion B1F", m.Peek8(sym.CurMap))
	}
	type attemptKey struct {
		SwitchOn bool
		X, Y     uint8
	}
	attempted := map[attemptKey]bool{}
	for step := 0; step < 6; step++ {
		if mansionTargetReachable(m, romData, mansionSecretKeyX, mansionSecretKeyY) {
			if err := Pickup(m, romData, mansionSecretKeyX, mansionSecretKeyY, mansionSecretKeyItem, policy); err != nil {
				return fmt.Errorf("skill: AcquireCinnabarSecretKey: collect Secret Key: %w", err)
			}
			return nil
		}
		before := currentMansionSwitchOn(m)
		toggled := false
		for _, sw := range mansionB1FSwitches {
			key := attemptKey{SwitchOn: before, X: sw.TargetX, Y: sw.TargetY}
			if attempted[key] || !mansionTileReachable(m, romData, sw.StandX, sw.StandY) {
				continue
			}
			attempted[key] = true
			if err := setMansionSwitch(m, romData, sw, !before, policy); err != nil {
				return fmt.Errorf("skill: AcquireCinnabarSecretKey: toggle B1F statue at (%d,%d): %w", sw.TargetX, sw.TargetY, err)
			}
			toggled = true
			break
		}
		if !toggled {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: Secret Key is unreachable and no untried reachable B1F switch remains")
		}
	}
	return fmt.Errorf("skill: AcquireCinnabarSecretKey: exhausted B1F switch-state search without reaching Secret Key")
}

func mansionTileReachable(m *emu.Emu, romData []byte, tx, ty uint8) bool {
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return false
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false
	}
	sx, sy := playerXY(m)
	_, err = world.FindPath(grid, int(sx), int(sy), int(tx), int(ty), nil)
	return err == nil
}

func mansionTargetReachable(m *emu.Emu, romData []byte, tx, ty uint8) bool {
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return false
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return false
	}
	sx, sy := playerXY(m)
	_, _, err = world.FindPathAdjacent(grid, int(sx), int(sy), int(tx), int(ty), nil)
	return err == nil
}

func setMansionSwitch(m *emu.Emu, romData []byte, sw mansionSwitchSpec, want bool, policy MovePolicy) error {
	if m.Peek8(sym.CurMap) != sw.Map {
		return fmt.Errorf("Mansion switch (%d,%d) belongs to map %#04x, current map %#04x", sw.TargetX, sw.TargetY, sw.Map, m.Peek8(sym.CurMap))
	}
	if currentMansionSwitchOn(m) == want {
		return nil
	}
	arrived := false
	for attempt := 0; attempt < mansionSwitchStandAttempts; attempt++ {
		if _, err := TravelFlee(m, romData, Destination{Map: sw.Map, X: sw.StandX, Y: sw.StandY}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("reach Mansion switch stand (%d,%d): %w", sw.StandX, sw.StandY, err)
		}
		px, py := playerXY(m)
		if px == sw.StandX && py == sw.StandY {
			arrived = true
			break
		}
		m.StepFrames(npcWaitFrames)
	}
	if !arrived {
		px, py := playerXY(m)
		return fmt.Errorf("Mansion switch stand (%d,%d) stayed occupied; stopped at (%d,%d)", sw.StandX, sw.StandY, px, py)
	}
	if err := Face(m, sw.TargetX, sw.TargetY); err != nil {
		return fmt.Errorf("face Mansion switch (%d,%d): %w", sw.TargetX, sw.TargetY, err)
	}
	return driveMansionSwitchInteraction(m, sw, want)
}

func driveMansionSwitchInteraction(m *emu.Emu, sw mansionSwitchSpec, want bool) error {
	if px, py := playerXY(m); px != sw.StandX || py != sw.StandY {
		return fmt.Errorf("Mansion switch (%d,%d) requires stand (%d,%d), at (%d,%d)",
			sw.TargetX, sw.TargetY, sw.StandX, sw.StandY, px, py)
	}

	m.Tap(emu.A, 3, 7)
	answered := false
	for frame := 0; frame < mansionSwitchDriveBudget; frame++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
		if answered && facts.MansionSwitchOn == want && state.Controllable(&mem) {
			m.StepFrames(2)
			return nil
		}
		interaction := state.DecodeInteraction(&mem)
		switch interaction.Kind {
		case state.InteractionTwoOption:
			if answered {
				return fmt.Errorf("Mansion switch opened a second unexpected two-option prompt")
			}
			if err := AnswerYesNo(m, true); err != nil {
				return fmt.Errorf("answer Mansion switch YES: %w", err)
			}
			answered = true
		case state.InteractionDialogue:
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		case state.InteractionNone:
			m.StepFrame()
		default:
			return fmt.Errorf("Mansion switch entered unexpected interaction %q text=%q", interaction.Kind, interaction.Text)
		}
	}
	return fmt.Errorf("Mansion switch (%d,%d) did not reach switch_on=%v within %d frames", sw.TargetX, sw.TargetY, want, mansionSwitchDriveBudget)
}
