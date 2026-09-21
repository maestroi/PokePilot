package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

type FieldMove uint8

const (
	FieldCut FieldMove = iota
	FieldFly
	FieldSurf
	FieldStrength
	FieldFlash
)

type FieldCapability struct {
	Move       FieldMove
	Name       string
	Badge      string
	BadgeOwned bool
	HMOwned    bool
	Learned    bool
	PartySlot  int
	Usable     bool
	Preparable bool
}

type yellowFieldMoveSpec struct {
	move      FieldMove
	name      string
	item      uint8
	moveID    uint8
	menuID    uint8
	badgeBit  uint8
	badgeName string
}

var yellowFieldMoveSpecs = [...]yellowFieldMoveSpec{
	{move: FieldCut, name: "CUT", item: yellowrom.HM01Item, moveID: 0x0f, menuID: 1, badgeBit: 1, badgeName: "Cascade"},
	{move: FieldFly, name: "FLY", item: yellowrom.HM01Item + 1, moveID: 0x13, menuID: 2, badgeBit: 2, badgeName: "Thunder"},
	{move: FieldSurf, name: "SURF", item: yellowrom.HM01Item + 2, moveID: 0x39, menuID: 4, badgeBit: 4, badgeName: "Soul"},
	{move: FieldStrength, name: "STRENGTH", item: yellowrom.HM01Item + 3, moveID: 0x46, menuID: 5, badgeBit: 3, badgeName: "Rainbow"},
	{move: FieldFlash, name: "FLASH", item: yellowrom.HM01Item + 4, moveID: 0x94, menuID: 6, badgeBit: 0, badgeName: "Boulder"},
}

func yellowFieldSpec(move FieldMove) (yellowFieldMoveSpec, bool) {
	if int(move) < 0 || int(move) >= len(yellowFieldMoveSpecs) {
		return yellowFieldMoveSpec{}, false
	}
	return yellowFieldMoveSpecs[move], true
}

func yellowPartyMoveSlot(m *emu.Emu, moveID uint8) int {
	count := int(m.Peek8(sym.PartyCount))
	for slot := 0; slot < count && slot < 6; slot++ {
		base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
		for i := 0; i < 4; i++ {
			if m.Peek8(base+0x08+uint16(i)) == moveID {
				return slot
			}
		}
	}
	return -1
}

func yellowHasBagItem(m *emu.Emu, item uint8) bool {
	_, qty := yellowBagEntry(m, item)
	return qty > 0
}

func yellowBadgeOwned(m *emu.Emu, bit uint8) bool {
	return m.Peek8(sym.ObtainedBadges)&(1<<bit) != 0
}

func fieldCapabilityFor(m *emu.Emu, romData []byte, move FieldMove) FieldCapability {
	spec, ok := yellowFieldSpec(move)
	if !ok {
		return FieldCapability{Move: move, PartySlot: -1}
	}
	slot := yellowPartyMoveSlot(m, spec.moveID)
	badge := yellowBadgeOwned(m, spec.badgeBit)
	hm := yellowHasBagItem(m, spec.item)
	learned := slot >= 0
	preparable := learned
	if !preparable && badge && hm {
		if candidate, _, err := yellowTMHMRecipient(m, romData, spec.item, -1); err == nil && candidate >= 0 {
			preparable = true
		}
	}
	return FieldCapability{
		Move:       move,
		Name:       spec.name,
		Badge:      spec.badgeName,
		BadgeOwned: badge,
		HMOwned:    hm,
		Learned:    learned,
		PartySlot:  slot,
		Usable:     badge && learned,
		Preparable: preparable,
	}
}

func FieldCapabilities(m *emu.Emu, romData []byte) []FieldCapability {
	out := make([]FieldCapability, 0, len(yellowFieldMoveSpecs))
	for move := FieldCut; move <= FieldFlash; move++ {
		out = append(out, fieldCapabilityFor(m, romData, move))
	}
	return out
}

func EnsureFieldMove(m *emu.Emu, romData []byte, move FieldMove) (int, error) {
	spec, ok := yellowFieldSpec(move)
	if !ok {
		return -1, fmt.Errorf("yellow field move: unknown move %d", move)
	}
	if !yellowBadgeOwned(m, spec.badgeBit) {
		return -1, fmt.Errorf("yellow field move: %s requires the %s Badge", spec.name, spec.badgeName)
	}
	if slot := yellowPartyMoveSlot(m, spec.moveID); slot >= 0 {
		return slot, nil
	}
	if !yellowHasBagItem(m, spec.item) {
		return -1, fmt.Errorf("yellow field move: %s requires HM item %#02x", spec.name, spec.item)
	}
	if err := TeachTMHM(m, romData, spec.item, -1); err != nil {
		return -1, fmt.Errorf("yellow field move: teach %s: %w", spec.name, err)
	}
	slot := yellowPartyMoveSlot(m, spec.moveID)
	if slot < 0 {
		return -1, fmt.Errorf("yellow field move: %s was not verified in party RAM", spec.name)
	}
	return slot, nil
}

func yellowFieldMoveMenuIndex(m *emu.Emu, menuID uint8) int {
	for i := 0; i < 4; i++ {
		id := m.Peek8(sym.FieldMoves + uint16(i))
		if id == 0 {
			return -1
		}
		if id == menuID {
			return i
		}
	}
	return -1
}

func openYellowFieldMoveMenu(m *emu.Emu, romData []byte, slot int, menuID uint8) (int, error) {
	_, pokemonIndex, _, err := yellowStartMenuShape(m, romData)
	if err != nil {
		return -1, err
	}
	if err := openYellowStartMenuEntry(m, romData, pokemonIndex); err != nil {
		return -1, fmt.Errorf("open POKEMON: %w", err)
	}
	if _, err := m.StepUntil(1000, func(m *emu.Emu) bool {
		return m.Peek8(sym.PartyMenuTypeOrMessage) == 0 && m.Peek8(sym.PartyCount) > 0
	}); err != nil {
		return -1, fmt.Errorf("party menu did not appear")
	}
	if err := chooseYellowPartySlot(m, slot); err != nil {
		return -1, err
	}
	var index int
	if _, err := m.StepUntil(1200, func(m *emu.Emu) bool {
		index = yellowFieldMoveMenuIndex(m, menuID)
		return index >= 0
	}); err != nil {
		return -1, fmt.Errorf("field move menu id %d did not appear: screen=%q fields=[%d %d %d %d]",
			menuID, strings.Join(strings.Fields(screenText(m)), " "),
			m.Peek8(sym.FieldMoves), m.Peek8(sym.FieldMoves+1), m.Peek8(sym.FieldMoves+2), m.Peek8(sym.FieldMoves+3))
	}
	return index, nil
}

func yellowFieldActionEffect(m *emu.Emu, move FieldMove) bool {
	switch move {
	case FieldCut:
		return m.Peek8(sym.ActionResult) == 1
	case FieldSurf:
		return m.Peek8(sym.ActionResult) == 1 && m.Peek8(sym.WalkBikeSurfState) == 2
	case FieldStrength:
		return m.Peek8(sym.StatusFlags1)&1 != 0
	case FieldFlash:
		return m.Peek8(sym.MapPalOffset) == 0
	default:
		return false
	}
}

func yellowFieldActionAlreadyActive(m *emu.Emu, move FieldMove) bool {
	switch move {
	case FieldSurf:
		return m.Peek8(sym.WalkBikeSurfState) == 2
	case FieldStrength:
		return m.Peek8(sym.StatusFlags1)&1 != 0
	case FieldFlash:
		return m.Peek8(sym.MapPalOffset) == 0
	default:
		return false
	}
}

// UseFieldMove executes Cut, Surf, Strength or Flash from Yellow's real
// START -> POKEMON -> field-move menu and waits for a positive ROM-side
// postcondition. Fly is destination-aware and is intentionally handled by
// FlyTo rather than making a blind city selection here.
func UseFieldMove(m *emu.Emu, romData []byte, move FieldMove) error {
	if m == nil {
		return fmt.Errorf("yellow field move: nil emulator")
	}
	spec, ok := yellowFieldSpec(move)
	if !ok {
		return fmt.Errorf("yellow field move: unknown move %d", move)
	}
	if move == FieldFly {
		return fmt.Errorf("yellow field move: Fly requires a destination; use FlyTo")
	}
	if yellowFieldActionAlreadyActive(m, move) {
		return nil
	}
	ready, err := yellowprofileObservation(m, romData)
	if err != nil || !ready {
		return fmt.Errorf("yellow field move: %s requires a stable overworld boundary", spec.name)
	}
	slot, err := EnsureFieldMove(m, romData, move)
	if err != nil {
		return err
	}
	index, err := openYellowFieldMoveMenu(m, romData, slot, spec.menuID)
	if err != nil {
		return fmt.Errorf("yellow field move: %s: %w", spec.name, err)
	}
	if err := selectYellowLinearMenuItem(m, index); err != nil {
		return fmt.Errorf("yellow field move: %s selection: %w", spec.name, err)
	}
	m.Tap(emu.A, 3, 7)

	for frame := 0; frame < 4000; frame++ {
		if yellowFieldActionEffect(m, move) {
			ready, err := yellowprofileObservation(m, romData)
			if err == nil && ready {
				return nil
			}
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow field move: %s opened unexpected choice: screen=%q",
				spec.name, strings.Join(strings.Fields(screenText(m)), " "))
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow field move: %s did not reach its expected postcondition", spec.name)
}

var ErrFlyDestinationUnvisited = errors.New("yellow Fly: destination has not been visited")

const (
	yellowFlyCityCount = 11
	yellowNotVisited   = 0xfe
)

// FlyTo executes Yellow's destination-aware Fly UI. The town-map list is
// built by the ROM from wTownVisitedFlag; this controller reads that live list,
// refuses unvisited destinations, drives only among entries the game exposed,
// and verifies the actual destination map after the fly transition.
func FlyTo(m *emu.Emu, romData []byte, destMap uint8) error {
	if m == nil {
		return fmt.Errorf("yellow Fly: nil emulator")
	}
	if destMap >= yellowFlyCityCount || yellowrom.MapName(destMap) == "" {
		return fmt.Errorf("yellow Fly: map %#02x is not a fly city", destMap)
	}
	if m.Peek8(sym.CurMap) == destMap {
		return nil
	}

	spec, _ := yellowFieldSpec(FieldFly)
	slot, err := EnsureFieldMove(m, romData, FieldFly)
	if err != nil {
		return err
	}
	index, err := openYellowFieldMoveMenu(m, romData, slot, spec.menuID)
	if err != nil {
		return fmt.Errorf("yellow Fly: open field menu: %w", err)
	}
	if err := selectYellowLinearMenuItem(m, index); err != nil {
		return fmt.Errorf("yellow Fly: choose FLY: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	if _, err := m.StepUntil(2400, func(m *emu.Emu) bool {
		return strings.Contains(strings.ToUpper(screenText(m)), "TO")
	}); err != nil {
		return fmt.Errorf("yellow Fly: town map did not open: %w", err)
	}

	locations := make([]uint8, yellowFlyCityCount)
	m.PeekInto(sym.FlyLocationsList, locations)
	if locations[destMap] == yellowNotVisited {
		// B returns safely from the town map.
		m.Tap(emu.B, 3, 7)
		return fmt.Errorf("%w: %s", ErrFlyDestinationUnvisited, yellowrom.MapName(destMap))
	}
	if locations[destMap] != destMap {
		m.Tap(emu.B, 3, 7)
		return fmt.Errorf("yellow Fly: destination list entry %d=%#02x, want %#02x",
			destMap, locations[destMap], destMap)
	}

	// LoadTownMap_Fly starts at entry zero. UP advances the list pointer and
	// skips NOT_VISITED entries. Count exactly the visible visited entries
	// preceding our destination; no wraparound or guessed city ordering.
	steps := 0
	for city := 1; city <= int(destMap); city++ {
		if locations[city] != yellowNotVisited {
			steps++
		}
	}
	for i := 0; i < steps; i++ {
		before := screenText(m)
		m.Tap(emu.Up, 3, 7)
		if _, err := m.StepUntil(180, func(m *emu.Emu) bool {
			return screenText(m) != before
		}); err != nil {
			return fmt.Errorf("yellow Fly: destination cursor did not advance at step %d/%d", i+1, steps)
		}
	}

	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(1200, func(m *emu.Emu) bool {
		return m.Peek8(sym.DestinationMap) == destMap
	}); err != nil {
		return fmt.Errorf("yellow Fly: town map did not commit destination %#02x: %w", destMap, err)
	}
	if _, err := m.StepUntil(6000, func(m *emu.Emu) bool {
		return m.Peek8(sym.CurMap) == destMap
	}); err != nil {
		return fmt.Errorf("yellow Fly: transition did not reach %s: %w", yellowrom.MapName(destMap), err)
	}
	if err := waitYellowControllable(m, romData, 3000); err != nil {
		return fmt.Errorf("yellow Fly: arrival did not settle: %w", err)
	}
	return nil
}
