package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSCoastalHMAndGymCoordinatesMatchDecomp(t *testing.T) {
	// From pokegold/maps/OlivineCafe.asm, CianwoodGym.asm,
	// CianwoodCity.asm and OlivineGym.asm.
	if gsStrengthSailorX != 4 || gsStrengthSailorY != 3 ||
		gsStrengthSailorStandX != 4 || gsStrengthSailorStandY != 4 {
		t.Fatalf("Strength sailor = (%d,%d), staging = (%d,%d)",
			gsStrengthSailorX, gsStrengthSailorY,
			gsStrengthSailorStandX, gsStrengthSailorStandY)
	}
	if gsChuckX != 4 || gsChuckY != 1 || gsChuckStandX != 4 || gsChuckStandY != 2 {
		t.Fatalf("Chuck = (%d,%d), staging = (%d,%d)",
			gsChuckX, gsChuckY, gsChuckStandX, gsChuckStandY)
	}
	// pokegold/macros/scripts/maps.asm object_const_def starts at 2, and
	// CianwoodCity.asm lists Chuck's wife as the tenth object_event → 11.
	if gsChucksWifeSlot != 11 || gsChucksWifeStandX != 10 || gsChucksWifeStandY != 45 {
		t.Fatalf("wife live object slot/stand = %d (%d,%d)",
			gsChucksWifeSlot, gsChucksWifeStandX, gsChucksWifeStandY)
	}
	if gsJasmineGymX != 5 || gsJasmineGymY != 3 ||
		gsJasmineGymStandX != 5 || gsJasmineGymStandY != 4 {
		t.Fatalf("Jasmine = (%d,%d), staging = (%d,%d)",
			gsJasmineGymX, gsJasmineGymY, gsJasmineGymStandX, gsJasmineGymStandY)
	}
}

func TestGSCoastalGymsAreOwnedWithLocalHealing(t *testing.T) {
	for _, tc := range []struct {
		city, gym, center string
	}{
		{"OLIVINE_CITY", "OLIVINE_GYM", "OLIVINE_POKECENTER_1F"},
		{"CIANWOOD_CITY", "CIANWOOD_GYM", "CIANWOOD_POKECENTER_1F"},
	} {
		city, err := gsOpeningMapID(tc.city)
		if err != nil {
			t.Fatal(err)
		}
		gym, err := gsOpeningMapID(tc.gym)
		if err != nil {
			t.Fatal(err)
		}
		center, err := gsOpeningMapID(tc.center)
		if err != nil {
			t.Fatal(err)
		}
		if !gsSecondBadgeOwnedMap(gym) || !gsSecondBadgeOwnedMap(center) {
			t.Fatalf("coastal gym ownership %s=%v center %s=%v",
				tc.gym, gsSecondBadgeOwnedMap(gym), tc.center, gsSecondBadgeOwnedMap(center))
		}
		local, err := gsOwnedPokecenterID(city)
		if err != nil || local != center {
			t.Fatalf("%s center = %#04x (%v), want %#04x", tc.city, local, err, center)
		}
		if info, ok := gsdata.Map(gym); !ok || info.Name != tc.gym {
			t.Fatalf("native catalog lacks %s: %+v", tc.gym, info)
		}
	}
	cafe, err := gsOpeningMapID("OLIVINE_CAFE")
	if err != nil || !gsSecondBadgeOwnedMap(cafe) {
		t.Fatalf("Olivine Cafe is outside owned corridor: id %#04x err=%v", cafe, err)
	}
}

// TestGSHM02FlyReplay replays the captured failure checkpoint for
// triage:a7579146e17b7296 (issue #2615) through the HM02 Fly objective and
// asserts it completes. The regression: an un-settled 3-frame A tap pressed
// immediately after Face never opened Chuck's wife's dialogue, so the
// interruption driver returned on a controllable player and the executor
// declared a terminal "returned without HM02 Fly". The fix settles before the
// press and retries a press that opens no dialogue.
//
// It is gated on the Gold ROM and a checkpoint path (GS_HM02_FLY_CHECKPOINT)
// so it runs where the capture is available and skips in the ROM-free CI gate;
// the .state capture is never committed.
func TestGSHM02FlyReplay(t *testing.T) {
	rom := os.Getenv("POKEMON_GOLD_ROM")
	checkpoint := os.Getenv("GS_HM02_FLY_CHECKPOINT")
	if rom == "" || checkpoint == "" {
		t.Skip("set POKEMON_GOLD_ROM and GS_HM02_FLY_CHECKPOINT to replay the captured failure")
	}
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	m, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("load state: %v", err)
	}
	obj := Objective{
		Kind:     KindProgress,
		Progress: ProgressID("gs_hm02_fly_acquired"),
		Starter:  skill.StarterCharmander,
	}
	result, execErr := ExecuteWithRoutePriority(m, m.ROM(), obj, RoutePriorityConservative)
	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if result.Outcome != OutcomeCompleted {
		t.Fatalf("outcome=%s cause=%s summary=%s, want completed",
			result.Outcome, result.Cause, result.Summary)
	}
}
