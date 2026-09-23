package skill

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func controllableFastTravelMem() state.Mem {
	var mem state.Mem
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 9
	return mem
}

func setTownVisited(mem *state.Mem, mapID uint8) {
	mem[sym.TownVisitedFlag+uint16(mapID)/8] |= 1 << (mapID % 8)
}

func TestLegalFastTravelOptionsExposeOnlyUnlockedFlyDestinations(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x01
	mem[sym.CurMapTileset] = overworldTileset
	mem[sym.ObtainedBadges] = 1 << 2 // Thunder Badge
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = fieldFlyMove
	setTownVisited(&mem, 0)
	setTownVisited(&mem, 6)

	options := legalFastTravelOptions(&mem)
	if len(options) != 2 {
		t.Fatalf("options=%+v, want Pallet and Celadon Fly edges", options)
	}
	seen := map[uint8]bool{}
	for _, option := range options {
		if option.Kind != fastTravelFly {
			t.Fatalf("unexpected non-Fly option: %+v", option)
		}
		seen[option.Landing.Map] = true
	}
	if !seen[0] || !seen[6] {
		t.Fatalf("Fly options maps=%v, want visited 0 and 6", seen)
	}

	mem[sym.ObtainedBadges] = 0
	if got := legalFastTravelOptions(&mem); len(got) != 0 {
		t.Fatalf("Fly options without Thunder Badge=%+v, want none", got)
	}
}

func TestLegalFastTravelOptionsDigAndEscapeReturnToLastCenter(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x3d
	mem[sym.CurMapTileset] = 17 // CAVERN
	mem[sym.LastBlackoutMap] = 0x02
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = digMoveID
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = escapeRopeItem
	mem[sym.BagItems+1] = 2

	options := legalFastTravelOptions(&mem)
	var dig, rope *fastTravelOption
	for i := range options {
		switch options[i].Kind {
		case fastTravelDig:
			dig = &options[i]
		case fastTravelEscapeRope:
			rope = &options[i]
		}
	}
	if dig == nil || rope == nil {
		t.Fatalf("options=%+v, want Dig and Escape Rope", options)
	}
	want := flyLanding[0x02]
	if dig.Landing != want || rope.Landing != want {
		t.Fatalf("escape landings dig=%+v rope=%+v want=%+v", dig.Landing, rope.Landing, want)
	}
	if dig.ActionCost >= rope.ActionCost {
		t.Fatalf("Dig cost=%d Escape Rope=%d; reusable Dig should be cheaper", dig.ActionCost, rope.ActionCost)
	}
}

func TestEscapeTravelUsesSpecialWarpLandingsForRouteCenters(t *testing.T) {
	for mapID, want := range map[uint8]Destination{
		0x0f: {Map: 0x0f, X: 11, Y: 6},
		0x15: {Map: 0x15, X: 11, Y: 20},
	} {
		got, ok := specialWarpLanding(mapID)
		if !ok || got != want {
			t.Fatalf("specialWarpLanding(%#02x) = %+v,%v; want %+v", mapID, got, ok, want)
		}
	}
}

func TestEscapeTravelAllowedRejectsAgathaDespiteCemeteryTileset(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = agathasRoomMap
	mem[sym.CurMapTileset] = 15 // CEMETERY
	if escapeTravelAllowed(&mem) {
		t.Fatal("Agatha's room incorrectly allows Dig/Escape Rope")
	}
	mem[sym.CurMap] = 0x95
	if !escapeTravelAllowed(&mem) {
		t.Fatal("ordinary cemetery map unexpectedly rejects Dig/Escape Rope")
	}
}

func TestEmergencyEgressPrefersDigOverEscapeRope(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x3d
	mem[sym.CurMapTileset] = 17 // CAVERN
	mem[sym.LastBlackoutMap] = 0x02
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = digMoveID
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = escapeRopeItem
	mem[sym.BagItems+1] = 1

	got := chooseEmergencyEgress(&mem)
	if got.Method != emergencyEgressDig {
		t.Fatalf("emergency egress = %+v, want Dig", got)
	}
	if got.Landing != flyLanding[0x02] {
		t.Fatalf("landing = %+v, want %+v", got.Landing, flyLanding[0x02])
	}
}

func TestEmergencyEgressUsesEscapeRopeWhenDigUnavailable(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x3d
	mem[sym.CurMapTileset] = 17 // CAVERN
	mem[sym.LastBlackoutMap] = 0x02
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = escapeRopeItem
	mem[sym.BagItems+1] = 1

	if got := chooseEmergencyEgress(&mem); got.Method != emergencyEgressEscapeRope {
		t.Fatalf("emergency egress = %+v, want Escape Rope", got)
	}
}

func TestEmergencyEgressUsesTeleportOutside(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x01
	mem[sym.CurMapTileset] = overworldTileset
	mem[sym.LastBlackoutMap] = 0x02
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = teleportMoveID

	if got := chooseEmergencyEgress(&mem); got.Method != emergencyEgressTeleport || got.Landing != flyLanding[0x02] {
		t.Fatalf("emergency egress = %+v, want Teleport to %+v", got, flyLanding[0x02])
	}
}

func TestEmergencyEgressFallsBackToFlyToLastVisitedTown(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x01
	mem[sym.CurMapTileset] = overworldTileset
	mem[sym.LastBlackoutMap] = 0x06
	mem[sym.ObtainedBadges] = 1 << 2 // Thunder Badge
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = fieldFlyMove
	setTownVisited(&mem, 0)
	setTownVisited(&mem, 6)

	if got := chooseEmergencyEgress(&mem); got.Method != emergencyEgressFly || got.Landing != flyLanding[0x06] {
		t.Fatalf("emergency egress = %+v, want Fly to %+v", got, flyLanding[0x06])
	}
}

func TestChooseFastTravelByCostPreservesCheaperWalking(t *testing.T) {
	options := []fastTravelOption{{
		Kind:       fastTravelFly,
		Landing:    Destination{Map: 6, X: 41, Y: 10},
		ActionCost: fastTravelFlyActionCost,
	}}
	got := chooseFastTravelByCost(100, true, options, func(Destination) (int, bool) {
		return 0, true
	})
	if got.Kind != fastTravelNone {
		t.Fatalf("choice=%+v, want walking because 100 < Fly cost %d", got, fastTravelFlyActionCost)
	}
}

func TestChooseFastTravelByCostUsesShortcutOnlyWhenItWins(t *testing.T) {
	celadon := Destination{Map: 6, X: 41, Y: 10}
	options := []fastTravelOption{{
		Kind:       fastTravelFly,
		Landing:    celadon,
		ActionCost: fastTravelFlyActionCost,
	}}
	got := chooseFastTravelByCost(500, true, options, func(landing Destination) (int, bool) {
		if landing != celadon {
			return 0, false
		}
		return 40, true
	})
	if got.Kind != fastTravelFly || got.Cost != fastTravelFlyActionCost+40 {
		t.Fatalf("choice=%+v, want Fly total=%d", got, fastTravelFlyActionCost+40)
	}
}

func TestChooseFastTravelByCostCanUseIntermediateLanding(t *testing.T) {
	cerulean := Destination{Map: 3, X: 19, Y: 18}
	celadon := Destination{Map: 6, X: 41, Y: 10}
	options := []fastTravelOption{
		{Kind: fastTravelFly, Landing: cerulean, ActionCost: fastTravelFlyActionCost},
		{Kind: fastTravelFly, Landing: celadon, ActionCost: fastTravelFlyActionCost},
	}
	got := chooseFastTravelByCost(600, true, options, func(landing Destination) (int, bool) {
		switch landing.Map {
		case 3:
			return 300, true
		case 6:
			return 100, true
		default:
			return 0, false
		}
	})
	if got.Kind != fastTravelFly || got.Map != 6 {
		t.Fatalf("choice=%+v, want Fly via cheaper Celadon onward route", got)
	}
}

func TestChooseFastTravelByCostPrefersReusableDigOverEscapeRope(t *testing.T) {
	landing := Destination{Map: 2, X: 13, Y: 26}
	options := []fastTravelOption{
		{Kind: fastTravelEscapeRope, Landing: landing, ActionCost: fastTravelEscapeActionCost},
		{Kind: fastTravelDig, Landing: landing, ActionCost: fastTravelDigActionCost},
	}
	got := chooseFastTravelByCost(400, true, options, func(Destination) (int, bool) {
		return 100, true
	})
	if got.Kind != fastTravelDig {
		t.Fatalf("choice=%+v, want Dig over consumable Escape Rope", got)
	}
}

func TestChooseFastTravelByCostCanBypassBlockedOrdinaryRoute(t *testing.T) {
	landing := Destination{Map: 2, X: 13, Y: 26}
	options := []fastTravelOption{{
		Kind:       fastTravelDig,
		Landing:    landing,
		ActionCost: fastTravelDigActionCost,
	}}
	got := chooseFastTravelByCost(0, false, options, func(Destination) (int, bool) {
		return 100, true
	})
	if got.Kind != fastTravelDig {
		t.Fatalf("choice=%+v, want legal shortcut when ordinary route is unavailable", got)
	}
}

// TestEmergencyEgressCauseCoversNoRoute: GoTo's terminal "no route" error
// (world.ErrNoRoute, wrapped through GoTo's every in-map recovery attempt)
// must classify as an emergency-egress cause. Without this, a player stranded
// in a walkable component with zero graph edges out — e.g. dropped by a
// one-way ledge into a pocket whose only warp loops back on itself, measured
// on Vermilion City (12,23), issue #1553 — has no recovery: walking can never
// find a route from a component with no outgoing edges, so only the same
// Fly/Teleport/Dig emergency egress that already rescues a stalled or
// replan-exhausted journey can get it unstuck.
func TestEmergencyEgressCauseCoversNoRoute(t *testing.T) {
	wrapped := fmt.Errorf("skill: GoTo: no route from map %02x at (%d,%d) to map %02x at (%d,%d): %w",
		0x05, 12, 23, 0x08, 11, 12, world.ErrNoRoute)
	if got := emergencyEgressCause(wrapped); got != "no_route" {
		t.Fatalf("emergencyEgressCause(%v) = %q, want %q", wrapped, got, "no_route")
	}
	if got := emergencyEgressCause(errors.New("unrelated")); got != "" {
		t.Fatalf("emergencyEgressCause(unrelated) = %q, want empty", got)
	}
}

func TestCyclingRoadExplicitInputSuppressesAutoDown(t *testing.T) {
	if !cyclingRoadAutoDown(route17Map, false, false) {
		t.Fatal("Route 17 idle input did not trigger downhill coast")
	}
	if cyclingRoadAutoDown(route17Map, false, true) {
		t.Fatal("explicit Route 17 input still triggered downhill coast")
	}
	if cyclingRoadAutoDown(route17Map, true, false) {
		t.Fatal("trainer battle did not suppress Route 17 downhill coast")
	}
}


func TestVisitedIndigoPlateauIsLegalFlyDestination(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x01
	mem[sym.CurMapTileset] = overworldTileset
	mem[sym.ObtainedBadges] = 1 << 2 // Thunder Badge
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = fieldFlyMove
	setTownVisited(&mem, indigoPlateauMap)

	options := legalFastTravelOptions(&mem)
	for _, option := range options {
		if option.Kind == fastTravelFly && option.Landing.Map == indigoPlateauMap {
			return
		}
	}
	t.Fatalf("Fly options=%+v, want visited Indigo Plateau map %#02x", options, indigoPlateauMap)
}

func TestIndigoLobbyIsNamedPokemonCenterCheckpoint(t *testing.T) {
	dest, ok := Place("indigo plateau pokemon center")
	if !ok {
		t.Fatal("Indigo Plateau Pokemon Center is not a semantic place")
	}
	if dest.Map != indigoPlateauLobbyMap || dest.Kind != DestinationMap {
		t.Fatalf("Indigo Center destination=%+v, want lobby map %#02x with map-arrival semantics", dest, indigoPlateauLobbyMap)
	}
}
