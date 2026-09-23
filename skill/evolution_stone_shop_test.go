package skill

import "testing"

func TestEvolutionStoneShopUsesInteractionDestination(t *testing.T) {
	dest, ok := Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("Celadon Mart 4F stone shop destination missing")
	}
	if dest.Kind != DestinationInteraction {
		t.Fatalf("stone shop destination kind = %s, want interaction", dest.KindName())
	}
	if dest.Map != celadonMart4FMap || dest.X != celadonMart4FClerkX || dest.Y != celadonMart4FClerkY {
		t.Fatalf("stone shop target = map %02x (%d,%d), want clerk %02x (%d,%d)",
			dest.Map, dest.X, dest.Y,
			celadonMart4FMap, celadonMart4FClerkX, celadonMart4FClerkY)
	}
}
