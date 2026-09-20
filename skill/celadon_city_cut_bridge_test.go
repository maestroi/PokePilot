package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestCeladonCutPocketBlocksLandRouteToGameCorner pins farm triage
// d66255b02d6cdbab (run-3hksgfzyx8naz3kvcvmuevzjeo): standing at Celadon City
// (12,28) inside the gym-yard Cut pocket, component routing cannot reach the
// Game Corner stand used by RocketHideout / silph_scope_acquired. Land-only
// FindRoutePlan must keep reporting ErrNoRoute so GoTo's field-path bridge
// (#1357) remains the recovery path rather than a silent graph invent.
func TestCeladonCutPocketBlocksLandRouteToGameCorner(t *testing.T) {
	romData := badgeFourROM(t)

	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder | 1<<state.BadgeRainbow
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 44 // Oddish: Cut-compatible
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}

	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, celadonCityMap, gameCornerStand.Map, 12, 28, int(gameCornerStand.X), int(gameCornerStand.Y), nil, prereqs,
	)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("land route from Celadon (12,28) to Game Corner: err=%v, want ErrNoRoute", err)
	}
}

// TestCeladonCutPocketFieldPathOpensGameCornerDoorApproach proves the same
// pocket is escapable with destination-aware Cut: from (12,28) a field path
// must reach an ordinary standing tile beside the Game Corner door warp
// (28,19). That is the port GoTo's field-path bridge ranks before re-planning
// the cross-map RocketHideout Travel.
func TestCeladonCutPocketFieldPathOpensGameCornerDoorApproach(t *testing.T) {
	romData := badgeFourROM(t)
	h, err := rom.ParseMap(romData, celadonCityMap)
	if err != nil {
		t.Fatalf("ParseMap(Celadon): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(Celadon): %v", err)
	}

	const doorX, doorY = 28, 19
	var opened bool
	for _, step := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		ax, ay := doorX+step.DX, doorY+step.DY
		if !grid.Walkable(ax, ay) {
			continue
		}
		plan, perr := planFieldPath(grid, grid, h.Tileset, 12, 28, ax, ay, nil, true, false, false)
		if perr != nil {
			continue
		}
		if countFieldActions(plan, fieldPathCut) == 0 {
			t.Fatalf("field path to Game Corner approach (%d,%d) used no Cut; plan=%+v", ax, ay, plan)
		}
		opened = true
		break
	}
	if !opened {
		t.Fatal("no Cut-aware field path from Celadon (12,28) to a Game Corner door approach")
	}
}

// TestCeladonCutPocketBlocksLandRouteToCenter pins farm triage
// ab11fbcf89382c39 (run-jjzpcm0bpqco24ijjvh3vm93q): VirtualTrade / GoTo the
// Celadon Pokemon Center from the same (12,28) gym-yard Cut pocket dies on
// world.ErrNoRoute. Land-only FindRoutePlan must keep reporting that so
// GoTo's field-path bridge (#1357) remains the recovery path.
func TestCeladonCutPocketBlocksLandRouteToCenter(t *testing.T) {
	romData := badgeFourROM(t)
	center, ok := Place("celadon pokemon center")
	if !ok {
		t.Fatal(`Place("celadon pokemon center") missing`)
	}

	var mem state.Mem
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder | 1<<state.BadgeRainbow
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 44 // Oddish: Cut-compatible
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff

	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanCut) {
		t.Fatalf("capabilities did not include %q: %v", capCanCut, prereqs.Capabilities)
	}

	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, celadonCityMap, center.Map, 12, 28, int(center.X), int(center.Y), nil, prereqs,
	)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("land route from Celadon (12,28) to Pokemon Center: err=%v, want ErrNoRoute", err)
	}
}

// TestCeladonCutPocketFieldPathOpensCenterDoorApproach proves the same pocket
// is escapable toward the Center: from (12,28) a field path must reach an
// ordinary standing tile beside the Pokemon Center door warp (41,9).
func TestCeladonCutPocketFieldPathOpensCenterDoorApproach(t *testing.T) {
	romData := badgeFourROM(t)
	h, err := rom.ParseMap(romData, celadonCityMap)
	if err != nil {
		t.Fatalf("ParseMap(Celadon): %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(Celadon): %v", err)
	}

	const doorX, doorY = 41, 9
	var opened bool
	for _, step := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		ax, ay := doorX+step.DX, doorY+step.DY
		if !grid.Walkable(ax, ay) {
			continue
		}
		plan, perr := planFieldPath(grid, grid, h.Tileset, 12, 28, ax, ay, nil, true, false, false)
		if perr != nil {
			continue
		}
		if countFieldActions(plan, fieldPathCut) == 0 {
			t.Fatalf("field path to Pokemon Center approach (%d,%d) used no Cut; plan=%+v", ax, ay, plan)
		}
		opened = true
		break
	}
	if !opened {
		t.Fatal("no Cut-aware field path from Celadon (12,28) to a Pokemon Center door approach")
	}
}
