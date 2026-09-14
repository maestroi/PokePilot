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
	Candidates []Objective             `json:"candidates"`
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
	knownMaps       map[uint8]bool
	adjacentMaps    map[uint8]bool
	hops            map[uint8]int
	semanticBlocked map[string]bool
	unroutable      map[string]bool
}

func newObjectiveOfferContext(obs Observation, known *Knowledge) *objectiveOfferContext {
	ctx := &objectiveOfferContext{
		obs:             obs,
		known:           known,
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
		if d, ok := skill.Place(name); ok {
			ctx.knownMaps[d.Map] = true
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
	return objectiveProviderResult{Candidates: []Objective{
		{Kind: KindStarter, Starter: skill.StarterCharmander},
		{Kind: KindStarter, Starter: skill.StarterSquirtle},
		{Kind: KindStarter, Starter: skill.StarterBulbasaur},
	}}
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
	out := make([]Objective, 0, len(obs.WildGrass))
	for _, w := range obs.WildGrass {
		sp, ok := SpeciesByName(w.Name)
		if !ok || owned[sp] {
			continue
		}
		out = append(out, Objective{Kind: KindCatch, Species: sp})
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
	if isCenter(obs.MapName) {
		heal := Objective{Kind: KindHeal}
		if ppExhausted {
			heal.Note = "(lead has no PP; Center restores PP without spending finite items)"
		}
		out = append(out, heal)
	} else if partyHurt(obs) || ppExhausted {
		if name, ok := nearestKnownCenter(obs, known, ctx.knownMaps); ok {
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
		id, _ := ItemByName(it.Name)
		for slot, mon := range obs.Party {
			if medReaches(mon, want) {
				out = append(out, Objective{Kind: KindUseItem, Item: id, Slot: slot})
			}
		}
	}
	if ppExhausted && len(obs.Party) > 0 && !isCenter(obs.MapName) {
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
	out := make([]Objective, 0, 2)
	blocked := make([]ObjectiveBlockEvidence, 0, 2)
	if g, ok := skill.GymAt(obs.Map); ok && !hasBadge(obs, g.Badge) {
		gym := Objective{Kind: KindGym, Place: g.Place}
		switch {
		case g.Map == viridianGymMap && journeyProgressionBlocked(obs, g.Map):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "progression_gate", &gym, g.Place, "story_progression"))
		case obs.Map != g.Map && ctx.semanticBlocked[string(g.Place)]:
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "route_prerequisite", &gym, g.Place, "route_requirement"))
		case gymLossRecorded(known, g.Place):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTraining, "combat_readiness", &gym, g.Place, "material_party_progress"))
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
	if !isMart(ctx.obs.MapName) {
		return objectiveProviderResult{}
	}
	economy := EconomyContext(ctx.obs)
	if economy == nil {
		return objectiveProviderResult{}
	}
	out := make([]Objective, 0, len(economy.Purchases))
	for _, advice := range economy.Purchases {
		if !advice.ShouldBuy || advice.SuggestedQty < 1 {
			continue
		}
		if it, ok := ItemByName(advice.Item); ok {
			out = append(out, Objective{Kind: KindBuy, Item: it, Qty: advice.SuggestedQty})
		}
	}
	return objectiveProviderResult{Candidates: out}
}

type explorationObjectiveProvider struct{}

func (explorationObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyExploration }
func (explorationObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	out := make([]Objective, 0, len(obs.MapObjects))
	for _, object := range obs.MapObjects {
		switch object.Kind {
		case "person":
			if !known.Talked[obs.Map][[2]uint8{object.X, object.Y}] {
				out = append(out, Objective{Kind: KindTalk, X: object.X, Y: object.Y})
			}
		case "trainer":
			challenge := Objective{Kind: KindTrainer, X: object.X, Y: object.Y}
			if object.Challengeable && !object.Defeated && known.completionCount(challenge) == 0 {
				out = append(out, challenge)
			}
		case "item":
			if id, ok := ItemByName(object.Item); ok {
				out = append(out, Objective{Kind: KindPickup, X: object.X, Y: object.Y, Item: id})
			}
		}
	}
	return objectiveProviderResult{Candidates: out}
}

type travelObjectiveProvider struct{}

func (travelObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyTravel }
func (travelObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs, known := ctx.obs, ctx.known
	placeNames := make([]string, 0, 16)
	blocked := make([]ObjectiveBlockEvidence, 0, len(ctx.semanticBlocked))
	for _, name := range skill.PlaceNames() {
		d, _ := skill.Place(name)
		place := PlaceID(name)
		switch {
		case !ctx.knownMaps[d.Map]:
			continue
		case journeyProgressionBlocked(obs, d.Map) || placeProgressionBlocked(obs, name):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTravel, "progression_gate", nil, place, "story_progression"))
			continue
		case d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y:
			continue
		case ctx.semanticBlocked[name]:
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTravel, "route_prerequisite", nil, place, "route_requirement"))
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
		d, _ := skill.Place(name)
		plain := Objective{Kind: KindGoTo, Place: PlaceID(name)}
		flee := Objective{Kind: KindGoTo, Place: PlaceID(name), Flee: true}
		if ctx.adjacentMaps[d.Map] && !known.Visited[d.Map] {
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
