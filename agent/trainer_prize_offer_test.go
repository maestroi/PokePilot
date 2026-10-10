package agent

import "testing"

// The planner sees the exact ROM prize instead of inferring it.
func TestTrainerOfferCarriesPrizeMoney(t *testing.T) {
	ctx := &objectiveOfferContext{known: NewKnowledge(nil), catalog: ObjectiveCatalog{Interactables: []CatalogInteractable{{
		X: 4, Y: 5, Kind: CatalogInteractableTrainer, Challengeable: true, PrizeMoney: 165,
	}}}}
	got := explorationObjectiveProvider{}.Provide(ctx).Candidates
	if len(got) != 1 || got[0].Kind != KindTrainer || got[0].Note != "(prize ¥165 on win)" {
		t.Fatalf("trainer offer = %+v, want prize note", got)
	}
}
