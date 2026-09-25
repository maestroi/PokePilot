package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestConnectionCrossingCandidateBudgetCoversWholeScopedBand(t *testing.T) {
	grid := &world.Grid{Width: 100, Height: 18}
	edge := world.Edge{
		Kind:       world.EdgeConnection,
		Dir:        3,
		BandScoped: true,
		BandStart:  2,
		BandEnd:    16,
	}
	if got := connectionCrossingCandidateBudget(grid, edge); got != 15 {
		t.Fatalf("candidate budget = %d, want 15 for band 2..16", got)
	}
}

func TestConnectionCrossingCandidateBudgetUsesWholeUnscopedEdge(t *testing.T) {
	grid := &world.Grid{Width: 100, Height: 18}
	if got := connectionCrossingCandidateBudget(grid, world.Edge{Kind: world.EdgeConnection, Dir: 0}); got != 100 {
		t.Fatalf("north-edge candidate budget = %d, want width 100", got)
	}
	if got := connectionCrossingCandidateBudget(grid, world.Edge{Kind: world.EdgeConnection, Dir: 3}); got != 18 {
		t.Fatalf("east-edge candidate budget = %d, want height 18", got)
	}
}

func TestConnectionCrossingCandidateBudgetClampsScopedBandToGrid(t *testing.T) {
	grid := &world.Grid{Width: 10, Height: 6}
	edge := world.Edge{
		Kind:       world.EdgeConnection,
		Dir:        3,
		BandScoped: true,
		BandStart:  4,
		BandEnd:    20,
	}
	if got := connectionCrossingCandidateBudget(grid, edge); got != 2 {
		t.Fatalf("clamped candidate budget = %d, want 2 for rows 4..5", got)
	}
}
