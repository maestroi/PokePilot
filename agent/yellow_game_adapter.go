package agent

import (
	"errors"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

var errYellowControllerUnavailable = errors.New("no Pokémon Yellow controller owns this story goal yet")

// yellowObjectiveAdapter runs Yellow objectives. The verbs the shared Gen-I
// engine owns (travel, talk, trainers, gyms, healing, training, capture,
// items, shopping, field-move repair) execute through that engine: Yellow's
// cartridge binds the canonical memory view and its ROM tables, so the same
// controllers drive it. The opening and story progression are Yellow's own
// (Pikachu, Jessie & James, a different rival) and stay Yellow-owned: they run
// through the Yellow story controller (yellow_story.go, yellow/story), and a
// story goal no Yellow controller owns yet fails as a typed, non-recoverable
// block rather than replaying Red's story scripts.
type yellowObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
	gen1    *redObjectiveAdapter
}

func newYellowObjectiveAdapter(m *emu.Emu, romData []byte, priority RoutePriority) *yellowObjectiveAdapter {
	gen1 := newRedObjectiveAdapterWithRoutePriority(m, romData, priority)
	gen1.gameID = yellowprofile.GameID
	return &yellowObjectiveAdapter{m: m, romData: romData, gen1: gen1}
}

func init() {
	registerObjectiveAdapterFactory(yellowprofile.GameID, func(m *emu.Emu, romData []byte, priority RoutePriority) ObjectiveGameAdapter {
		return newYellowObjectiveAdapter(m, romData, priority)
	})
	registerObjectiveCatalogProvider(yellowprofile.GameID, &yellowObjectiveAdapter{})
}

// yellowOwnedKind reports the objective kinds whose semantics are Yellow's
// story rather than the shared Gen-I engine's.
func yellowOwnedKind(kind Kind) bool {
	return kind == KindStarter || kind == KindProgress
}

// Observe is Yellow's own observation: the profile's Yellow story projection
// must not be replaced by Red's story decoder (redObjectiveAdapter.Observe).
func (a *yellowObjectiveAdapter) Observe() (Observation, error) {
	return ObserveChecked(a.m, a.romData)
}

func (a *yellowObjectiveAdapter) Validate(o Objective, obs Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if yellowOwnedKind(o.Kind) {
		return validateYellowOwned(o)
	}
	return a.gen1.Validate(o, obs)
}

func (a *yellowObjectiveAdapter) NormalizeBoundary() error {
	return a.gen1.NormalizeBoundary()
}

// ObserveBattleTurns implements BattleTurnObservingAdapter.
func (a *yellowObjectiveAdapter) ObserveBattleTurns(observer BattleTurnObserver) {
	a.gen1.ObserveBattleTurns(observer)
}

func (a *yellowObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	if yellowOwnedKind(o.Kind) {
		restoreMoveObserver := skill.WithMoveObserver(a.m, gen1MoveObserver(a.romData, a.gen1.battleTurns))
		defer restoreMoveObserver()
		return executeYellowOwned(a.m, a.romData, o)
	}
	return a.gen1.ExecuteOwned(o)
}

func (a *yellowObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	return a.gen1.WithinObjectiveBudget(o, fn)
}

func (a *yellowObjectiveAdapter) SettlePostcondition(o Objective) error {
	return a.gen1.SettlePostcondition(o)
}

func (a *yellowObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	if o.Kind == KindStarter {
		return verifyYellowStarterPostcondition(o, final)
	}
	if yellowOwnedKind(o.Kind) {
		_, err := verifyObjectivePostcondition(o, initial, final, result)
		return err
	}
	return a.gen1.VerifyPostcondition(o, initial, final, result)
}

func (a *yellowObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, final Observation) gameruntime.Failure {
	if failure, ok := normalizeYellowStoryFailure(phase, err); ok {
		return failure
	}
	return a.gen1.NormalizeFailure(phase, err, final)
}

func (a *yellowObjectiveAdapter) CaptureFailure(o Objective, err error) error {
	// Forensic dumps record native RAM, and the decoded summary reads the
	// canonical view, so the shared Gen-I capture is safe on Yellow.
	return a.gen1.CaptureFailure(o, err)
}

// ProgressionObjectives implements ProgressionPlanner with Yellow's own story
// goals; Red's story registry is never consulted for Yellow.
func (a *yellowObjectiveAdapter) ProgressionObjectives(obs Observation) []Objective {
	return yellowProgressionObjectives(obs)
}

func (a *yellowObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	return yellowObjectiveCatalog(obs)
}

// yellowObjectiveCatalog is the shared Gen-I catalog with Yellow's own facts:
// its map vocabulary, and the scripted Pikachu opening in place of Oak's
// three-ball choice. Story goals come from ProgressionObjectives, and only
// the ones the Yellow controller owns.
func yellowObjectiveCatalog(obs Observation) ObjectiveCatalog {
	facts := gen1CatalogFacts{Location: yellowLocationID}
	if obs.PartyCount == 0 {
		facts.Starters = []CatalogStarter{{Species: "pikachu"}}
	}
	return gen1ObjectiveCatalog(obs, facts)
}
