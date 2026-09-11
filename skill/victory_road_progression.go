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
	route22Map               uint8 = 0x21
	route23Map               uint8 = 0x22
	indigoPlateauMap         uint8 = 0x09
	indigoPlateauLobbyMap    uint8 = 0xAE
	route23NorthCaveY              = 31
	victoryRoadTravelBattles       = 64
	victoryRoadWarpBudget          = 1800
)

var route23SurfBarrierRows = [...]int{101, 92, 81}

var (
	route22RivalApproach = Destination{Map: route22Map, X: 28, Y: 5}
	route23SouthEntry    = Destination{Map: route23Map, X: 7, Y: 138}
	victoryRoad1FEntry   = Destination{Map: victoryRoad1FMap, X: 8, Y: 17}
	victoryRoad2FEntry   = Destination{Map: victoryRoad2FMap, X: 0, Y: 8}
	victoryRoad3FEntry   = Destination{Map: victoryRoad3FMap, X: 23, Y: 7}
	indigoLobbyNurse     = Destination{Map: indigoPlateauLobbyMap, X: 7, Y: 6}
)

func currentStoryFacts(m *emu.Emu) state.StoryFacts {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
}

func inVictoryRoad(mapID uint8) bool {
	return mapID == victoryRoad1FMap || mapID == victoryRoad2FMap || mapID == victoryRoad3FMap
}

func route23SurfCell(g *world.Grid, x, y int) bool {
	if tile, ok := g.FieldTile(x, y); ok && tile == surfWaterTile {
		return true
	}
	tile, ok := g.Tile(x, y)
	return ok && tile == surfWaterTile
}

type route23SurfPlan struct {
	destination Destination
	stand       world.Point
	water       world.Point
	steps       int
}

// planRoute23SurfBand finds a real shoreline entry and a walkable landing on
// the north side of one of Route 23's measured full-width water barriers. The
// path is planned on the same live TraversalWater grid normal navigation uses;
// no input sequence or fixed X coordinate is encoded here.
func planRoute23SurfBand(m *emu.Emu, romData []byte, barrierY int) (route23SurfPlan, error) {
	if got := m.Peek8(sym.CurMap); got != route23Map {
		return route23SurfPlan{}, fmt.Errorf("skill: Route23 Surf planner is on map %#02x, want %#02x", got, route23Map)
	}
	h, err := rom.ParseMap(romData, route23Map)
	if err != nil {
		return route23SurfPlan{}, fmt.Errorf("skill: Route23 Surf parse map: %w", err)
	}
	land, err := liveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return route23SurfPlan{}, fmt.Errorf("skill: Route23 Surf land grid: %w", err)
	}
	water, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return route23SurfPlan{}, fmt.Errorf("skill: Route23 Surf water grid: %w", err)
	}

	sx, sy := playerXY(m)
	blocked := spriteBlockers(m)
	best := route23SurfPlan{steps: -1}
	for delta := 1; delta <= 16; delta++ {
		y := barrierY - delta
		if y < 0 {
			break
		}
		for x := 0; x < land.Width; x++ {
			if !land.Walkable(x, y) || blocked[[2]int{x, y}] {
				continue
			}
			path, err := world.FindPath(water, int(sx), int(sy), x, y, blocked)
			if err != nil {
				continue
			}

			px, py := int(sx), int(sy)
			stand := world.Point{}
			entry := world.Point{}
			foundWater := false
			for _, step := range path {
				nx, ny := px+step.DX, py+step.DY
				if route23SurfCell(water, nx, ny) && !route23SurfCell(water, px, py) {
					stand = world.Point{X: px, Y: py}
					entry = world.Point{X: nx, Y: ny}
					foundWater = true
					break
				}
				px, py = nx, ny
			}
			if !foundWater {
				continue
			}
			if best.steps >= 0 && len(path) >= best.steps {
				continue
			}
			best = route23SurfPlan{
				destination: Destination{Map: route23Map, X: uint8(x), Y: uint8(y)},
				stand:       stand,
				water:       entry,
				steps:       len(path),
			}
		}
	}
	if best.steps < 0 {
		return route23SurfPlan{}, fmt.Errorf("skill: Route23 Surf found no live water route north of barrier row %d from (%d,%d)", barrierY, sx, sy)
	}
	return best, nil
}

func crossRoute23SurfBandNorth(m *emu.Emu, romData []byte, policy MovePolicy, barrierY int) error {
	if got := m.Peek8(sym.CurMap); got != route23Map {
		return fmt.Errorf("skill: Route23 Surf barrier %d started on map %#02x", barrierY, got)
	}
	_, y := playerXY(m)
	if int(y) < barrierY {
		return nil
	}

	plan, err := planRoute23SurfBand(m, romData, barrierY)
	if err != nil {
		return err
	}
	if m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		stand := Destination{Map: route23Map, X: uint8(plan.stand.X), Y: uint8(plan.stand.Y)}
		if _, err := TravelFlee(m, romData, stand, policy, victoryRoadTravelBattles); err != nil {
			return fmt.Errorf("skill: Route23 Surf reach barrier %d shoreline: %w", barrierY, err)
		}
		if err := Face(m, uint8(plan.water.X), uint8(plan.water.Y)); err != nil {
			return fmt.Errorf("skill: Route23 Surf face water at (%d,%d): %w", plan.water.X, plan.water.Y, err)
		}
		m.StepFrames(2)
		result, err := UseFieldMove(m, FieldSurf)
		if err != nil {
			return fmt.Errorf("skill: Route23 Surf enter mode at barrier %d: %w", barrierY, err)
		}
		if !result.Surfing || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
			return fmt.Errorf("skill: Route23 Surf barrier %d did not enter Surf mode", barrierY)
		}
	}
	if _, err := TravelFlee(m, romData, plan.destination, policy, victoryRoadTravelBattles); err != nil {
		return fmt.Errorf("skill: Route23 Surf cross barrier %d: %w", barrierY, err)
	}
	if gotMap := m.Peek8(sym.CurMap); gotMap != route23Map {
		return fmt.Errorf("skill: Route23 Surf barrier %d left Route 23 for map %#02x", barrierY, gotMap)
	}
	_, gotY := playerXY(m)
	if int(gotY) >= barrierY {
		return fmt.Errorf("skill: Route23 Surf barrier %d postcondition failed: y=%d", barrierY, gotY)
	}
	return nil
}

func resolveRoute22LeagueRival(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if currentStoryFacts(m).Route22RivalResolved {
		return nil
	}
	if _, err := TravelFlee(m, romData, route22RivalApproach, policy, victoryRoadTravelBattles); err != nil {
		return fmt.Errorf("skill: VictoryRoadProgression: Route 22 rival: %w", err)
	}
	if !currentStoryFacts(m).Route22RivalResolved {
		return fmt.Errorf("skill: VictoryRoadProgression: Route 22 rival battle did not set its completion event")
	}
	return nil
}

func descendVictoryRoad3FHole(m *emu.Emu, romData []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if mem.U8(sym.CurMap) == victoryRoad2FMap && state.HasEvent(&mem, eventVictoryRoad3BoulderInHole) {
		return nil
	}
	if mem.U8(sym.CurMap) != victoryRoad3FMap {
		return fmt.Errorf("skill: Victory Road hole descent is on map %#02x, want 3F %#02x", mem.U8(sym.CurMap), victoryRoad3FMap)
	}
	if !state.HasEvent(&mem, eventVictoryRoad3BoulderInHole) {
		return fmt.Errorf("skill: Victory Road hole descent requested before the 3F boulder reached the hole")
	}

	h, err := rom.ParseMap(romData, victoryRoad3FMap)
	if err != nil {
		return fmt.Errorf("skill: Victory Road hole parse 3F: %w", err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return fmt.Errorf("skill: Victory Road hole live grid: %w", err)
	}
	px, py := playerXY(m)
	path, _, err := world.FindPathAdjacent(grid, int(px), int(py), 23, 15, spriteBlockers(m))
	if err != nil {
		return fmt.Errorf("skill: Victory Road hole has no live approach path: %w", err)
	}
	ax, ay := int(px), int(py)
	for _, step := range path {
		ax += step.DX
		ay += step.DY
	}
	if _, err := TravelFlee(m, romData, Destination{Map: victoryRoad3FMap, X: uint8(ax), Y: uint8(ay)}, policy, victoryRoadTravelBattles); err != nil {
		return fmt.Errorf("skill: Victory Road reach 3F hole edge: %w", err)
	}
	px, py = playerXY(m)
	step := world.Step{DX: 23 - int(px), DY: 15 - int(py)}
	btn, ok := buttonFor(step)
	if !ok {
		return fmt.Errorf("skill: Victory Road hole approach ended at (%d,%d), not adjacent to (23,15)", px, py)
	}
	m.Tap(btn, 3, 7)
	for spent := 0; spent <= victoryRoadWarpBudget; spent += 10 {
		state.Snapshot(m, &mem)
		if mem.U8(sym.CurMap) == victoryRoad2FMap && state.Controllable(&mem) {
			return nil
		}
		if battle := state.DecodeBattle(&mem); battle != nil {
			return fmt.Errorf("skill: Victory Road hole descent unexpectedly entered battle")
		}
		m.StepFrames(10)
	}
	return fmt.Errorf("skill: Victory Road 3F hole did not land on 2F within %d frames", victoryRoadWarpBudget)
}

func clearVictoryRoad(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for phase := 0; phase < 10; phase++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		switch mem.U8(sym.CurMap) {
		case victoryRoad1FMap:
			if _, err := SolveVictoryRoadBoulderSection(m, romData, policy, VictoryRoad1FSwitch); err != nil {
				return fmt.Errorf("skill: Victory Road 1F switch: %w", err)
			}
			if _, err := TravelFlee(m, romData, victoryRoad2FEntry, policy, victoryRoadTravelBattles); err != nil {
				return fmt.Errorf("skill: Victory Road reach 2F: %w", err)
			}

		case victoryRoad2FMap:
			if state.HasEvent(&mem, eventVictoryRoad3BoulderInHole) {
				if _, err := SolveVictoryRoadBoulderSection(m, romData, policy, VictoryRoad2FSwitch2); err != nil {
					return fmt.Errorf("skill: Victory Road 2F east switch: %w", err)
				}
				return nil
			}
			if _, err := SolveVictoryRoadBoulderSection(m, romData, policy, VictoryRoad2FSwitch1); err != nil {
				return fmt.Errorf("skill: Victory Road 2F west switch: %w", err)
			}
			if _, err := TravelFlee(m, romData, victoryRoad3FEntry, policy, victoryRoadTravelBattles); err != nil {
				return fmt.Errorf("skill: Victory Road reach 3F: %w", err)
			}

		case victoryRoad3FMap:
			if _, err := SolveVictoryRoadBoulderSection(m, romData, policy, VictoryRoad3FSwitch); err != nil {
				return fmt.Errorf("skill: Victory Road 3F switch: %w", err)
			}
			if _, err := SolveVictoryRoadBoulderSection(m, romData, policy, VictoryRoad3FHole); err != nil {
				return fmt.Errorf("skill: Victory Road 3F hole: %w", err)
			}
			if err := descendVictoryRoad3FHole(m, romData, policy); err != nil {
				return err
			}

		case route23Map, indigoPlateauMap, indigoPlateauLobbyMap:
			return nil
		default:
			return fmt.Errorf("skill: Victory Road progression left the cave on unexpected map %#02x", mem.U8(sym.CurMap))
		}
	}
	return fmt.Errorf("skill: Victory Road progression exceeded its bounded phase count")
}

func prepareIndigoLobby(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if _, err := TravelFlee(m, romData, indigoLobbyNurse, policy, victoryRoadTravelBattles); err != nil {
		return fmt.Errorf("skill: VictoryRoadProgression: reach Indigo Plateau lobby: %w", err)
	}
	if got := m.Peek8(sym.CurMap); got != indigoPlateauLobbyMap {
		return fmt.Errorf("skill: VictoryRoadProgression: expected Indigo lobby %#02x, observed %#02x", indigoPlateauLobbyMap, got)
	}
	if err := Heal(m); err != nil {
		return fmt.Errorf("skill: VictoryRoadProgression: heal before the League: %w", err)
	}
	return nil
}

// VictoryRoadProgression owns the eight-badge journey from Viridian through
// the final Route 22 rival, Route 23's badge checks and Surf bands, every live
// Victory Road Strength puzzle, and a healed Indigo Plateau lobby checkpoint.
//
// The operation is intentionally resumable. It derives its phase from current
// map/event/RAM state, so a checkpoint in Route 23, on any Victory Road floor,
// after the 3F boulder drop, or just outside Indigo can continue without
// replaying a blind canonical input sequence.
func VictoryRoadProgression(m *emu.Emu, romData []byte, policy MovePolicy) error {
	if policy == nil {
		return fmt.Errorf("skill: VictoryRoadProgression: nil move policy")
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	if facts.LeagueChallengeStarted || facts.LeagueChampionDefeated {
		return nil
	}
	if state.DecodeProgress(&mem).BadgeCount != 8 {
		return fmt.Errorf("%w: Victory Road requires all eight badges", ErrFieldMovePrerequisite)
	}
	if mem.U8(sym.CurMap) == indigoPlateauLobbyMap || mem.U8(sym.CurMap) == indigoPlateauMap {
		return prepareIndigoLobby(m, romData, policy)
	}

	if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldSurf, FieldStrength}); err != nil {
		return fmt.Errorf("skill: VictoryRoadProgression: prepare Surf + Strength: %w", err)
	}

	cur := m.Peek8(sym.CurMap)
	if !inVictoryRoad(cur) {
		if cur == route23Map {
			_, y := playerXY(m)
			if int(y) <= route23NorthCaveY {
				return prepareIndigoLobby(m, romData, policy)
			}
		} else {
			if err := resolveRoute22LeagueRival(m, romData, policy); err != nil {
				return err
			}
			if _, err := TravelFlee(m, romData, route23SouthEntry, policy, victoryRoadTravelBattles); err != nil {
				return fmt.Errorf("skill: VictoryRoadProgression: pass Route 22 gate: %w", err)
			}
		}

		if got := m.Peek8(sym.CurMap); got != route23Map {
			return fmt.Errorf("skill: VictoryRoadProgression: expected Route 23, observed map %#02x", got)
		}
		for _, barrierY := range route23SurfBarrierRows {
			if err := crossRoute23SurfBandNorth(m, romData, policy, barrierY); err != nil {
				return err
			}
		}
		if _, err := TravelFlee(m, romData, victoryRoad1FEntry, policy, victoryRoadTravelBattles); err != nil {
			return fmt.Errorf("skill: VictoryRoadProgression: pass final Route 23 badge checks and enter Victory Road: %w", err)
		}
		facts = currentStoryFacts(m)
		if !facts.Route22RivalResolved {
			return fmt.Errorf("skill: VictoryRoadProgression: Route 22 rival is not positively resolved")
		}
		if !facts.Route23BadgeChecksComplete || facts.Route23BadgeChecksPassed != 7 {
			return fmt.Errorf("skill: VictoryRoadProgression: Route 23 badge checks = %d/7 after cave entry", facts.Route23BadgeChecksPassed)
		}
	}

	if inVictoryRoad(m.Peek8(sym.CurMap)) {
		if err := clearVictoryRoad(m, romData, policy); err != nil {
			return err
		}
	}
	return prepareIndigoLobby(m, romData, policy)
}
