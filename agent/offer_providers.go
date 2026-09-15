package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/skill"
)

// ObjectiveFamily is the stable category of one objective provider. Families
// describe why an action is offered, not the concrete game implementation that
// supplied it.
type ObjectiveFamily string

const (
	ObjectiveFamilyProgression ObjectiveFamily = "progression"
	ObjectiveFamilyTravel      ObjectiveFamily = "travel"
	ObjectiveFamilyRecovery    ObjectiveFamily = "recovery"
	ObjectiveFamilyCollection  ObjectiveFamily = "collection"
	ObjectiveFamilyEconomy     ObjectiveFamily = "economy"
	ObjectiveFamilyTraining    ObjectiveFamily = "training"
	ObjectiveFamilyExploration ObjectiveFamily = "exploration"
)

// ObjectiveBlockEvidence is structured evidence for an action family that was
// considered but is not currently actionable. It is deliberately semantic and
// compact so future planners/play styles can reason over prerequisites without
// parsing notes or error strings.
type ObjectiveBlockEvidence struct {
	Family      ObjectiveFamily `json:"family"`
	Reason      string          `json:"reason"`
	Objective   *ObjectiveKey   `json:"objective,omitempty"`
	Place       PlaceID         `json:"place,omitempty"`
	Requirement string          `json:"requirement,omitempty"`
}

// ObjectiveOffer is the deterministic result of provider composition. Offer
// keeps returning Candidates for API compatibility; callers that need gating
// evidence can use OfferWithEvidence.
type ObjectiveOffer struct {
	Candidates []Objective              `json:"candidates"`
	Blocked    []ObjectiveBlockEvidence `json:"blocked,omitempty"`
}

type objectiveProvider interface {
	Family() ObjectiveFamily
	Provide(*objectiveOfferContext) objectiveProviderResult
}

type objectiveProviderResult struct {
	Candidates []Objective
	Blocked    []ObjectiveBlockEvidence
}

type objectiveOfferContext struct {
	obs             Observation
	known           *Knowledge
	catalog         ObjectiveCatalog
	knownMaps       map[uint8]bool
	adjacentMaps    map[uint8]bool
	hops            map[uint8]int
	semanticBlocked map[string]bool
	unroutable      map[string]bool
}

func newObjectiveOfferContext(obs Observation, known *Knowledge) *objectiveOfferContext {
	catalog := objectiveCatalogForObservation(obs)
	ctx := &objectiveOfferContext{
		obs:             obs,
		known:           known,
		catalog:         catalog,
		knownMaps:       map[uint8]bool{obs.Map: true},
		adjacentMaps:    map[uint8]bool{},
		hops:            mapHops(known.Adjacency, obs.Map),
		semanticBlocked: map[string]bool{},
		unroutable:      map[string]bool{},
	}
	for m := range known.Visited {
		ctx.knownMaps[m] = true
		for _, n := range known.Adjacency[m] {
			ctx.knownMaps[n] = true
		}
	}
	for _, n := range known.Adjacency[obs.Map] {
		ctx.knownMaps[n] = true
		ctx.adjacentMaps[n] = true
	}
	for name := range known.Places {
		if destination, ok := catalog.destination(PlaceID(name)); ok {
			ctx.knownMaps[destination.NativeMap] = true
		}
	}
	for _, blockage := range obs.RouteBlockages {
		ctx.semanticBlocked[string(blockage.Destination)] = true
	}
	for _, name := range obs.Unroutable {
		if !ctx.semanticBlocked[name] {
			ctx.unroutable[name] = true
		}
	}
	return ctx
}

func blockEvidence(family ObjectiveFamily, reason string, o *Objective, place PlaceID, requirement string) ObjectiveBlockEvidence {
	var key *ObjectiveKey
	if o != nil {
		value := o.Key()
		key = &value
	}
	return ObjectiveBlockEvidence{Family: family, Reason: reason, Objective: key, Place: place, Requirement: requirement}
}

var defaultObjectiveProviders = []objectiveProvider{
	starterObjectiveProvider{},
	wildCollectionProvider{},
	recoveryObjectiveProvider{},
	trainingObjectiveProvider{},
	economyObjectiveProvider{},
	explorationObjectiveProvider{},
	travelObjectiveProvider{},
}

// OfferWithEvidence composes the portable provider set in one explicit order.
// Providers own candidate construction and prerequisite evidence; this pipeline
// owns only ordering, shared recovery filters, annotation, and the bounded
// travel-last convention.
func OfferWithEvidence(obs Observation, known *Knowledge) ObjectiveOffer {
	if known == nil {
		known = NewKnowledge(nil)
	}
	if trainingUnviableHere(obs) {
		known.releaseCombatLossGates()
	}
	ctx := newObjectiveOfferContext(obs, known)
	local := make([]Objective, 0, 8)
	journeys := make([]Objective, 0, 2*journeyPlaceLimit)
	blocked := make([]ObjectiveBlockEvidence, 0, 8)
	for _, provider := range defaultObjectiveProviders {
		result := provider.Provide(ctx)
		blocked = append(blocked, result.Blocked...)
		if provider.Family() == ObjectiveFamilyTravel {
			journeys = append(journeys, result.Candidates...)
		} else {
			local = append(local, result.Candidates...)
		}
	}

	if note := lastResortEscapeNote(local, journeys, obs.RespawnPlace); note != "" {
		for i := range local {
			if local[i].Kind == KindTrain {
				local[i] = appendObjectiveNote(local[i], note)
			}
		}
	}
	candidates := append(local, journeys...)
	candidates = filterTrainerLossBlocked(candidates, known)
	candidates = annotate(candidates, known)
	return ObjectiveOffer{Candidates: candidates, Blocked: blocked}
}

type starterObjectiveProvider struct{}

func (starterObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyProgression }
func (starterObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	if ctx.obs.PartyCount != 0 {
		return objectiveProviderResult{}
	}
	out := make([]Objective, 0, len(ctx.catalog.Starters))
	for _, starter := range ctx.catalog.Starters {
		out = append(out, Objective{Kind: KindStarter, Starter: starter.Starter})
	}
	return objectiveProviderResult{Candidates: out}
}

type wildCollectionProvider struct{}

func (wildCollectionProvider) Family() ObjectiveFamily { return ObjectiveFamilyCollection }
func (wildCollectionProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs := ctx.obs
	if !obs.HasGrass {
		return objectiveProviderResult{Blocked: []ObjectiveBlockEvidence{
			blockEvidence(ObjectiveFamilyCollection, "no_local_habitat", nil, "", "tall_grass"),
		}}
	}
	if !hasBalls(obs) {
		return objectiveProviderResult{Blocked: []ObjectiveBlockEvidence{
			blockEvidence(ObjectiveFamilyCollection, "missing_resource", nil, "", "pokeball"),
		}}
	}
	owned := pokedexOwnedSet(obs)
	out := make([]Objective, 0, len(ctx.catalog.LocalEncounters))
	for _, encounter := range ctx.catalog.LocalEncounters {
		if encounter.Species == "" || owned[encounter.Species] {
			continue
		}
		out = append(out, Objective{Kind: KindCatch, Species: encounter.Species})
	}
	return objectiveProviderResult{Candidates: out}
}

type recoveryObjectiveProvider struct{}

func (recoveryObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyRecovery }
func (recoveryObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	out := make([]Objective, 0, 6)
	blocked := make([]ObjectiveBlockEvidence, 0, 2)
	ppExhausted := leadOutOfPP(obs)
	if ctx.catalog.CurrentCenter {
		heal := Objective{Kind: KindHeal}
		if ppExhausted {
			heal.Note = "(lead has no PP; Center restores PP without spending finite items)"
		}
		out = append(out, heal)
	} else if partyHurt(obs) || ppExhausted {
		if name, ok := nearestKnownCenter(obs, known, ctx.knownMaps, ctx.catalog); ok {
			note := ""
			if ppExhausted {
				note = "(lead has no PP; Center restores PP without spending finite items)"
			}
			out = append(out,
				Objective{Kind: KindHeal, Place: name, Note: note},
				Objective{Kind: KindHeal, Place: name, Flee: true, Note: note},
			)
		} else {
			blocked = append(blocked, blockEvidence(ObjectiveFamilyRecovery, "no_known_center", nil, "", "pokemon_center"))
		}
	}

	for _, it := range obs.Bag {
		want, ok := fieldMedStatus[it.Name]
		if !ok || it.Quantity < 1 {
			continue
		}
		id := ItemID(it.Name)
		for slot, mon := range obs.Party {
			if medReaches(mon, want) {
				out = append(out, Objective{Kind: KindUseItem, Item: id, Slot: slot})
			}
		}
	}
	if ppExhausted && len(obs.Party) > 0 && !ctx.catalog.CurrentCenter {
		for _, it := range obs.Bag {
			id, ok := ppRestoreItems[it.Name]
			if !ok || it.Quantity < 1 {
				continue
			}
			out = append(out, Objective{
				Kind: KindUseItem,
				Item: id,
				Slot: 0,
				Note: "(finite PP recovery; prefer a known Center when the detour is practical)",
			})
		}
	}
	return objectiveProviderResult{Candidates: out, Blocked: blocked}
}

type trainingObjectiveProvider struct{}

func (trainingObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyTraining }
func (trainingObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	out := make([]Objective, 0, 2+len(ctx.catalog.Challenges))
	blocked := make([]ObjectiveBlockEvidence, 0, 2)
	for _, challenge := range ctx.catalog.Challenges {
		if challenge.Complete {
			continue
		}
		gym := Objective{Kind: KindGym, Place: challenge.Place}
		switch {
		case routePlaceBlocked(obs, challenge.Place):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "route_prerequisite", &gym, challenge.Place, "route_requirement"))
		case gymLossRecorded(known, challenge.Place):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "combat_readiness", &gym, challenge.Place, "material_party_progress"))
		default:
			out = append(out, gym)
		}
	}

	if obs.HasGrass && len(obs.Party) > 0 {
		lead := obs.Party[0]
		switch {
		case trainingUnviableHere(obs):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "outside_training_budget", nil, "", "better_training_area"))
		case skill.BelowRetreatLine(lead.HP, lead.MaxHP):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "party_needs_recovery", nil, "", "healing"))
		default:
			if target := int(lead.Level) + trainStep; target <= 100 {
				out = append(out, Objective{
					Kind:  KindTrain,
					Level: uint8(target),
					Note:  trainingChoiceNote(lead, obs.WildGrass, obs.Training),
				})
			}
		}
	}
	return objectiveProviderResult{Candidates: out, Blocked: blocked}
}

type economyObjectiveProvider struct{}

func (economyObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyEconomy }
func (economyObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	if ctx.catalog.Shop == nil {
		return objectiveProviderResult{}
	}
	obs := ctx.obs
	if len(obs.MartStock) == 0 {
		obs.MartStock = make([]string, 0, len(ctx.catalog.Shop.Items))
		for _, item := range ctx.catalog.Shop.Items {
			obs.MartStock = append(obs.MartStock, item.Name)
		}
	}
	economy := EconomyContext(obs)
	if economy == nil {
		return objectiveProviderResult{}
	}
	out := make([]Objective, 0, len(economy.Purchases))
	for _, advice := range economy.Purchases {
		if !advice.ShouldBuy || advice.SuggestedQty < 1 {
			continue
		}
		if item, ok := ctx.catalog.shopItem(advice.Item); ok {
			out = append(out, Objective{Kind: KindBuy, Item: item, Qty: advice.SuggestedQty})
		}
	}
	return objectiveProviderResult{Candidates: out}
}

type explorationObjectiveProvider struct{}

func (explorationObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyExploration }
func (explorationObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	out := make([]Objective, 0, len(ctx.catalog.Interactables))
	for _, object := range ctx.catalog.Interactables {
		switch object.Kind {
		case CatalogInteractablePerson:
			if !known.Talked[obs.Map][[2]uint8{object.X, object.Y}] {
				out = append(out, Objective{Kind: KindTalk, X: object.X, Y: object.Y})
			}
		case CatalogInteractableTrainer:
			challenge := Objective{Kind: KindTrainer, X: object.X, Y: object.Y}
			if object.Challengeable && !object.Defeated && known.completionCount(challenge) == 0 {
				out = append(out, challenge)
			}
		case CatalogInteractableItem:
			if object.Item != "" {
				out = append(out, Objective{Kind: KindPickup, X: object.X, Y: object.Y, Item: object.Item})
			}
		}
	}
	return objectiveProviderResult{Candidates: out}
}

type travelObjectiveProvider struct{}

func (travelObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyTravel }
func (travelObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	placeNames := make([]string, 0, len(ctx.catalog.Destinations))
	blocked := make([]ObjectiveBlockEvidence, 0, len(ctx.semanticBlocked))
	for _, destination := range ctx.catalog.Destinations {
		name := string(destination.Place)
		place := destination.Place
		switch {
		case !ctx.knownMaps[destination.NativeMap]:
			continue
		case routePlaceBlocked(obs, place):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTravel, "route_prerequisite", nil, place, "route_requirement"))
			continue
		case destination.NativeMap == obs.Map && destination.X == obs.X && destination.Y == obs.Y:
			continue
		default:
			placeNames = append(placeNames, name)
		}
	}
	if len(ctx.unroutable) > 0 {
		routable := make([]string, 0, len(placeNames))
		for _, name := range placeNames {
			if ctx.unroutable[name] {
				blocked = append(blocked, blockEvidence(ObjectiveFamilyTravel, "route_unroutable", nil, PlaceID(name), "live_route"))
				continue
			}
			routable = append(routable, name)
		}
		// Preserve the historical fail-open rule: a faulty live route overlay
		// cannot erase every journey from the generic menu.
		if len(routable) > 0 {
			placeNames = routable
		}
	}
	if len(known.Adjacency) > 0 && len(placeNames) > journeyPlaceLimit {
		placeNames = selectJourneyPlaces(placeNames, known, ctx.hops)
	}
	out := make([]Objective, 0, 2*len(placeNames))
	for _, name := range placeNames {
		destination, ok := ctx.catalog.destination(PlaceID(name))
		if !ok {
			continue
		}
		plain := Objective{Kind: KindGoTo, Place: destination.Place}
		flee := Objective{Kind: KindGoTo, Place: destination.Place, Flee: true}
		if ctx.adjacentMaps[destination.NativeMap] && !known.Visited[destination.NativeMap] {
			plain.Note = "(unvisited adjacent map)"
			flee.Note = "(unvisited adjacent map)"
		}
		out = append(out, plain, flee)
	}
	return objectiveProviderResult{Candidates: out, Blocked: blocked}
}

// deterministicProviderNames is test/diagnostic evidence that provider order
// is explicit rather than incidental map iteration.
func deterministicProviderNames(providers []objectiveProvider) []string {
	out := make([]string, 0, len(providers))
	for _, provider := range providers {
		out = append(out, fmt.Sprintf("%s:%T", provider.Family(), provider))
	}
	return out
}

func sortBlockEvidence(blocked []ObjectiveBlockEvidence) {
	sort.SliceStable(blocked, func(i, j int) bool {
		if blocked[i].Family != blocked[j].Family {
			return blocked[i].Family < blocked[j].Family
		}
		if blocked[i].Reason != blocked[j].Reason {
			return blocked[i].Reason < blocked[j].Reason
		}
		return blocked[i].Place < blocked[j].Place
	})
}
