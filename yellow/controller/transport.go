package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowBicycleItem   = 0x06
	yellowPokeFluteItem = 0x49

	yellowRoute12 = 0x17
	yellowRoute16 = 0x1b

	yellowEventFightRoute12Snorlax = 0x48e
	yellowEventBeatRoute12Snorlax  = 0x48f
	yellowEventFightRoute16Snorlax = 0x4c8
	yellowEventBeatRoute16Snorlax  = 0x4c9
)

func yellowEventSet(m *emu.Emu, event uint16) bool {
	return m.Peek8(sym.EventFlags+event/8)&(1<<uint(event%8)) != 0
}

// UseBicycle mounts Yellow's Bicycle from the real bag and verifies the
// walking/biking/surfing state instead of treating a closed menu as success.
func UseBicycle(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow Bicycle: nil emulator")
	}
	if m.Peek8(sym.WalkBikeSurfState) == 1 {
		return nil
	}
	if m.Peek8(sym.WalkBikeSurfState) == 2 {
		return fmt.Errorf("yellow Bicycle: cannot mount while surfing")
	}
	if m.Peek8(sym.CurMap) >= 0x25 {
		return fmt.Errorf("yellow Bicycle: current map %#02x is indoors", m.Peek8(sym.CurMap))
	}
	idx, qty := yellowBagEntry(m, yellowBicycleItem)
	if idx < 0 || qty == 0 {
		return fmt.Errorf("yellow Bicycle: Bicycle is not in the bag")
	}
	if err := openYellowBag(m, romData); err != nil {
		return fmt.Errorf("yellow Bicycle: open bag: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return fmt.Errorf("yellow Bicycle: select item: %w", err)
	}
	for frame := 0; frame < 2400; frame++ {
		if m.Peek8(sym.WalkBikeSurfState) == 1 {
			if err := waitYellowControllable(m, romData, 1200); err == nil {
				return nil
			}
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow Bicycle: unexpected choice: screen=%q", strings.Join(strings.Fields(screenText(m)), " "))
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	for i := 0; i < 8; i++ {
		m.Tap(emu.B, 3, 7)
	}
	return fmt.Errorf("yellow Bicycle: biking state did not activate")
}

func yellowSnorlaxFluteTarget(m *emu.Emu) (fightEvent, beatEvent uint16, ok bool) {
	x, y := m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
	switch m.Peek8(sym.CurMap) {
	case yellowRoute12:
		switch {
		case x == 9 && y == 62, x == 10 && y == 61, x == 10 && y == 63, x == 11 && y == 62:
			return yellowEventFightRoute12Snorlax, yellowEventBeatRoute12Snorlax, true
		}
	case yellowRoute16:
		if y == 10 && (x == 25 || x == 27) {
			return yellowEventFightRoute16Snorlax, yellowEventBeatRoute16Snorlax, true
		}
	}
	return 0, 0, false
}

func startPokeFluteSnorlaxBattle(m *emu.Emu, romData []byte) (fightEvent, beatEvent uint16, err error) {
	if m == nil {
		return 0, 0, fmt.Errorf("yellow Poke Flute: nil emulator")
	}
	fightEvent, beatEvent, ok := yellowSnorlaxFluteTarget(m)
	if !ok {
		return 0, 0, fmt.Errorf("yellow Poke Flute: player is not at a Route 12/16 Snorlax activation tile")
	}
	if yellowEventSet(m, beatEvent) {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: Snorlax source is already consumed")
	}
	idx, qty := yellowBagEntry(m, yellowPokeFluteItem)
	if idx < 0 || qty == 0 {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: Poke Flute is not in the bag")
	}
	if err := openYellowBag(m, romData); err != nil {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: open bag: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: select item: %w", err)
	}
	if _, err := m.StepUntil(900, yellowUseTossPrompt); err != nil {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: USE/TOSS prompt did not appear")
	}
	if err := selectYellowLinearMenuItem(m, 0); err != nil {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: select USE: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	for frame := 0; frame < 5000; frame++ {
		if yellowEventSet(m, fightEvent) {
			break
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: unexpected choice before Snorlax fight")
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	if !yellowEventSet(m, fightEvent) {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: flute played without setting Snorlax fight event")
	}

	for frame := 0; frame < 3000 && m.Peek8(sym.IsInBattle) == 0; frame++ {
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	if m.Peek8(sym.IsInBattle) == 0 {
		return fightEvent, beatEvent, fmt.Errorf("yellow Poke Flute: Snorlax fight event set but battle did not start")
	}
	return fightEvent, beatEvent, nil
}

// UsePokeFluteAtSnorlax owns the two overworld Snorlax story interactions.
// The legal activation coordinates and event bits come from Yellow's ROM
// scripts. Success requires the fight event, a resolved battle, and the
// durable beat event; merely playing the flute is not enough.
func UsePokeFluteAtSnorlax(m *emu.Emu, romData []byte) error {
	_, beatEvent, err := startPokeFluteSnorlaxBattle(m, romData)
	if err != nil {
		return err
	}
	result, err := Battle(m, romData)
	if err != nil {
		return fmt.Errorf("yellow Poke Flute: Snorlax battle: %w", err)
	}
	if result.Outcome == BattleOutcomeLost {
		return fmt.Errorf("yellow Poke Flute: lost Snorlax battle")
	}

	for frame := 0; frame < 3000; frame++ {
		if yellowEventSet(m, beatEvent) {
			return waitYellowControllable(m, romData, 1800)
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow Poke Flute: Snorlax battle ended without durable beat event")
}
