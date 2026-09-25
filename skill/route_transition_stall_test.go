package skill

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

type stalledTransitionExecutor struct{}

func (stalledTransitionExecutor) ExecuteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, error) {
	return world.TransitionExecutionResult{}, world.ErrTransitionExecutionStalled
}

func TestTransitionExecutionStallKeepsTypedIdentity(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: 1, To: 2, Dir: 1, BandScoped: true, BandStart: 3, BandEnd: 4}
	transition := gameruntime.Transition{ID: "red:route21_surf"}
	_, err := world.ExecuteTransition(stalledTransitionExecutor{}, edge, transition)
	if !errors.Is(err, world.ErrTransitionExecutionStalled) {
		t.Fatalf("ExecuteTransition error = %v, want ErrTransitionExecutionStalled", err)
	}
	var typed *world.TransitionExecutionError
	if !errors.As(err, &typed) || typed.Edge != edge || typed.Transition.ID != transition.ID {
		t.Fatalf("typed transition evidence lost: %#v", err)
	}
}
