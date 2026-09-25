package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestConnectionTargetFailureScopesComponentBandToEdge(t *testing.T) {
	scoped := world.Edge{
		Kind:       world.EdgeConnection,
		From:       0x1f,
		To:         0x13,
		Dir:        3,
		BandScoped: true,
		BandStart:  2,
		BandEnd:    16,
	}
	err := connectionTargetFailure(scoped, errors.New("no reachable candidate"))
	if !errors.Is(err, ErrConnectionBandExhausted) {
		t.Fatalf("scoped failure = %v, want ErrConnectionBandExhausted", err)
	}
	edge, tile := legFailureBanScope(err)
	if !edge || tile {
		t.Fatalf("scoped failure ban scope = edge:%t tile:%t, want edge only", edge, tile)
	}
	if legFailureConsumesReplanBudget(err) {
		t.Fatal("scoped zero-candidate band must not spend transient replan budget")
	}
}

func TestConnectionTargetFailureKeepsUnscopedEdgePositionLocal(t *testing.T) {
	unscoped := world.Edge{Kind: world.EdgeConnection, From: 0x0d, To: 0x01, Dir: 1}
	err := connectionTargetFailure(unscoped, errors.New("ledge blocks this landing"))
	if !errors.Is(err, ErrLegUnwalkable) || errors.Is(err, ErrConnectionBandExhausted) {
		t.Fatalf("unscoped failure = %v, want ordinary ErrLegUnwalkable", err)
	}
	edge, tile := legFailureBanScope(err)
	if edge || !tile {
		t.Fatalf("unscoped failure ban scope = edge:%t tile:%t, want tile only", edge, tile)
	}
}
