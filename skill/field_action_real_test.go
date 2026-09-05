package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// loadPreparedFieldActionState loads an externally captured real-ROM state.
// These are intentionally not committed: .state files contain derived game
// data and the repository's fixture policy forbids checking them in. Farm or
// local checkpoints are suitable inputs.
func loadPreparedFieldActionState(t *testing.T, env string) *emu.Emu {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skipf("%s not set (real-ROM prepared-state test)", env)
	}
	m := openEmu(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s=%s: %v", env, path, err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatalf("load %s=%s: %v", env, path, err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		t.Fatalf("%s state is not a controllable overworld", env)
	}
	return m
}

// TestUseFieldMoveSurfRealROM requires a state standing beside surfable water
// with the Soul Badge and either learned Surf or HM03 plus a compatible party
// member. It proves the full START -> POKEMON -> SURF path and checks the
// game's actual wWalkBikeSurfState, not a timing assumption.
func TestUseFieldMoveSurfRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SURF_TEST_STATE")
	var before state.Mem
	state.Snapshot(m, &before)
	cap := FieldCapabilityFor(&before, FieldSurf)
	if !cap.Usable && !CanPrepareFieldMove(m.ROM(), &before, FieldSurf) {
		t.Fatalf("prepared Surf state cannot use or prepare Surf: %+v", cap)
	}

	result, err := UseFieldMove(m, FieldSurf)
	if err != nil {
		t.Fatalf("UseFieldMove(Surf): %v", err)
	}
	if !result.Surfing || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		t.Fatalf("Surf result=%+v wWalkBikeSurfState=%d, want surfing state %d", result, m.Peek8(sym.WalkBikeSurfState), fieldSurfingState)
	}
}

// TestUseFieldMoveStrengthRealROM requires a controllable state facing a live
// boulder with the Rainbow Badge and either learned Strength or HM04 plus a
// compatible party member. It verifies the ROM's BIT_STRENGTH_ACTIVE flag.
func TestUseFieldMoveStrengthRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_STRENGTH_TEST_STATE")
	var before state.Mem
	state.Snapshot(m, &before)
	if !boulderAhead(&before) {
		t.Fatal("prepared Strength state is not facing a live boulder")
	}
	cap := FieldCapabilityFor(&before, FieldStrength)
	if !cap.Usable && !CanPrepareFieldMove(m.ROM(), &before, FieldStrength) {
		t.Fatalf("prepared Strength state cannot use or prepare Strength: %+v", cap)
	}

	result, err := UseFieldMove(m, FieldStrength)
	if err != nil {
		t.Fatalf("UseFieldMove(Strength): %v", err)
	}
	if !result.StrengthActive || m.Peek8(sym.StatusFlags1)&fieldStrengthActiveBit == 0 {
		t.Fatalf("Strength result=%+v wStatusFlags1=%#02x, want active bit", result, m.Peek8(sym.StatusFlags1))
	}
}
