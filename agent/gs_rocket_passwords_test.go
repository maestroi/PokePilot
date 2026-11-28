package agent

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSRocketB3FObjectSlotsMatchDecomp(t *testing.T) {
	// pokegold/macros/scripts/maps.asm object_const_def starts at 2, and
	// TeamRocketBaseB3F.asm lists Murkrow / SlowpokeTail / RaticateTail as
	// object_events 3..5 → const indexes 4..6.
	if gsRocketMurkrowSlot != 4 || gsRocketSlowpokeGruntSlot != 5 || gsRocketRaticateGruntSlot != 6 {
		t.Fatalf("B3F object slots = murkrow %d slowpoke %d raticate %d",
			gsRocketMurkrowSlot, gsRocketSlowpokeGruntSlot, gsRocketRaticateGruntSlot)
	}
}

func TestGSRocketB3FOrthogonalNeighborFollowsMovedTrainer(t *testing.T) {
	// Sight-range trainers walk off their ROM home before talk-after. The
	// preferred stand beside home is then wrong; stage beside the live tile.
	sx, sy := gsOrthogonalNeighborNear(7, 15, 5, 14, 5, 14)
	if !gsOrthogonallyAdjacent(sx, sy, 7, 15) {
		t.Fatalf("stand (%d,%d) is not beside live (7,15)", sx, sy)
	}
	if gsOrthogonallyAdjacent(5, 14, 7, 15) {
		t.Fatal("preferred home stand must not still count as beside a moved grunt")
	}
}

// TestGSRocketRaticatePasswordLiveTalkReplay replays farm
// run-2lo95absd51sr2nj6z69foc0sc (triage dd12508a00d84aca / issue #2822):
// the RaticateTail grunt's sight script leaves him off (5,15), so facing the
// ROM home never runs the endifjustbattled password setevent. Gate on the
// Gold ROM and a private checkpoint path; the .state is never committed.
func TestGSRocketRaticatePasswordLiveTalkReplay(t *testing.T) {
	rom := os.Getenv("POKEMON_GOLD_ROM")
	checkpoint := os.Getenv("GS_ROCKET_RATICATE_PASSWORD_CHECKPOINT")
	if rom == "" || checkpoint == "" {
		t.Skip("set POKEMON_GOLD_ROM and GS_ROCKET_RATICATE_PASSWORD_CHECKPOINT to replay the captured failure")
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
		Progress: ProgressID(gsprofile.ProgressRocketPasswordsLearned),
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

func TestGSRocketB3FProgressionNeverSkipsPasswordsOrExecutive(t *testing.T) {
	for _, id := range []string{string(gsprofile.GoldGameID), string(gsprofile.SilverGameID)} {
		adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
		if id == string(gsprofile.SilverGameID) {
			adapter = newGSObjectiveAdapter(nil, nil, gsprofile.SilverGameID)
		}
		obs := Observation{GameID: adapter.gameID, PartyCount: 1,
			Story: ProgressState{
				{ID: gsprofile.ProgressStarterReceived, Complete: true},
				{ID: gsprofile.ProgressMineralBadgeEarned, Complete: true},
				{ID: gsprofile.ProgressRedGyaradosDefeated, Complete: true},
				{ID: gsprofile.ProgressLanceHelpAccepted, Complete: true},
				{ID: gsprofile.ProgressMahoganyStairsOpened, Complete: true},
				{ID: gsprofile.ProgressRocketLanceHealing, Complete: true},
			},
		}
		// The production planner is linear: having reached B2F does not
		// magically satisfy all earlier Johto prerequisites in a unit fixture.
		// Simulate the actual prerequisite chain before asserting this slice.
		reached := false
		for step := 0; step < 40; step++ {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 {
				t.Fatalf("%s has no prerequisite stage at step %d: %+v", id, step, goals)
			}
			if goals[0].Progress == gsprofile.ProgressRocketPasswordsLearned {
				reached = true
				break
			}
			obs.Story = append(obs.Story, ProgressFact{ID: goals[0].Progress, Complete: true})
		}
		if !reached {
			t.Fatalf("%s did not reach B3F through the story prerequisites", id)
		}
		for _, next := range []ProgressID{
			gsprofile.ProgressRocketPasswordsLearned,
			gsprofile.ProgressRocketOfficeOpened,
			gsprofile.ProgressRocketExecutiveBeaten,
			gsprofile.ProgressRocketMurkrowPassword,
			gsprofile.ProgressRocketTransmitterDoorOpened,
			gsprofile.ProgressRocketExecutiveFBeaten,
			gsprofile.ProgressRocketHideoutCleared,
		} {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) != 1 || goals[0].Progress != next {
				t.Fatalf("%s story returned %+v, want %s", id, goals, next)
			}
			if !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("%s objective %s is not executable", id, next)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: next, Complete: true})
		}
		if goals := adapter.ProgressionObjectives(obs); len(goals) != 1 ||
			goals[0].Progress != gsprofile.ProgressGlacierBadgeEarned {
			t.Fatalf("%s must offer experimental Pryce only after Rocket clear: %+v", id, goals)
		}
	}
}
