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

// CatchWater hunts wild encounters while surfing on the current map, then
// delegates wanted battles to the same verified ball/party/box/Pokedex path as
// ordinary grass catches. Surf itself is prepared through the existing field
// capability contract, so owning HM03 without the Soul Badge (or without a
// compatible current party member) is never treated as sufficient.
func CatchWater(m *emu.Emu, romData []byte, want []uint8, policy MovePolicy, maxBalls int) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: CatchWater: nil policy")
	}
	if len(want) == 0 {
		return CatchResult{}, fmt.Errorf("skill: CatchWater: want is empty")
	}
	if maxBalls <= 0 {
		return CatchResult{}, fmt.Errorf("skill: CatchWater: maxBalls must be > 0, got %d", maxBalls)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return CatchResult{}, fmt.Errorf("skill: CatchWater: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}

	// Collection uses the same positive acquisition evidence as Catch. Record
	// it before entering Surf so teaching HM03 or walking to the shoreline
	// cannot be mistaken for capture progress.
	partyBefore := int(state.DecodeParty(&mem).Count)
	boxBefore := int(state.DecodeBox(&mem).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&mem).Owned...)
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{}

	if m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		shore, err := nearestFishingShore(m, romData)
		if err != nil {
			return res, fmt.Errorf("skill: CatchWater: find Surf shoreline: %w", err)
		}
		if _, err := TravelFlee(m, romData, Destination{
			Map: m.Peek8(sym.CurMap), X: uint8(shore.standX), Y: uint8(shore.standY),
		}, policy, fishingTravelCap); err != nil {
			return res, fmt.Errorf("skill: CatchWater: reach shoreline (%d,%d): %w", shore.standX, shore.standY, err)
		}
		if err := Face(m, uint8(shore.waterX), uint8(shore.waterY)); err != nil {
			return res, fmt.Errorf("skill: CatchWater: face water (%d,%d): %w", shore.waterX, shore.waterY, err)
		}
		m.StepFrames(2)
		result, err := UseFieldMove(m, FieldSurf)
		if err != nil {
			return res, fmt.Errorf("skill: CatchWater: enter Surf: %w", err)
		}
		if !result.Surfing || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
			return res, fmt.Errorf("skill: CatchWater: Surf returned without verified surfing state")
		}
	}

	mapID := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return res, fmt.Errorf("skill: CatchWater: parse map %#04x: %w", mapID, err)
	}
	grid, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return res, fmt.Errorf("skill: CatchWater: water grid: %w", err)
	}
	water := surfEncounterCells(grid)
	if len(water) == 0 {
		return res, fmt.Errorf("skill: CatchWater: map %#04x has no Surf water cells", mapID)
	}

	now := currentWorld(m)
	a, b, ok := grindPair(water, grid, int(now.X), int(now.Y), spriteBlockers(m))
	if !ok {
		return res, fmt.Errorf("skill: CatchWater: map %#04x has no two reachable Surf water cells close enough to hunt between", mapID)
	}

	next := b
	legsSpent := 0
	for res.Encounters < catchHuntCap && legsSpent < catchGrassLegs {
		d := Destination{Map: mapID, X: uint8(next.x), Y: uint8(next.y)}
		if err := GoTo(m, romData, d); err != nil && !errors.Is(err, ErrBattle) {
			// Water sprites can move just like land sprites. Re-pick a reachable
			// pair and charge the failed traversal against the same bounded hunt.
			na, nb, ok := repickGrindPair(m, water, grid, a, b)
			if !ok {
				return res, fmt.Errorf("skill: CatchWater: hunt leg %d: %w", legsSpent+1, err)
			}
			a, b, next = na, nb, nb
			legsSpent++
			continue
		}
		legsSpent++
		next = flip(a, b, next)
		if !waitBattleStart(m, 1000) {
			continue
		}

		state.Snapshot(m, &mem)
		bs := state.DecodeBattle(&mem)
		if bs == nil {
			return res, fmt.Errorf("skill: CatchWater: hunt leg %d reported an encounter but no battle is in progress on map %#04x", legsSpent, m.Peek8(sym.CurMap))
		}
		res.Encounters++
		if !speciesIn(bs.EnemySpecies, want) {
			outcome, err := Battle(m, policy)
			if err != nil {
				return res, fmt.Errorf("skill: CatchWater: non-wanted battle %d (species %d): %w", res.Encounters, bs.EnemySpecies, err)
			}
			if outcome == state.ResultLost {
				return res, ErrCatchBlackout
			}
			continue
		}

		return catchWanted(m, &mem, want, wantDex, policy, partyBefore, boxBefore, ownedBefore, res, maxBalls)
	}
	return res, fmt.Errorf("%w: %d Surf legs and %d encounters (map %#04x)",
		ErrCatchHuntExhausted, legsSpent, res.Encounters, m.Peek8(sym.CurMap))
}

func surfEncounterCells(grid *world.Grid) []cell {
	if grid == nil {
		return nil
	}
	out := make([]cell, 0)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			field, fieldOK := grid.FieldTile(x, y)
			collision, collisionOK := grid.Tile(x, y)
			if (fieldOK && field == surfWaterTile) || (collisionOK && collision == surfWaterTile) {
				if grid.Walkable(x, y) {
					out = append(out, cell{x: x, y: y})
				}
			}
		}
	}
	return out
}
