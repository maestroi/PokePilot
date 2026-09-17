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
// for the staged League path. Public CI has neither the commercial ROM nor the
// derived checkpoint, so it skips there; the self-hosted qualification runner
// supplies both. Each bounded stage must positively commit its own semantic
// fact before the next stage is allowed to run.
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
	beforeFacts := redWram().DecodeStoryFacts(&before, redWram().DecodeInventory(&before))
	if beforeFacts.MainStoryComplete {
		t.Fatal("checkpoint already has main-story completion")
	}

	policy := StatAwareMove(m.ROM())
	stages := []struct {
		name string
		run  func() error
		done func(state.StoryFacts) bool
	}{
		{"LeagueStartChallenge", func() error { return LeagueStartChallenge(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueChallengeStarted }},
		{"LeagueDefeatLorelei", func() error { return LeagueDefeatLorelei(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueLoreleiDefeated }},
		{"LeagueDefeatBruno", func() error { return LeagueDefeatBruno(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueBrunoDefeated }},
		{"LeagueDefeatAgatha", func() error { return LeagueDefeatAgatha(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueAgathaDefeated }},
		{"LeagueDefeatLance", func() error { return LeagueDefeatLance(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueLanceDefeated }},
		{"LeagueDefeatChampion", func() error { return LeagueDefeatChampion(m, m.ROM(), policy) }, func(f state.StoryFacts) bool { return f.LeagueChampionDefeated }},
		{"LeagueFinishHallOfFame", func() error { return LeagueFinishHallOfFame(m) }, func(f state.StoryFacts) bool { return f.MainStoryComplete }},
	}
	for _, stage := range stages {
		if err := stage.run(); err != nil {
			t.Fatalf("%s: %v", stage.name, err)
		}
		var afterStage state.Mem
		state.Snapshot(m, &afterStage)
		facts := redWram().DecodeStoryFacts(&afterStage, redWram().DecodeInventory(&afterStage))
		if !stage.done(facts) {
			t.Fatalf("%s did not commit its semantic postcondition: %+v", stage.name, facts)
		}
	}

	var after state.Mem
	state.Snapshot(m, &after)
	afterFacts := redWram().DecodeStoryFacts(&after, redWram().DecodeInventory(&after))
	if !afterFacts.MainStoryComplete {
		t.Fatalf("main story not complete after staged League progression: %+v", afterFacts)
	}
	if !afterFacts.LeagueChampionDefeated {
		t.Fatalf("Champion semantic did not remain true after Hall of Fame reset: %+v", afterFacts)
	}
	if got := after.U8(sym.CurMap); got != hallOfFameMap {
		t.Fatalf("completion map = %#02x, want Hall of Fame %#02x", got, hallOfFameMap)
	}
}
