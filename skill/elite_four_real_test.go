package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestEliteFourProgressionQualification is the private checkpoint qualification
// for #38. Public CI has neither the commercial ROM nor the derived checkpoint,
// so it skips there; the self-hosted qualification runner supplies both.
func TestEliteFourProgressionQualification(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed qualification")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	corpus := os.Getenv("POKEPILOT_QUALIFICATION_CORPUS")
	if romPath == "" || corpus == "" {
		t.Skip("POKEMON_RED_ROM and POKEPILOT_QUALIFICATION_CORPUS are required")
	}

	checkpoint := filepath.Join(corpus, "elite-four-champion", "start.state")
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint %s: %v", checkpoint, err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if got := before.U8(sym.CurMap); got != indigoPlateauLobbyMap {
		t.Fatalf("checkpoint map = %#02x, want Indigo lobby %#02x", got, indigoPlateauLobbyMap)
	}
	beforeFacts := state.DecodeStoryFacts(&before, state.DecodeInventory(&before))
	if beforeFacts.MainStoryComplete {
		t.Fatal("checkpoint already has main-story completion")
	}

	if err := EliteFourProgression(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("EliteFourProgression: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterFacts := state.DecodeStoryFacts(&after, state.DecodeInventory(&after))
	if !afterFacts.MainStoryComplete {
		t.Fatalf("main story not complete after League progression: %+v", afterFacts)
	}
	if !afterFacts.LeagueChampionDefeated {
		t.Fatalf("Champion semantic did not remain true after Hall of Fame reset: %+v", afterFacts)
	}
	if got := after.U8(sym.CurMap); got != hallOfFameMap {
		t.Fatalf("completion map = %#02x, want Hall of Fame %#02x", got, hallOfFameMap)
	}
}
