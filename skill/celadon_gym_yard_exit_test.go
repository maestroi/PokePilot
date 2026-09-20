package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestCeladonGymYardHasNoLandRouteToCenter pins the component-routing gap
// behind run-jjzpcm0bpqco24ijjvh3vm93q: with can_cut, FindRoutePlan still
// reports no route from the gym yard (12,28) to the Pokemon Center because
// leave-and-return through the gym cannot relaxLanding onto an already-
// occupied city map. GoTo must open a field-path exit before re-planning.
func TestCeladonGymYardHasNoLandRouteToCenter(t *testing.T) {
	romData := badgeFourROM(t)
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder | 1<<state.BadgeRainbow
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 6
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities missing can_cut: %v", prereqs.Capabilities)
	}
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}
	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, celadonCityMap, center.Map, 12, 28, int(center.X), int(center.Y), nil, prereqs,
	)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("yard land route err=%v, want world.ErrNoRoute (GoTo field-exit recovery owns the Cut)", err)
	}
}

// TestGoToOpensCeladonGymYardTowardCenter is the live replay for
// run-jjzpcm0bpqco24ijjvh3vm93q: standing in the Cut-sealed gym yard, GoTo
// the Pokemon Center must Cut out and arrive. Supply the farm round state via
// POKEPILOT_CELADON_GYM_YARD_STATE (or REPRO_STATE).
func TestGoToOpensCeladonGymYardTowardCenter(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed gym-yard Cut exit; run without -short")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	statePath := os.Getenv("POKEPILOT_CELADON_GYM_YARD_STATE")
	if statePath == "" {
		statePath = os.Getenv("REPRO_STATE")
	}
	if romPath == "" || statePath == "" {
		t.Skip("POKEMON_RED_ROM and POKEPILOT_CELADON_GYM_YARD_STATE (or REPRO_STATE) required")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatal(err)
	}
	var before state.Mem
	state.Snapshot(m, &before)
	if before.U8(sym.CurMap) != celadonCityMap || before.U8(sym.XCoord) != 12 || before.U8(sym.YCoord) != 28 {
		t.Fatalf("prepared state at map %#02x (%d,%d), want Celadon City (12,28)",
			before.U8(sym.CurMap), before.U8(sym.XCoord), before.U8(sym.YCoord))
	}
	if !redRouteCapabilities(romData, &before).Has(capCanCut) {
		t.Fatal("prepared gym-yard state cannot Cut")
	}
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}
	if err := GoTo(m, romData, center); err != nil {
		t.Fatalf("GoTo celadon pokemon center from gym yard: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != center.Map {
		t.Fatalf("after GoTo map=%#02x, want %#02x", got, center.Map)
	}
}
