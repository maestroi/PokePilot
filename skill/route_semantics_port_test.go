package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/world"
)

func TestSemanticTransitionMarksSurfAsPortBypass(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: semanticPalletTownMap, To: semanticRoute21Map}

	surf := semanticTransition("test:surf", edge, capCanSurf)
	if !surf.PortBypass {
		t.Fatalf("Surf transition did not own its map-edge port: %+v", surf)
	}

	cut := semanticTransition("test:cut", edge, capCanCut)
	if cut.PortBypass {
		t.Fatalf("Cut transition incorrectly owns a dead map-edge port: %+v", cut)
	}

	snorlax := semanticTransition("test:snorlax", edge, capCanClearSnorlax)
	if snorlax.PortBypass {
		t.Fatalf("Snorlax transition incorrectly owns a dead map-edge port: %+v", snorlax)
	}
}
