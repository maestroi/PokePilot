package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowFishingAttemptCap = 32
	yellowFishingUseBudget  = 9000
)

var (
	ErrYellowFishingExhausted = errors.New("yellow fishing: rod attempts exhausted")
	ErrYellowFishingNoFish    = errors.New("yellow fishing: selected rod has no fish on this map")
)

func yellowRodCanCatch(romData []byte, mapID, rod, species uint8) (bool, error) {
	encounters, err := yellowrom.FishingEncounters(romData)
	if err != nil {
		return false, err
	}
	for _, enc := range encounters {
		if enc.Rod != rod || enc.Species != species {
			continue
		}
		if enc.Global || enc.MapID == mapID {
			return true, nil
		}
	}
	return false, nil
}

func castYellowRodOnce(m *emu.Emu, romData []byte, rod uint8) (bool, error) {
	if rod != yellowrom.OldRodItem && rod != yellowrom.GoodRodItem && rod != yellowrom.SuperRodItem {
		return false, fmt.Errorf("yellow fishing: item %#02x is not a rod", rod)
	}
	idx, qty := yellowBagEntry(m, rod)
	if idx < 0 || qty <= 0 {
		return false, fmt.Errorf("yellow fishing: rod %#02x is not in the bag", rod)
	}
	if m.Peek8(sym.WalkBikeSurfState) == 2 {
		return false, fmt.Errorf("yellow fishing: cannot fish while surfing")
	}
	if err := openYellowBag(m, romData); err != nil {
		return false, fmt.Errorf("yellow fishing: open bag: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return false, fmt.Errorf("yellow fishing: select rod: %w", err)
	}
	if _, err := m.StepUntil(900, yellowUseTossPrompt); err != nil {
		return false, fmt.Errorf("yellow fishing: USE/TOSS prompt did not appear")
	}
	if err := selectYellowLinearMenuItem(m, 0); err != nil {
		return false, fmt.Errorf("yellow fishing: select USE: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	start := m.FrameCount()
	actionStarted := false
	for int(m.FrameCount()-start) <= yellowFishingUseBudget {
		if m.Peek8(sym.IsInBattle) != 0 {
			return true, nil
		}
		if yellowUseTossPrompt(m) {
			m.Tap(emu.A, 3, 7)
			continue
		}
		actionStarted = true
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return false, fmt.Errorf("yellow fishing: unexpected choice while resolving rod use: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if actionStarted && int(m.FrameCount()-start) >= 120 {
			ready, err := yellowprofileObservation(m, romData)
			if err == nil && ready {
				switch m.Peek8(sym.RodResponse) {
				case 2:
					return false, ErrYellowFishingNoFish
				case 0:
					return false, nil
				case 1:
					// A bite should transition into battle after the fishing
					// animation; give the script more frames.
				}
			}
		}
		m.StepFrames(4)
	}
	return false, fmt.Errorf("yellow fishing: rod result exceeded %d frames", yellowFishingUseBudget)
}

// Fish repeatedly casts one owned Yellow rod from the nearest reachable
// shoreline until the requested species bites, then delegates only the ball
// phase to the verified Yellow catcher.
func Fish(m *emu.Emu, romData []byte, rod, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow fishing: nil emulator")
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}
	mapID := m.Peek8(sym.CurMap)
	ok, err := yellowRodCanCatch(romData, mapID, rod, species)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, fmt.Errorf("yellow fishing: rod %#02x cannot produce species %#02x on map %#02x", rod, species, mapID)
	}

	shore, err := nearestYellowShore(m, romData)
	if err != nil {
		return result, err
	}
	if err := walkTo(m, romData, shore.standX, shore.standY, nil); err != nil {
		return result, fmt.Errorf("yellow fishing: reach shoreline: %w", err)
	}

	for attempt := 1; attempt <= yellowFishingAttemptCap; attempt++ {
		if err := faceYellowCoordinate(m, shore.waterX, shore.waterY); err != nil {
			return result, fmt.Errorf("yellow fishing: attempt %d face water: %w", attempt, err)
		}
		bite, err := castYellowRodOnce(m, romData, rod)
		if errors.Is(err, ErrYellowFishingNoFish) {
			return result, err
		}
		if err != nil {
			return result, fmt.Errorf("yellow fishing: attempt %d: %w", attempt, err)
		}
		if !bite {
			continue
		}
		enemy, err := yellowWaitEnemySpecies(m)
		if err != nil {
			return result, err
		}
		result.Encounters++
		if enemy != species {
			outcome, err := Battle(m, romData)
			if err != nil {
				return result, fmt.Errorf("yellow fishing: unwanted species %#02x: %w", enemy, err)
			}
			if outcome.Outcome == BattleOutcomeLost {
				return result, fmt.Errorf("yellow fishing: blacked out during hunt")
			}
			continue
		}

		for thrown := 0; thrown < yellowCatchBallBudget; thrown++ {
			caught, ended, err := throwYellowBall(m, romData, species, false)
			if errors.Is(err, ErrYellowCatchOutOfBalls) {
				return result, err
			}
			if err != nil {
				return result, err
			}
			result.BallsThrown++
			if caught {
				result.Caught = true
				result.Species = species
				return result, nil
			}
			if ended {
				break
			}
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			if _, err := Battle(m, romData); err != nil {
				return result, fmt.Errorf("yellow fishing: resolve uncaught target: %w", err)
			}
		}
	}

	return result, fmt.Errorf("%w: rod=%#02x map=%#02x encounters=%d",
		ErrYellowFishingExhausted, rod, mapID, result.Encounters)
}
