package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestHallOfFamePostCompletionNeedsSoftResetEgress pins the shared invariant
// behind run-d6dokr184ky81: after the durable Hall-of-Fame bit is set, the
// world graph still offers HoF -> Champion -> Lance, but Lance's Champion
// entrance is a disconnected north pocket, so Kanto destinations stay
// world: no_route. Dig/Escape Rope are illegal on the HoF tileset. The only
// cartridge exit is soft-reset + CONTINUE; navigation must recognize that
// start boundary rather than pathfind from the stale CurMap.
func TestHallOfFamePostCompletionNeedsSoftResetEgress(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	mem := leagueMem(t, true)
	mem[sym.CurMap] = hallOfFameMap
	mem[sym.XCoord] = 4
	mem[sym.YCoord] = 2
	mem[sym.UpdateSpritesEnabled] = 1
	mem[sym.CurMapTileset] = 7 // HALL_OF_FAME gym tileset; escape travel illegal

	if escapeTravelAllowed(mem) {
		t.Fatal("Hall of Fame tileset unexpectedly allows Dig/Escape Rope")
	}

	round := leagueRoutePrereqs(t, romData, mem)
	pallet, ok := Place("pallet town")
	if !ok {
		t.Fatal("pallet town place is not registered")
	}
	_, err := findRoutePlanForDestination(round.graph, hallOfFameMap, 4, 2, pallet, nil, round.prereqs)
	if err == nil {
		t.Fatal("HoF -> Pallet unexpectedly routed; soft-reset egress would be unnecessary")
	}

	// Post-Init title boundary: sprites $ff is the positive Init proof even
	// when CurMap still reads as Hall of Fame.
	mem[sym.UpdateSpritesEnabled] = initSpritesDisabled
	if mem.U8(sym.UpdateSpritesEnabled) != initSpritesDisabled {
		t.Fatal("test setup failed to project post-Init sprites")
	}
	if !state.MainStoryComplete(mem) {
		t.Fatal("test setup missing main-story completion")
	}
}

// TestControllableSpritesDisabledIsNotTitleResume pins the shared invariant
// behind run-2f5bbo4uwzhnv5w20i59sq17l / triage:2d686ba194d84d8a and
// run-3ftqlmhp1tmm3cpzdasspkv2o / triage:db176fe578912837: after PromoteToLead
// closes the START menu — or VirtualTrade returns to a Controllable Trade
// Center room — wUpdateSpritesEnabled can remain $ff while Controllable still
// reads true. That is a leftover menu/overworld boundary, not post-Init
// title/main-menu. softResetToSave must not skip A+B+START+SELECT there, and
// ensureNavigableOverworld must not treat it as a title resume: both paths
// would Start/A-hunt into the START menu / Pokédex ("BULBASAUR SEED") and fail
// with "main menu did not appear".
func TestControllableSpritesDisabledIsNotTitleResume(t *testing.T) {
	mem := state.Mem{}
	mem[sym.CurMap] = 0x85 // Celadon Pokemon Center
	mem[sym.CurMapWidth] = 4
	mem[sym.CurMapHeight] = 4
	mem[sym.UpdateSpritesEnabled] = initSpritesDisabled
	mem[sym.IsInBattle] = 0
	mem[sym.FontLoaded] = 0
	mem[sym.JoyIgnore] = 0
	mem[sym.WalkCounter] = 0
	mem[sym.CurOpponent] = 0
	mem[sym.StatusFlags4] = 0
	mem[sym.StatusFlags6] = 0
	mem[sym.EnteringCableClub] = 0
	if !state.Controllable(&mem) {
		t.Fatal("test setup must be Controllable with sprites disabled")
	}
	if state.DecodeBattle(&mem) != nil {
		t.Fatal("test setup must not decode as a live battle")
	}
	// Controllable + $ff must not look like SoftReset/Init, so softResetToSave
	// keeps the four-button hold and ensureNavigableOverworld does not resume.
	if postInitTitleBoundary(&mem) {
		t.Fatal("controllable sprites-$ff boundary unexpectedly looks like post-Init title")
	}
}

// TestStartMenuSpritesDisabledIsNotTitleResume pins the failure-frame shape
// from run-3ftqlmhp1tmm3cpzdasspkv2o: Trade Center START menu with FontLoaded
// set. Controllable is false, but this is still not SoftReset/Init — skipping
// the four-button hold would Start/A-hunt inside the same menu.
func TestStartMenuSpritesDisabledIsNotTitleResume(t *testing.T) {
	mem := state.Mem{}
	mem[sym.CurMap] = 0xef // Trade Center
	mem[sym.CurMapWidth] = 5
	mem[sym.CurMapHeight] = 4
	mem[sym.UpdateSpritesEnabled] = initSpritesDisabled
	mem[sym.IsInBattle] = 0
	mem[sym.FontLoaded] = 1
	mem[sym.JoyIgnore] = 0
	mem[sym.WalkCounter] = 0
	mem[sym.CurOpponent] = 0
	mem[sym.StatusFlags4] = 0
	mem[sym.StatusFlags6] = 0
	mem[sym.EnteringCableClub] = 0
	mem[sym.CurrentMenuItem] = 0
	mem[sym.MaxMenuItem] = 7
	mem[sym.TopMenuItemX] = 0x0b
	mem[sym.TopMenuItemY] = 0x02
	if state.Controllable(&mem) {
		t.Fatal("START menu setup must not be Controllable")
	}
	if postInitTitleBoundary(&mem) {
		t.Fatal("START menu sprites-$ff boundary unexpectedly looks like post-Init title")
	}
}

// TestPostInitTitleBoundaryRequiresDisabledSprites pins the genuine SoftReset
// skip surface: sprites $ff, no battle, not controllable, font clear, no menu.
func TestPostInitTitleBoundaryRequiresDisabledSprites(t *testing.T) {
	mem := state.Mem{}
	mem[sym.UpdateSpritesEnabled] = initSpritesDisabled
	mem[sym.IsInBattle] = 0
	mem[sym.FontLoaded] = 0
	mem[sym.JoyIgnore] = 0
	mem[sym.WalkCounter] = 0
	mem[sym.CurOpponent] = 0
	mem[sym.StatusFlags4] = 0
	mem[sym.StatusFlags6] = 0
	mem[sym.EnteringCableClub] = 0
	// Cleared WRAM map dimensions keep Controllable false after Init.
	mem[sym.CurMapWidth] = 0
	mem[sym.CurMapHeight] = 0
	if state.Controllable(&mem) {
		t.Fatal("post-Init setup must not be Controllable")
	}
	if !postInitTitleBoundary(&mem) {
		t.Fatal("post-Init sprites-$ff boundary must authorize soft-reset skip")
	}
	mem[sym.UpdateSpritesEnabled] = 1
	if postInitTitleBoundary(&mem) {
		t.Fatal("sprites-enabled overworld must not look like post-Init title")
	}
}
