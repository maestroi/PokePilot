package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowSafariZoneGate   uint8 = 0x9c
	yellowSafariZoneEast   uint8 = 0xd9
	yellowSafariZoneNorth  uint8 = 0xda
	yellowSafariZoneWest   uint8 = 0xdb
	yellowSafariZoneCenter uint8 = 0xdc

	yellowEventSafariGameOver uint16 = 0x24e
	yellowEventInSafariZone   uint16 = 0x24f

	yellowSafariSessions       = 3
	yellowSafariLegsPerSession = 500
	yellowSafariBallBudget     = 10
	yellowSafariGateBudget     = 9000
)

var ErrYellowSafariExhausted = errors.New("yellow Safari: bounded sessions exhausted")

func yellowSafariMap(mapID uint8) bool {
	return mapID >= yellowSafariZoneEast && mapID <= yellowSafariZoneCenter
}

func yellowInSafari(m *emu.Emu) bool {
	return m != nil && yellowEventSet(m, yellowEventInSafariZone)
}

func yellowSafariMenuUp(m *emu.Emu) bool {
	if m == nil || m.Peek8(sym.IsInBattle) == 0 || m.Peek8(sym.BattleType) != 2 {
		return false
	}
	text := strings.ToUpper(screenText(m))
	return strings.Contains(text, "BALL") &&
		strings.Contains(text, "BAIT") &&
		strings.Contains(text, "ROCK") &&
		strings.Contains(text, "RUN")
}

func waitYellowSafariMenu(m *emu.Emu) error {
	for frame := 0; frame < yellowCatchMenuBudget; frame++ {
		if m.Peek8(sym.IsInBattle) == 0 {
			return fmt.Errorf("yellow Safari: battle ended before menu appeared")
		}
		if yellowSafariMenuUp(m) {
			return nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return err
			}
			continue
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow Safari: unexpected choice before battle menu: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		m.Tap(emu.A, 3, 7)
	}
	return fmt.Errorf("yellow Safari: battle menu did not appear")
}

// FleeSafari uses the Yellow Safari battle's bottom-right RUN command. It is
// also used by semantic travel when a random Safari encounter interrupts a
// route through the paid finite-step session.
func FleeSafari(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow Safari: nil emulator")
	}
	if m.Peek8(sym.IsInBattle) == 0 {
		return nil
	}
	if m.Peek8(sym.BattleType) != 2 {
		return fmt.Errorf("yellow Safari: battle type %#02x is not Safari", m.Peek8(sym.BattleType))
	}
	if err := waitYellowSafariMenu(m); err != nil {
		return err
	}
	if err := selectYellowBattleMainMenu(m, yellowBattleMenuRightX, 1); err != nil {
		return fmt.Errorf("yellow Safari: select RUN: %w", err)
	}
	m.Tap(emu.A, 3, 7)
	for frame := 0; frame < yellowCatchSettleBudget; frame++ {
		if m.Peek8(sym.IsInBattle) == 0 {
			return waitYellowControllable(m, romData, yellowCatchSettleBudget)
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow Safari: RUN did not end battle")
}

func throwYellowSafariBall(m *emu.Emu, romData []byte, species uint8) (caught, ended bool, err error) {
	before := m.Peek8(sym.NumSafariBalls)
	if before == 0 {
		return false, false, ErrYellowCatchOutOfBalls
	}
	if err := waitYellowSafariMenu(m); err != nil {
		return false, false, err
	}
	if err := selectYellowBattleMainMenu(m, yellowBattleMenuLeftX, 0); err != nil {
		return false, false, fmt.Errorf("yellow Safari: select BALL: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	for frame := 0; frame < yellowCatchSettleBudget; frame++ {
		owned, ownedErr := yellowPokedexOwnsInternal(m, romData, species)
		if ownedErr == nil && owned && m.Peek8(sym.IsInBattle) == 0 {
			if err := waitYellowControllable(m, romData, yellowCatchSettleBudget); err != nil {
				return false, true, err
			}
			return true, true, nil
		}
		if m.Peek8(sym.IsInBattle) == 0 {
			if err := waitYellowControllable(m, romData, yellowCatchSettleBudget); err != nil {
				return false, true, err
			}
			owned, ownedErr = yellowPokedexOwnsInternal(m, romData, species)
			return ownedErr == nil && owned, true, ownedErr
		}
		if yellowSafariMenuUp(m) {
			after := m.Peek8(sym.NumSafariBalls)
			if after+1 != before {
				return false, false, fmt.Errorf("yellow Safari: BALL count changed %d -> %d, want one consumed", before, after)
			}
			return false, false, nil
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return false, false, fmt.Errorf("yellow Safari: decline nickname: %w", err)
			}
			continue
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return false, false, fmt.Errorf("yellow Safari: unexpected choice after BALL: screen=%q",
				strings.Join(strings.Fields(screenText(m)), " "))
		}
		m.Tap(emu.A, 3, 7)
	}
	return false, false, fmt.Errorf("yellow Safari: BALL result did not settle")
}

func enterYellowSafari(m *emu.Emu, romData []byte) error {
	if yellowInSafari(m) && m.Peek8(sym.NumSafariBalls) > 0 {
		return nil
	}

	// Stop safely inside the gate before the y=2 entry trigger. The Fuchsia
	// warp arrives around y=5, so (3,4) is a stable pre-dialogue boundary.
	if m.Peek8(sym.CurMap) != yellowSafariZoneGate {
		if err := GoTo(m, romData, yellowSafariZoneGate, 3, 4); err != nil {
			return fmt.Errorf("yellow Safari: reach gate: %w", err)
		}
	}
	if !yellowInSafari(m) {
		if err := walkTo(m, romData, 3, 2, nil); err != nil {
			// The expected entry script owns control at (3,2). A dialogue
			// takeover is success evidence for this transition, not a walk
			// failure.
			if m.Peek8(sym.CurMap) != yellowSafariZoneGate ||
				(m.Peek8(sym.FontLoaded) == 0 && m.Peek8(sym.JoyIgnore) == 0) {
				return fmt.Errorf("yellow Safari: trigger admission: %w", err)
			}
		}
	}

	for frame := 0; frame < yellowSafariGateBudget; frame++ {
		if yellowInSafari(m) && m.Peek8(sym.NumSafariBalls) > 0 && yellowSafariMap(m.Peek8(sym.CurMap)) {
			return waitYellowControllable(m, romData, 1800)
		}
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow Safari: accept admission: %w", err)
			}
			continue
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow Safari: admission did not start a session; map=%#02x balls=%d event=%v",
		m.Peek8(sym.CurMap), m.Peek8(sym.NumSafariBalls), yellowInSafari(m))
}

func leaveYellowSafari(m *emu.Emu, romData []byte) error {
	if !yellowInSafari(m) {
		return nil
	}
	// Returning to the gate may be interrupted by Safari encounters; GoTo's
	// interruption handler uses FleeSafari for them.
	if m.Peek8(sym.CurMap) != yellowSafariZoneGate {
		_ = GoTo(m, romData, yellowSafariZoneGate, 3, 2)
	}
	for frame := 0; frame < yellowSafariGateBudget; frame++ {
		if !yellowInSafari(m) && m.Peek8(sym.IsInBattle) == 0 {
			if ready, err := yellowprofileObservation(m, romData); err == nil && ready {
				return nil
			}
		}
		if m.Peek8(sym.IsInBattle) != 0 && m.Peek8(sym.BattleType) == 2 {
			if err := FleeSafari(m, romData); err != nil {
				return err
			}
			continue
		}
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			// "Leaving early?" -> YES.
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow Safari: confirm early exit: %w", err)
			}
			continue
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow Safari: session did not close")
}

func travelToYellowSafariGrass(m *emu.Emu, romData []byte, targetMap uint8) error {
	cells, err := yellowrom.GrassEncounterCells(romData, targetMap)
	if err != nil {
		return err
	}
	if len(cells) == 0 {
		return fmt.Errorf("yellow Safari: map %#02x has no encounter grass", targetMap)
	}
	h, err := yellowrom.ParseMap(romData, targetMap)
	if err != nil {
		return err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return err
	}
	components := world.Components(grid)
	seen := map[int]bool{}
	attempts := 0
	var last error
	for _, c := range cells {
		x, y := int(c.X), int(c.Y)
		if !grid.InBounds(x, y) {
			continue
		}
		component := components[y][x]
		if component == 0 || seen[component] {
			continue
		}
		seen[component] = true
		attempts++
		if err := GoTo(m, romData, targetMap, c.X, c.Y); err == nil {
			return nil
		} else {
			last = err
		}
		if !yellowInSafari(m) || attempts >= 8 {
			break
		}
	}
	if last == nil {
		last = fmt.Errorf("no connected Safari grass component")
	}
	return fmt.Errorf("yellow Safari: reach map %#02x grass: %w", targetMap, last)
}

// CaptureSafari owns paid Yellow Safari sessions, habitat routing and the
// special BALL/BAIT/ROCK/RUN battle menu. Completion uses the same Pokédex
// owned-bit proof as every other Yellow capture path.
func CaptureSafari(m *emu.Emu, romData []byte, targetMap, species uint8) (CaptureResult, error) {
	var result CaptureResult
	if m == nil {
		return result, fmt.Errorf("yellow Safari: nil emulator")
	}
	if !yellowSafariMap(targetMap) {
		return result, fmt.Errorf("yellow Safari: map %#02x is not a Safari habitat", targetMap)
	}
	has, err := yellowrom.HasWildSpecies(romData, targetMap, gen1rom.HabitatGrass, species)
	if err != nil {
		return result, err
	}
	if !has {
		return result, fmt.Errorf("yellow Safari: species %#02x is not in map %#02x grass table", species, targetMap)
	}
	owned, err := yellowPokedexOwnsInternal(m, romData, species)
	if err != nil {
		return result, err
	}
	if owned {
		return CaptureResult{Caught: true, Species: species}, nil
	}

	for session := 1; session <= yellowSafariSessions; session++ {
		if err := enterYellowSafari(m, romData); err != nil {
			return result, fmt.Errorf("yellow Safari: enter session %d: %w", session, err)
		}
		if err := travelToYellowSafariGrass(m, romData, targetMap); err != nil {
			if !yellowInSafari(m) {
				continue
			}
			return result, fmt.Errorf("yellow Safari: session %d: %w", session, err)
		}
		a, b, err := chooseYellowGrassPair(romData, targetMap, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
		if err != nil {
			return result, err
		}
		next := a
		for leg := 0; leg < yellowSafariLegsPerSession; leg++ {
			if !yellowInSafari(m) || m.Peek8(sym.NumSafariBalls) == 0 || m.Peek8(sym.CurMap) != targetMap {
				break
			}
			if err := walkTo(m, romData, next.x, next.y, nil); err != nil && m.Peek8(sym.IsInBattle) == 0 {
				if !yellowInSafari(m) || m.Peek8(sym.CurMap) != targetMap {
					break
				}
				a, b, err = chooseYellowGrassPair(romData, targetMap, int(m.Peek8(sym.XCoord)), int(m.Peek8(sym.YCoord)))
				if err != nil {
					return result, err
				}
				next = a
				continue
			}
			if next == a {
				next = b
			} else {
				next = a
			}
			if m.Peek8(sym.IsInBattle) == 0 {
				continue
			}
			if m.Peek8(sym.BattleType) != 2 {
				return result, fmt.Errorf("yellow Safari: encounter battle type %#02x is not Safari", m.Peek8(sym.BattleType))
			}
			enemy, err := yellowWaitEnemySpecies(m)
			if err != nil {
				return result, err
			}
			result.Encounters++
			if enemy != species {
				if err := FleeSafari(m, romData); err != nil {
					return result, fmt.Errorf("yellow Safari: flee species %#02x: %w", enemy, err)
				}
				continue
			}

			limit := yellowSafariBallBudget
			if balls := int(m.Peek8(sym.NumSafariBalls)); balls < limit {
				limit = balls
			}
			for thrown := 0; thrown < limit; thrown++ {
				caught, ended, err := throwYellowSafariBall(m, romData, species)
				if errors.Is(err, ErrYellowCatchOutOfBalls) {
					break
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
				if err := FleeSafari(m, romData); err != nil {
					return result, err
				}
			}
		}

		if yellowInSafari(m) {
			if err := leaveYellowSafari(m, romData); err != nil {
				return result, fmt.Errorf("yellow Safari: close session %d: %w", session, err)
			}
		}
	}

	return result, fmt.Errorf("%w: encounters=%d balls=%d", ErrYellowSafariExhausted, result.Encounters, result.BallsThrown)
}
