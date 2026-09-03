package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func offeredGym(obs Observation, known *Knowledge) (Objective, bool) {
	for _, o := range Offer(obs, known) {
		if o.Kind == KindGym {
			return o, true
		}
	}
	return Objective{}, false
}

func pewterGymObservation() Observation {
	return Observation{
		Map: 0x36, MapName: "PEWTER_GYM", X: 4, Y: 2, PartyCount: 1,
		Party: []PartyMon{{Level: 9, HP: 28, MaxHP: 28}},
	}
}

// TestGymLossRequiresTrainingBeforeRechallenge pins the live Brock loop from
// #67: a leader loss followed by a successful walk back must not make the
// unchanged challenge available again. The first attempt is still allowed at
// any level; only evidence from an actual loss creates the recovery gate.
func TestGymLossRequiresTrainingBeforeRechallenge(t *testing.T) {
	pewter := pewterGymObservation()
	known := NewKnowledge(nil)
	known.SawMap(pewter.Map)

	gym, ok := offeredGym(pewter, known)
	if !ok {
		t.Fatal("fresh Pewter Gym: challenge not offered")
	}
	if gym.Place != "pewter gym" {
		t.Fatalf("offered gym Place = %q, want the deterministic gym identity %q", gym.Place, "pewter gym")
	}

	loss := gymOutcomeErr(gym, state.ResultLost)
	if loss == nil {
		t.Fatal("gymOutcomeErr(ResultLost) = nil")
	}
	known.Failed(gym, loss)

	key := gymLossFailureKey("pewter gym")
	if got := known.Failures[key]; got.Times != 1 {
		t.Fatalf("scoped gym failure = %+v, want one failure under %q", got, key)
	}
	if _, ok := offeredGym(pewter, known); ok {
		t.Fatal("Pewter Gym offered immediately after losing to Brock with unchanged progression")
	}

	// These are the two successful rounds that made the live loop invisible
	// to Run's consecutive-failure guard. Neither changes battle readiness.
	known.Done(Objective{Kind: KindGoTo, Place: "pewter gym"})
	known.Done(Objective{Kind: KindHeal})
	if _, ok := offeredGym(pewter, known); ok {
		t.Fatal("successful travel/heal cleared the Brock loss gate; the commute loop can return")
	}

	// The gate is scoped to the gym that beat the party. A Brock loss must
	// not globally disable a different legal gym challenge.
	cerulean := Observation{
		Map: 0x41, MapName: "CERULEAN_GYM", X: 4, Y: 3, PartyCount: 1,
		Party:  []PartyMon{{Level: 9, HP: 28, MaxHP: 28}},
		Badges: []string{"Boulder"},
	}
	other, ok := offeredGym(cerulean, known)
	if !ok {
		t.Fatal("Brock loss blocked the distinct Cerulean Gym challenge")
	}
	if other.Place != "cerulean gym" {
		t.Fatalf("Cerulean gym Place = %q, want %q", other.Place, "cerulean gym")
	}

	known.Done(Objective{Kind: KindTrain, Level: 11})
	if _, blocked := known.Failures[key]; blocked {
		t.Fatalf("successful training left gym recovery failure %q behind", key)
	}
	if _, ok := offeredGym(pewter, known); !ok {
		t.Fatal("Pewter Gym not re-enabled after a successful training rung")
	}
}

// TestGymControlFailureDoesNotDemandTraining ensures the recovery gate means
// what it says. A path/menu/control defect is not evidence that the party is
// underpowered, so the same challenge remains available for ordinary recovery
// instead of forcing an unrelated grind.
func TestGymControlFailureDoesNotDemandTraining(t *testing.T) {
	pewter := pewterGymObservation()
	known := NewKnowledge(nil)
	gym, ok := offeredGym(pewter, known)
	if !ok {
		t.Fatal("fresh Pewter Gym: challenge not offered")
	}

	known.Failed(gym, errors.New("agent: beat the gym leader here: skill: Gym: movement blocked"))
	if _, ok := offeredGym(pewter, known); !ok {
		t.Fatal("non-battle gym failure was mistaken for a leader loss and forced training")
	}
}

// TestGymLossGateSurvivesCheckpointMemory proves resume cannot forget the
// learned loss. The gate is encoded as an ordinary scoped Failure, so it uses
// the existing versioned checkpoint-memory path without a new side channel.
func TestGymLossGateSurvivesCheckpointMemory(t *testing.T) {
	pewter := pewterGymObservation()
	known := NewKnowledge(nil)
	gym, ok := offeredGym(pewter, known)
	if !ok {
		t.Fatal("fresh Pewter Gym: challenge not offered")
	}
	known.Failed(gym, gymOutcomeErr(gym, state.ResultLost))

	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-010-frame-0001234567-gym.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMemoryFile(statePath, known, "get stronger before retrying the gym", 0); err != nil {
		t.Fatalf("writeMemoryFile: %v", err)
	}

	resumed := LoadCheckpointMemory(statePath, nil, nil)
	if _, ok := offeredGym(pewter, resumed.Knowledge); ok {
		t.Fatal("checkpoint resume forgot the Brock loss gate")
	}
	resumed.Knowledge.Done(Objective{Kind: KindTrain, Level: 11})
	if _, ok := offeredGym(pewter, resumed.Knowledge); !ok {
		t.Fatal("training did not unlock the resumed gym-loss gate")
	}
}
