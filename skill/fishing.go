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
	itemOldRod   uint8 = 0x4c
	itemGoodRod  uint8 = 0x4d
	itemSuperRod uint8 = 0x4e

	fishingAttemptCap = 32
	fishingUseBudget  = 9000
	fishingTravelCap  = 20
)

var (
	ErrFishingHuntExhausted = errors.New("skill: Fish: rod attempts exhausted without a wanted species")
	ErrFishingNoShoreline   = errors.New("skill: Fish: no reachable fishing shoreline on the current map")
	ErrFishingNoFishHere    = errors.New("skill: Fish: the selected rod has no fish on this map")
)

type fishingShore struct {
	standX, standY int
	waterX, waterY int
	distance       int
}

// Fish repeatedly casts one owned rod from a reachable shoreline until a
// wanted species bites, then delegates the actual capture to Catch's verified
// in-battle catcher. It never duplicates ball/nickname/storage handling: the
// same party/box/Pokedex postconditions used by grass catches prove success.
func Fish(m *emu.Emu, romData []byte, rod uint8, want []uint8, policy MovePolicy, maxBalls int) (CatchResult, error) {
	if policy == nil {
		return CatchResult{}, fmt.Errorf("skill: Fish: nil policy")
	}
	if len(want) == 0 {
		return CatchResult{}, fmt.Errorf("skill: Fish: want is empty")
	}
	if maxBalls <= 0 {
		return CatchResult{}, fmt.Errorf("skill: Fish: maxBalls must be > 0, got %d", maxBalls)
	}
	if !fishingRodItem(rod) {
		return CatchResult{}, fmt.Errorf("skill: Fish: item %#02x is not a fishing rod", rod)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return CatchResult{}, fmt.Errorf("skill: Fish: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	if _, qty := bagEntry(&mem, rod); qty <= 0 {
		return CatchResult{}, fmt.Errorf("skill: Fish: %w (rod %#02x)", ErrNotInBag, rod)
	}

	shore, err := nearestFishingShore(m, romData)
	if err != nil {
		return CatchResult{}, err
	}
	if _, err := TravelFlee(m, romData, Destination{
		Map: m.Peek8(sym.CurMap), X: uint8(shore.standX), Y: uint8(shore.standY),
	}, policy, fishingTravelCap); err != nil {
		return CatchResult{}, fmt.Errorf("skill: Fish: reach shoreline (%d,%d): %w", shore.standX, shore.standY, err)
	}
	if err := Face(m, uint8(shore.waterX), uint8(shore.waterY)); err != nil {
		return CatchResult{}, fmt.Errorf("skill: Fish: face water (%d,%d): %w", shore.waterX, shore.waterY, err)
	}
	m.StepFrames(2)

	state.Snapshot(m, &mem)
	partyBefore := int(state.DecodeParty(&mem).Count)
	boxBefore := int(state.DecodeBox(&mem).Count)
	ownedBefore := append([]uint8(nil), state.DecodePokedex(&mem).Owned...)
	wantDex := wantedDexNumbers(romData, want)
	res := CatchResult{}

	for attempt := 1; attempt <= fishingAttemptCap; attempt++ {
		// Battle and menu handling can change facing. Re-prove the cast target
		// immediately before every attempt instead of assuming it stayed put.
		if err := Face(m, uint8(shore.waterX), uint8(shore.waterY)); err != nil {
			return res, fmt.Errorf("skill: Fish: attempt %d face water: %w", attempt, err)
		}
		bite, err := castRodOnce(m, rod)
		if err != nil {
			return res, fmt.Errorf("skill: Fish: attempt %d: %w", attempt, err)
		}
		if !bite {
			continue
		}

		state.Snapshot(m, &mem)
		bs := state.DecodeBattle(&mem)
		if bs == nil {
			return res, fmt.Errorf("skill: Fish: attempt %d reported a bite but no battle is in progress", attempt)
		}
		res.Encounters++
		if !speciesIn(bs.EnemySpecies, want) {
			outcome, battleErr := Battle(m, policy)
			if battleErr != nil {
				return res, fmt.Errorf("skill: Fish: unwanted species %d: %w", bs.EnemySpecies, battleErr)
			}
			if outcome == state.ResultLost {
				return res, ErrCatchBlackout
			}
			continue
		}

		return catchWanted(m, &mem, want, wantDex, policy, partyBefore, boxBefore, ownedBefore, res, maxBalls)
	}
	return res, fmt.Errorf("%w: %d casts and %d encounters on map %#04x", ErrFishingHuntExhausted, fishingAttemptCap, res.Encounters, m.Peek8(sym.CurMap))
}

func fishingRodItem(item uint8) bool {
	return item == itemOldRod || item == itemGoodRod || item == itemSuperRod
}

// castRodOnce owns START -> ITEM -> rod -> USE and the fishing animation/text.
// It returns only after either a battle has started or the no-bite result has
// settled back to the overworld. Unknown prompts are never answered blindly.
func castRodOnce(m *emu.Emu, rod uint8) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		return false, fmt.Errorf("player not controllable")
	}
	idx, _ := bagEntry(&mem, rod)
	if idx < 0 {
		return false, fmt.Errorf("%w (rod %#02x)", ErrNotInBag, rod)
	}

	wantMax, itemIndex := startMenuShape(&mem)
	drawn := func(m *emu.Emu) bool {
		return m.Peek8(sym.FontLoaded) != 0 && int(m.Peek8(sym.MaxMenuItem)) == wantMax
	}
	for attempt := 0; attempt < 5 && !drawn(m); attempt++ {
		m.Tap(emu.Start, 3, 7)
		_, _ = m.StepUntil(startMenuDrawBudget, drawn)
	}
	if !drawn(m) {
		return false, fmt.Errorf("start menu did not draw")
	}
	if err := SelectMenuItem(m, itemIndex); err != nil {
		return false, fmt.Errorf("select ITEM: %w", err)
	}
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == itemListMenuID
	}); err != nil {
		return false, fmt.Errorf("bag list did not open: %w", err)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return false, fmt.Errorf("select rod: %w", err)
	}
	if _, err := m.StepUntil(useTossBudget, func(m *emu.Emu) bool {
		state.Snapshot(m, &mem)
		return useTossPrompt(&mem) != nil
	}); err != nil {
		return false, fmt.Errorf("USE/TOSS prompt did not open: %w", err)
	}
	state.Snapshot(m, &mem)
	if p := useTossPrompt(&mem); p == nil || p.Index != 0 {
		return false, fmt.Errorf("USE/TOSS cursor is not on USE")
	}

	m.Tap(emu.A, 3, 7)
	start := m.FrameCount()
	actionStarted := false
	for int(m.FrameCount()-start) <= fishingUseBudget {
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return true, nil
		}
		if p := useTossPrompt(&mem); p != nil {
			// This exact prompt is known and its cursor was already verified on
			// USE. Menu transitions can leave it visible for a few frames after
			// the first A; re-confirming USE is safe and avoids misclassifying
			// that race as an unknown choice prompt.
			if p.Index != 0 {
				return false, fmt.Errorf("USE/TOSS cursor moved off USE while resolving rod use")
			}
			m.Tap(emu.A, 3, 7)
			continue
		}
		actionStarted = true
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return false, fmt.Errorf("unexpected choice prompt while resolving rod use")
		}
		if actionStarted && (state.DecodeDialogue(&mem) != nil || mem.U8(sym.FontLoaded) != 0) {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if actionStarted && int(m.FrameCount()-start) >= 120 && state.Controllable(&mem) && !state.MenuUp(&mem) && mem.U8(sym.FontLoaded) == 0 {
			// wRodResponse is WRAM scratch at 0xCD3D: 0=no bite, 1=bite,
			// 2=no fish on map. A real bite should already have transitioned
			// into battle above; response 2 is a deterministic bad source.
			if mem.U8(sym.RodResponse) == 2 {
				return false, ErrFishingNoFishHere
			}
			return false, nil
		}
		m.StepFrames(4)
	}
	return false, fmt.Errorf("rod result exceeded %d frames", fishingUseBudget)
}

func nearestFishingShore(m *emu.Emu, romData []byte) (fishingShore, error) {
	mapID := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return fishingShore{}, fmt.Errorf("skill: Fish: parse map %#04x: %w", mapID, err)
	}
	land, err := liveMapGridForTraversal(m, romData, h, world.TraversalLand)
	if err != nil {
		return fishingShore{}, fmt.Errorf("skill: Fish: land grid: %w", err)
	}
	water, err := liveMapGridForTraversal(m, romData, h, world.TraversalWater)
	if err != nil {
		return fishingShore{}, fmt.Errorf("skill: Fish: water grid: %w", err)
	}
	sx, sy := playerXY(m)
	blocked := spriteBlockers(m)
	dirs := [][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}}

	best := fishingShore{distance: int(^uint(0) >> 1)}
	found := false
	for y := 0; y < land.Height; y++ {
		for x := 0; x < land.Width; x++ {
			if !land.Walkable(x, y) {
				continue
			}
			steps, pathErr := world.FindPath(land, int(sx), int(sy), x, y, blocked)
			if pathErr != nil {
				continue
			}
			for _, d := range dirs {
				nx, ny := x+d[0], y+d[1]
				if !water.InBounds(nx, ny) || land.Passable(x, y, nx, ny) || !water.Passable(x, y, nx, ny) {
					continue
				}
				candidate := fishingShore{standX: x, standY: y, waterX: nx, waterY: ny, distance: len(steps)}
				if !found || candidate.distance < best.distance ||
					(candidate.distance == best.distance && (candidate.standY < best.standY ||
						(candidate.standY == best.standY && candidate.standX < best.standX))) {
					best, found = candidate, true
				}
			}
		}
	}
	if !found {
		return fishingShore{}, ErrFishingNoShoreline
	}
	return best, nil
}
