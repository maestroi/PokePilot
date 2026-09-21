package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/world"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

type yellowGiftSpec struct {
	species uint8
	name    string
	mapID   uint8
	x, y    uint8
	event   uint16
}

var yellowGiftSpecs = []yellowGiftSpec{
	{species: 0x99, name: "bulbasaur", mapID: 0x3f, x: 3, y: 1, event: 168},
	{species: 0xb0, name: "charmander", mapID: 0x23, x: 6, y: 5, event: 0x54f},
	{species: 0xb1, name: "squirtle", mapID: 0x05, x: 19, y: 15, event: 327},
}

func yellowGiftForSpecies(species uint8) (yellowGiftSpec, bool) {
	for _, spec := range yellowGiftSpecs {
		if spec.species == species {
			return spec, true
		}
	}
	return yellowGiftSpec{}, false
}

func yellowPokedexOwnsInternal(m *emu.Emu, romData []byte, species uint8) (bool, error) {
	dex, err := yellowrom.InternalSpeciesDexNumber(romData, species)
	if err != nil {
		return false, err
	}
	if dex == 0 {
		return false, fmt.Errorf("yellow gift: species %#02x has zero Dex number", species)
	}
	bit := int(dex) - 1
	return m.Peek8(sym.PokedexOwned+uint16(bit/8))&(1<<uint(bit%8)) != 0, nil
}

func yellowInteractionApproaches(romData []byte, spec yellowGiftSpec) ([][2]uint8, error) {
	h, err := yellowrom.ParseMap(romData, spec.mapID)
	if err != nil {
		return nil, err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return nil, err
	}
	var out [][2]uint8
	for _, step := range []world.Step{world.StepDown, world.StepUp, world.StepRight, world.StepLeft} {
		x, y := int(spec.x)+step.DX, int(spec.y)+step.DY
		if grid.InBounds(x, y) && grid.Walkable(x, y) {
			out = append(out, [2]uint8{uint8(x), uint8(y)})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("yellow gift: no walkable approach to %s at (%d,%d)", spec.name, spec.x, spec.y)
	}
	return out, nil
}

func travelToYellowGift(m *emu.Emu, romData []byte, spec yellowGiftSpec) error {
	if m.Peek8(sym.CurMap) == spec.mapID {
		return nil
	}
	approaches, err := yellowInteractionApproaches(romData, spec)
	if err != nil {
		return err
	}
	var last error
	for _, p := range approaches {
		if err := GoTo(m, romData, spec.mapID, p[0], p[1]); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return fmt.Errorf("yellow gift: travel to %s: %w", spec.name, last)
}

// ReceiveGift receives Yellow's Bulbasaur, Charmander or Squirtle scripted
// gift. Completion requires the source's durable event and the Pokédex owned
// bit, so declining the offer or exhausting dialogue can never look complete.
func ReceiveGift(m *emu.Emu, romData []byte, species uint8) error {
	if m == nil {
		return fmt.Errorf("yellow gift: nil emulator")
	}
	spec, ok := yellowGiftForSpecies(species)
	if !ok {
		return fmt.Errorf("yellow gift: unsupported species %#02x", species)
	}
	if yellowEventSet(m, spec.event) {
		owned, err := yellowPokedexOwnsInternal(m, romData, species)
		if err != nil {
			return err
		}
		if owned {
			return nil
		}
		return fmt.Errorf("yellow gift: %s event is set but Pokédex ownership is absent", spec.name)
	}
	if err := travelToYellowGift(m, romData, spec); err != nil {
		return err
	}
	if err := interactAt(m, romData, int(spec.x), int(spec.y)); err != nil {
		return fmt.Errorf("yellow gift: talk to %s: %w", spec.name, err)
	}

	for frame := 0; frame < 9000; frame++ {
		if yellowEventSet(m, spec.event) {
			owned, err := yellowPokedexOwnsInternal(m, romData, species)
			if err == nil && owned {
				if m.Peek8(sym.FontLoaded) != 0 || m.Peek8(sym.JoyIgnore) != 0 {
					if _, err := RecoverDialogue(m, romData); err != nil {
						return fmt.Errorf("yellow gift: settle %s dialogue: %w", spec.name, err)
					}
				}
				return waitYellowControllable(m, romData, 1800)
			}
		}
		if nicknamePrompt(m) {
			if err := declineNickname(m); err != nil {
				return fmt.Errorf("yellow gift: decline %s nickname: %w", spec.name, err)
			}
			continue
		}
		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow gift: accept %s: %w", spec.name, err)
			}
			continue
		}
		ready, _ := yellowprofileObservation(m, romData)
		if ready && frame > 120 && !yellowEventSet(m, spec.event) {
			return fmt.Errorf("yellow gift: %s interaction returned to overworld without setting event; prerequisite or party/storage condition is unmet", spec.name)
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow gift: %s did not complete within frame budget", spec.name)
}
