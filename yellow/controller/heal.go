package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowNurseSpriteID  = 0x29
	yellowHealMenuBudget = 3000
	yellowHealRunBudget  = 30000
)

type nurseApproach struct {
	steps []world.Step
	face  world.Step
}

func findYellowNurseApproach(romData []byte, mapID uint8, sx, sy int) (nurseApproach, error) {
	h, err := yellowrom.ParseMap(romData, mapID)
	if err != nil {
		return nurseApproach{}, err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return nurseApproach{}, err
	}

	var nurse *yellowrom.Object
	for i := range h.Objects {
		if h.Objects[i].SpriteID == yellowNurseSpriteID {
			nurse = &h.Objects[i]
			break
		}
	}
	if nurse == nil {
		return nurseApproach{}, fmt.Errorf("no Yellow nurse sprite on map %#02x (%s)", mapID, yellowrom.MapName(mapID))
	}

	blocked := staticObjectBlockers(h, nil)
	delete(blocked, [2]int{sx, sy})

	best := nurseApproach{}
	bestLen := -1
	for _, outward := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		midX := int(nurse.X) + outward.DX
		midY := int(nurse.Y) + outward.DY
		standX := int(nurse.X) + 2*outward.DX
		standY := int(nurse.Y) + 2*outward.DY
		if !grid.InBounds(midX, midY) || !grid.InBounds(standX, standY) {
			continue
		}
		if grid.Walkable(midX, midY) || !grid.Walkable(standX, standY) {
			continue
		}
		if blocked[[2]int{standX, standY}] {
			continue
		}
		steps, err := world.FindPath(grid, sx, sy, standX, standY, blocked)
		if err != nil {
			continue
		}
		if bestLen < 0 || len(steps) < bestLen {
			bestLen = len(steps)
			best = nurseApproach{
				steps: steps,
				face:  world.Step{DX: -outward.DX, DY: -outward.DY},
			}
		}
	}
	if bestLen < 0 {
		return nurseApproach{}, fmt.Errorf("no reachable counter approach for nurse at (%d,%d) on map %#02x",
			nurse.X, nurse.Y, mapID)
	}
	return best, nil
}

func allYellowPartyRecovered(m *emu.Emu, romData []byte) (bool, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return false, err
	}
	if len(obs.Party) == 0 {
		return false, nil
	}
	for _, mon := range obs.Party {
		if mon.MaxHP == 0 || mon.HP != mon.MaxHP || mon.Status != "" {
			return false, nil
		}
	}
	return true, nil
}

// Heal restores the party at a Yellow Pokémon Center. Nurse location comes
// from the Yellow ROM object table and the counter approach is derived from
// collision geometry, so no city-specific center coordinates are required.
func Heal(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow heal: nil emulator")
	}
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return fmt.Errorf("yellow heal: observe: %w", err)
	}
	if !obs.Controllable || obs.InBattle {
		return fmt.Errorf("yellow heal: player is not at a controllable overworld boundary")
	}
	if len(obs.Party) == 0 {
		return fmt.Errorf("yellow heal: empty party")
	}
	mapName := strings.ToUpper(yellowrom.MapName(uint8(obs.NativeMapID)))
	if !strings.Contains(mapName, "POKECENTER") && mapName != "INDIGO_PLATEAU_LOBBY" {
		return fmt.Errorf("yellow heal: map %s is not a supported Center", mapName)
	}

	approach, err := findYellowNurseApproach(romData, uint8(obs.NativeMapID), int(obs.X), int(obs.Y))
	if err != nil {
		return fmt.Errorf("yellow heal: nurse approach: %w", err)
	}
	if err := walkPath(m, uint8(obs.NativeMapID), approach.steps); err != nil {
		return fmt.Errorf("yellow heal: walk to nurse counter: %w", err)
	}
	btn, ok := buttonFor(approach.face)
	if !ok {
		return fmt.Errorf("yellow heal: invalid nurse-facing direction %+v", approach.face)
	}
	m.Tap(btn, 3, 7)
	m.Tap(emu.A, 3, 7)

	menuReady := false
	for frame := 0; frame < yellowHealMenuBudget; frame++ {
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			menuReady = true
			break
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			return fmt.Errorf("yellow heal: unexpected battle while opening nurse dialogue")
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	if !menuReady {
		return fmt.Errorf("yellow heal: nurse YES/NO prompt did not appear within %d frames", yellowHealMenuBudget)
	}
	if err := selectYellowTwoOption(m, false); err != nil {
		return fmt.Errorf("yellow heal: choose YES: %w", err)
	}

	stable := 0
	for frame := 0; frame < yellowHealRunBudget; frame++ {
		recovered, err := allYellowPartyRecovered(m, romData)
		if err != nil {
			return fmt.Errorf("yellow heal: verify party: %w", err)
		}
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return fmt.Errorf("yellow heal: observe completion: %w", err)
		}
		if recovered && obs.Controllable && !obs.InBattle {
			stable++
			if stable >= 12 {
				return nil
			}
		} else {
			stable = 0
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow heal: party did not recover to a stable boundary within %d frames", yellowHealRunBudget)
}
