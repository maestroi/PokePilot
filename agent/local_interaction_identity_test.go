package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestLocalObjectiveKeysAreLocationScoped(t *testing.T) {
	alpha := LocationID("alpha-house")
	beta := LocationID("beta-house")

	for _, tc := range []struct {
		name string
		a    Objective
		b    Objective
	}{
		{
			name: "trainer",
			a:    Objective{Kind: KindTrainer, Location: alpha, X: 5, Y: 7},
			b:    Objective{Kind: KindTrainer, Location: beta, X: 5, Y: 7},
		},
		{
			name: "npc",
			a:    Objective{Kind: KindTalk, Location: alpha, X: 5, Y: 7},
			b:    Objective{Kind: KindTalk, Location: beta, X: 5, Y: 7},
		},
		{
			name: "pickup",
			a:    Objective{Kind: KindPickup, Location: alpha, X: 5, Y: 7, Item: "potion"},
			b:    Objective{Kind: KindPickup, Location: beta, X: 5, Y: 7, Item: "potion"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a.String() != tc.b.String() {
				t.Fatalf("presentation changed across location: %q != %q", tc.a.String(), tc.b.String())
			}
			if tc.a.Key() == tc.b.Key() {
				t.Fatalf("objective keys collide across locations: %+v", tc.a.Key())
			}
		})
	}
}

func TestCompletedTrainerAtSameCoordinatesOnAnotherLocationStillOffered(t *testing.T) {
	known := NewKnowledge(nil)
	known.Done(Objective{Kind: KindTrainer, Location: "alpha-house", X: 10, Y: 6})

	obs := Observation{
		Location:   "beta-house",
		PartyCount: 1,
		Catalog: ObjectiveCatalog{Interactables: []CatalogInteractable{
			{Kind: CatalogInteractableTrainer, X: 10, Y: 6, Challengeable: true},
		}},
	}
	want := Objective{Kind: KindTrainer, Location: "beta-house", X: 10, Y: 6}
	if !containsObjectiveKey(Offer(obs, known), want.Key()) {
		t.Fatalf("trainer at same coordinates on another location was suppressed: %v", Offer(obs, known))
	}
}

func TestTrainerFailureAtSameCoordinatesOnAnotherLocationDoesNotGateOffer(t *testing.T) {
	known := NewKnowledge(nil)
	alpha := Objective{Kind: KindTrainer, Location: "alpha-house", X: 10, Y: 6}
	known.Failed(alpha, errors.Join(errors.New("battle failed"), skill.ErrTrainerBlackedOut))

	obs := Observation{
		Location:   "beta-house",
		PartyCount: 1,
		Catalog: ObjectiveCatalog{Interactables: []CatalogInteractable{
			{Kind: CatalogInteractableTrainer, X: 10, Y: 6, Challengeable: true},
		}},
	}
	want := Objective{Kind: KindTrainer, Location: "beta-house", X: 10, Y: 6}
	if !containsObjectiveKey(Offer(obs, known), want.Key()) {
		t.Fatalf("trainer failure leaked across locations: %v", Offer(obs, known))
	}
}

func TestNPCInteractionAtSameCoordinatesOnAnotherLocationStillOffered(t *testing.T) {
	known := NewKnowledge(nil)
	known.TalkedAt("alpha-house", 4, 4)
	obs := Observation{
		Location: "beta-house",
		Catalog: ObjectiveCatalog{Interactables: []CatalogInteractable{
			{Kind: CatalogInteractablePerson, X: 4, Y: 4},
		}},
	}
	want := Objective{Kind: KindTalk, Location: "beta-house", X: 4, Y: 4}
	if !containsObjectiveKey(Offer(obs, known), want.Key()) {
		t.Fatalf("NPC interaction leaked across locations: %v", Offer(obs, known))
	}
}

func containsObjectiveKey(objs []Objective, want ObjectiveKey) bool {
	for _, obj := range objs {
		if obj.Key() == want {
			return true
		}
	}
	return false
}
