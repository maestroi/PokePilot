package fixture

import (
	"os"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

func TestResolveDirHonoursEnvOverride(t *testing.T) {
	t.Setenv("POKEPILOT_FIXTURE_DIR", "/tmp/pokepilot-fixtures-test")
	if got := ResolveDir(); got != "/tmp/pokepilot-fixtures-test" {
		t.Fatalf("ResolveDir() = %q, want the env override", got)
	}
}

func TestResolveDirDefaultsToRepoPath(t *testing.T) {
	t.Setenv("POKEPILOT_FIXTURE_DIR", "")
	if got := ResolveDir(); got != "testdata/fixtures" {
		t.Fatalf("ResolveDir() = %q, want the in-repo default", got)
	}
}

func checkOverworld(t *testing.T, e *emu.Emu) {
	t.Helper()
	var m state.Mem
	state.Snapshot(e, &m)
	gs := state.Decode(&m)
	if gs.Player.MapID != 0x26 {
		t.Errorf("MapID = %#04x, want 0x26", gs.Player.MapID)
	}
	if gs.Player.X != 3 || gs.Player.Y != 6 {
		t.Errorf("coords = (%d,%d), want (3,6)", gs.Player.X, gs.Player.Y)
	}
}

func TestLoadGeneratesAndCaches(t *testing.T) {
	e1 := Load(t, "reds_bedroom")
	if _, err := os.Stat(fixturePath("reds_bedroom")); err != nil {
		t.Fatalf("fixture file not created after first Load: %v", err)
	}
	checkOverworld(t, e1)

	e2 := Load(t, "reds_bedroom")
	checkOverworld(t, e2)
}

// TestLoadStatePlain: the non-test entry point is the same validated,
// versioned cache Load uses, and it reports problems as errors instead of
// failing a test — so code outside a test (a replay of a failed run) can
// stand on it.
func TestLoadStatePlain(t *testing.T) {
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot generate or load fixture")
	}
	e, err := LoadState("reds_bedroom")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	defer e.Close()
	checkOverworld(t, e)

	// An unregistered name is an error, not a panic and not a nil emu.
	if _, err := LoadState("no_such_fixture"); err == nil {
		t.Fatal("LoadState of an unregistered name returned a nil error")
	}
}

func TestLoadRejectsPoisonedFixture(t *testing.T) {
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot write poisoned fixture")
	}
	// A state stepped only 60 frames is still on the title screen with zero
	// map dimensions: deliberately invalid fixture content.
	e, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	e.StepFrames(60)
	b, err := e.SaveState()
	if err != nil {
		e.Close()
		t.Fatalf("SaveState: %v", err)
	}
	e.Close()
	if err := os.MkdirAll(ResolveDir(), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", ResolveDir(), err)
	}
	path := fixturePath("reds_bedroom")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write poisoned fixture: %v", err)
	}

	loaded := Load(t, "reds_bedroom")
	var m state.Mem
	state.Snapshot(loaded, &m)
	if !state.Controllable(&m) {
		t.Fatal("Load returned a non-controllable state; poisoned fixture was trusted instead of regenerated")
	}
	checkOverworld(t, loaded)
}

func TestFixtureIsFast(t *testing.T) {
	Load(t, "reds_bedroom")
	start := time.Now()
	Load(t, "reds_bedroom")
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("second Load took %v, want < 2s (cache should make it fast)", d)
	}
}

func TestFixtureStateIsControllable(t *testing.T) {
	e := Load(t, "reds_bedroom")
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.Controllable(&m) {
		t.Error("Controllable = false, want true")
	}
}

// TestCheckpointFixturesAtPlace: each post-story fixture must land exactly
// on the Place entry it was built from, and every fixture must be
// controllable.
func TestCheckpointFixturesAtPlace(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		place   string
	}{
		{"pallet_town", "pallet town"},
		{"viridian_city", "viridian city"},
		{"viridian_pokecenter", "viridian pokemon center"},
		{"pewter_city", "pewter city"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			e := Load(t, tc.fixture)
			dest, ok := skill.Place(tc.place)
			if !ok {
				t.Fatalf(`Place: %q not found`, tc.place)
			}
			var m state.Mem
			state.Snapshot(e, &m)
			if !state.Controllable(&m) {
				t.Fatal("Controllable = false, want true")
			}
			gs := state.Decode(&m)
			if gs.Player.MapID != dest.Map {
				t.Errorf("MapID = %#04x, want %#04x (%s)", gs.Player.MapID, dest.Map, tc.place)
			}
			if gs.Player.X != dest.X || gs.Player.Y != dest.Y {
				t.Errorf("coords = (%d,%d), want (%d,%d) (%s)", gs.Player.X, gs.Player.Y, dest.X, dest.Y, tc.place)
			}
		})
	}
}

// TestPostStarterFixture: the story fixture has no Place entry (the player
// ends in Oak's lab), so assert the story flag instead of coordinates.
func TestPostStarterFixture(t *testing.T) {
	e := Load(t, "post_starter")
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.HasEvent(&m, state.EventBattledRivalInOaksLab) {
		t.Error("EventBattledRivalInOaksLab not set; fixture is not post-story")
	}
	if !state.Controllable(&m) {
		t.Error("Controllable = false, want true")
	}
}

// TestForestNorthGateFixture: the north-gate fixture has no Place entry
// (the gate tile is not a journey destination), so assert the gate
// coordinates and the grind progress itself: the player stands at the gate
// and the lead is at the level that beats Brock. These are the positive
// contracts the gym tests and the S10-9 talk test rely on — "controllable"
// alone would happily pass on a state parked anywhere else on the road.
func TestForestNorthGateFixture(t *testing.T) {
	e := Load(t, "forest_north_gate")
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.Controllable(&m) {
		t.Error("Controllable = false, want true")
	}
	gs := state.Decode(&m)
	if gs.Player.MapID != 0x2F {
		t.Errorf("MapID = %#04x, want 0x2f (the Viridian Forest north gate)", gs.Player.MapID)
	}
	if gs.Player.X != 5 || gs.Player.Y != 1 {
		t.Errorf("coords = (%d,%d), want (5,1)", gs.Player.X, gs.Player.Y)
	}
	if lead := state.DecodeParty(&m).Mons[0]; int(lead.Level) < 12 {
		t.Errorf("lead is level %d, want >= 12 (the level that beats Brock)", lead.Level)
	}
}

// TestPostBoulderFixture: the badge fixture must stand at Place("pewter
// city") with the Boulder Badge set — the positive progress the Cascade
// test relies on, not merely the absence of a bad state.
func TestPostBoulderFixture(t *testing.T) {
	e := Load(t, "post_boulder")
	var m state.Mem
	state.Snapshot(e, &m)
	if !state.Controllable(&m) {
		t.Error("Controllable = false, want true")
	}
	dest, ok := skill.Place("pewter city")
	if !ok {
		t.Fatal(`Place: "pewter city" not found`)
	}
	gs := state.Decode(&m)
	if gs.Player.MapID != dest.Map || gs.Player.X != dest.X || gs.Player.Y != dest.Y {
		t.Errorf("player is on map %#04x at (%d,%d), want map %#04x at (%d,%d) (Pewter City)",
			gs.Player.MapID, gs.Player.X, gs.Player.Y, dest.Map, dest.X, dest.Y)
	}
	if !state.DecodeProgress(&m).Has(state.BadgeBoulder) {
		t.Error("the Boulder Badge is not set; the fixture is not post-Brock")
	}
}
