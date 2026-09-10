package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	cinnabarIslandMap       uint8 = 0x08
	pokemonMansion1FMap     uint8 = 0xa5
	cinnabarPokemonCenterMap uint8 = 0xab
	pokemonMansion2FMap     uint8 = 0xd6
	pokemonMansion3FMap     uint8 = 0xd7
	pokemonMansionB1FMap    uint8 = 0xd8

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
	mansion1FSwitch = mansionSwitchSpec{Map: pokemonMansion1FMap, TargetX: 5, TargetY: 2, StandX: 5, StandY: 3}
	mansion2FSwitch = mansionSwitchSpec{Map: pokemonMansion2FMap, TargetX: 11, TargetY: 2, StandX: 11, StandY: 3}
	mansion3FSwitch = mansionSwitchSpec{Map: pokemonMansion3FMap, TargetX: 5, TargetY: 10, StandX: 5, StandY: 11}
	mansionB1FSwitches = []mansionSwitchSpec{
		{Map: pokemonMansionB1FMap, TargetX: 3, TargetY: 20, StandX: 3, StandY: 21},
		{Map: pokemonMansionB1FMap, TargetX: 25, TargetY: 18, StandX: 25, StandY: 19},
	}
	mansion1FTo2FWarp = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion1FMap, To: pokemonMansion2FMap, WarpX: 5, WarpY: 10}
	mansion2FTo3FWarp = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion2FMap, To: pokemonMansion3FMap, WarpX: 7, WarpY: 10}
	mansion1FToB1FWarp = world.Edge{Kind: world.EdgeWarp, From: pokemonMansion1FMap, To: pokemonMansionB1FMap, WarpX: 21, WarpY: 23}
	mansionDropHoles = [][2]uint8{{16, 14}, {17, 14}}
)

func init() {
	places["cinnabar island"] = Destination{Map: cinnabarIslandMap, X: 11, Y: 12}
	places["pokemon mansion"] = Destination{Map: pokemonMansion1FMap, X: 5, Y: 26}
	places["cinnabar pokemon center"] = Destination{Map: cinnabarPokemonCenterMap, X: 3, Y: 4}
}

// CinnabarSecretKeyOwned is the durable story postcondition for the first #35
// phase. The semantic decoder derives it from the current bag, so a resumed
// checkpoint never depends on remembering whether the pickup dialogue played.
func CinnabarSecretKeyOwned(mem *state.Mem) bool {
	return state.DecodeStoryFacts(mem, state.DecodeInventory(mem)).SecretKeyOwned
}

// CinnabarSecretKeyReady makes the #34 -> #35 handoff explicit. Reaching the
// Mansion is only offered after the complete Saffron slice (Silph rescue plus
// Marsh Badge), while FuchsiaProgressionComplete proves the run owns Surf and
// the Soul Badge needed to use it.
func CinnabarSecretKeyReady(mem *state.Mem) bool {
	facts := state.DecodeStoryFacts(mem, state.DecodeInventory(mem))
	progress := state.DecodeProgress(mem)
	return facts.FuchsiaProgressionComplete && facts.SilphRescueComplete && progress.Has(state.BadgeMarsh)
}

// AcquireCinnabarSecretKey reaches Cinnabar by the supported Pallet -> Route
// 21 Surf corridor, enters Pokemon Mansion, follows live switch-driven
// collision, takes the required 3F fall, and collects the basement Secret Key.
//
// The operation is resumable from any ordinary checkpoint. It reconstructs
// switch state and the key postcondition from RAM on every call; it never
// assumes a remembered statue sequence or patches immutable collision data.
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
		// Force the supported sea route instead of allowing the global graph to
		// choose the Seafoam/Route 20 side. Route 21 already owns a semantic
		// Surf transition that positively enters surfing mode from live RAM.
		if _, err := TravelFlee(m, romData, Destination{Map: semanticPalletTownMap, X: 5, Y: 6}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: reach Pallet for Route 21: %w", err)
		}
		if _, err := TravelFlee(m, romData, Destination{Map: cinnabarIslandMap, X: 11, Y: 12}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: Surf Route 21 to Cinnabar: %w", err)
		}
	}

	if !isPokemonMansionMap(m.Peek8(sym.CurMap)) {
		if _, err := TravelFlee(m, romData, Destination{Map: pokemonMansion1FMap, X: 5, Y: 26}, policy, mansionTravelBattles); err != nil {
			return fmt.Errorf("skill: AcquireCinnabarSecretKey: enter Pokemon Mansion: %w", err)
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
	if mansionTileReachable(m, romData, edge.WarpX, edge.WarpY) {
		return nil
	}
	if m.Peek8(sym.CurMap) != sw.Map {
		return fmt.Errorf("switch for map %#04x requested while on %#04x", sw.Map, m.Peek8(sym.CurMap))
	}
	if !mansionTileReachable(m, romData, sw.StandX, sw.StandY) {
		return fmt.Errorf("neither warp (%d,%d) nor switch stand (%d,%d) is reachable on map %#04x",
			edge.WarpX, edge.WarpY, sw.StandX, sw.StandY, sw.Map)
	}
	if err := setMansionSwitch(m, romData, sw, !currentMansionSwitchOn(m), policy); err != nil {
		return err
	}
	if !mansionTileReachable(m, romData, edge.WarpX, edge.WarpY) {
		return fmt.Errorf("warp (%d,%d) remains unreachable after toggling Mansion switch", edge.WarpX, edge.WarpY)
	}
	return nil
}

func dropMansion3FTo1F(m *emu.Emu, romData []byte, policy MovePolicy) error {
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

	// Do not call Face here. Face is allowed to walk onto a passable tile;
	// for a dungeon-warp hole that would start the fall before this function
	// has armed its positive destination check.
	m.Press(btn)
	crossed := false
	for i := 0; i < mansionDropBudget; i++ {
		if m.Peek8(sym.CurMap) != pokemonMansion3FMap {
			crossed = true
			break
		}
		m.StepFrame()
	}
	m.Release(btn)
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

// setMansionSwitch uses the ROM's semantic YES/NO surface and verifies the
// global EVENT_MANSION_SWITCH_ON state afterward. The switches are hidden
// events that only fire while facing up, so every spec includes its exact
// south-side standing tile instead of allowing a generic adjacent approach.
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
	if px, py := playerXY(m); px != sw.StandX || py != sw.StandY {
		return fmt.Errorf("facing Mansion switch moved player off required stand to (%d,%d)", px, py)
	}
	m.Tap(emu.A, 3, 7)

	answered := false
	for frame := 0; frame < mansionSwitchDriveBudget; frame++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
		if answered && facts.MansionSwitchOn == want && state.Controllable(&mem) {
			// Give the current-map callback a frame to apply the event-driven
			// ReplaceTileBlock operations before the next reachability query.
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
