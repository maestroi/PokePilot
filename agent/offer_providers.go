package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/skill"
)

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

type ObjectiveBlockEvidence struct {
	Family      ObjectiveFamily `json:"family"`
	Reason      string          `json:"reason"`
	Objective   *ObjectiveKey   `json:"objective,omitempty"`
	Place       PlaceID         `json:"place,omitempty"`
	Requirement string          `json:"requirement,omitempty"`
}

type ObjectiveOffer struct {
	Candidates    []Objective                    `json:"candidates"`
	Blocked       []ObjectiveBlockEvidence       `json:"blocked,omitempty"`
	Readiness     []ChallengeReadiness           `json:"readiness,omitempty"`
	Recovery      []RecoveryCheckpointAssessment `json:"recovery_checkpoints,omitempty"`
	TrainingAreas []TrainingAreaAssessment       `json:"training_areas,omitempty"`
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
	obs               Observation
	known             *Knowledge
	catalog           ObjectiveCatalog
	currentLocation   LocationID
	knownLocations    map[LocationID]bool
	adjacentLocations map[LocationID]bool
	hops              map[LocationID]int
	semanticBlocked   map[string]bool
	unroutable        map[string]bool
}

func newObjectiveOfferContext(obs Observation, known *Knowledge) *objectiveOfferContext {
	catalog := objectiveCatalogForObservation(obs)
	current := observationLocation(obs, known)
	ctx := &objectiveOfferContext{
		obs:               obs,
		known:             known,
		catalog:           catalog,
		currentLocation:   current,
		knownLocations:    map[LocationID]bool{current: true},
		adjacentLocations: map[LocationID]bool{},
		hops:              mapHops(known.Adjacency, current),
		semanticBlocked:   map[string]bool{},
		unroutable:        map[string]bool{},
	}
	for location := range known.Visited {
		ctx.knownLocations[location] = true
		for _, neighbor := range known.Adjacency[location] {
			ctx.knownLocations[neighbor] = true
		}
	}
	for _, neighbor := range known.Adjacency[current] {
		ctx.knownLocations[neighbor] = true
		ctx.adjacentLocations[neighbor] = true
	}
	for name := range known.Places {
		if destination, ok := catalog.destination(PlaceID(name)); ok && destination.Location != "" {
			ctx.knownLocations[destination.Location] = true
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

func OfferWithEvidence(obs Observation, known *Knowledge) ObjectiveOffer {
	if known == nil {
		known = NewKnowledge(nil)
	}
	if trainingUnviableHere(obs) {
		// Old checkpoints did not persist a readiness target and historically
		// escaped a dead-end weak grass patch by scheduling a retry. New combat
		// losses carry a target and stay locked so the planner seeks a stronger
		// training area instead of retrying the same underpowered fight.
		known.promoteCombatLossesToRetryWhere(func(f Failure) bool { return f.ReadinessTarget == 0 })
	}
	ctx := newObjectiveOfferContext(obs, known)

	// A catalog that offers starters while the party is empty owns the game's
	// mandatory opening transaction. Do not advertise travel (or other
	// unrelated objectives) beside it: in Red, Pallet Town's north exit is
	// physically script-locked until Oak has taken the player into the lab and
	// the starter sequence completes. Offering "go to ..." here lets the
	// strategist select an objective that deterministic execution cannot
	// legally satisfy; the Oak cutscene then interrupts the crossing and the
	// run surfaces a terminal navigation error (farm #1497/#1496).
	//
	// Keep this generic by keying off the catalog's actual starter candidates:
	// games/catalog states with no mandatory starter continue through the
	// ordinary provider pipeline unchanged.
	if obs.PartyCount == 0 {
		starterOnly := (starterObjectiveProvider{}).Provide(ctx)
		if len(starterOnly.Candidates) > 0 {
			candidates := annotate(starterOnly.Candidates, known)
			offer := ObjectiveOffer{Candidates: candidates, Blocked: starterOnly.Blocked}
			offer.Readiness = challengeReadinessForOffer(obs, known, offer)
			offer.Recovery = rankRecoveryCheckpoints(obs, known, ctx.knownLocations, ctx.catalog, ctx.unroutable)
			return offer
		}
	}

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
	candidates = filterCombatRecoveryBlocked(candidates, known)
	candidates = annotate(candidates, known)
	offer := ObjectiveOffer{Candidates: candidates, Blocked: blocked}
	offer.Readiness = challengeReadinessForOffer(obs, known, offer)
	offer.Recovery = rankRecoveryCheckpoints(obs, known, ctx.knownLocations, ctx.catalog, ctx.unroutable)
	return offer
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
		return objectiveProviderResult{Blocked: []ObjectiveBlockEvidence{blockEvidence(ObjectiveFamilyCollection, "no_local_habitat", nil, "", "tall_grass")}}
	}
	if !hasBalls(obs) {
		return objectiveProviderResult{Blocked: []ObjectiveBlockEvidence{blockEvidence(ObjectiveFamilyCollection, "missing_resource", nil, "", "pokeball")}}
	}
	owned := pokedexOwnedSet(obs)
	out := make([]Objective, 0, len(ctx.catalog.LocalEncounters))
	for _, encounter := range ctx.catalog.LocalEncounters {
		if encounter.Species != "" && !owned[encounter.Species] {
			out = append(out, Objective{Kind: KindCatch, Species: encounter.Species})
		}
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
		ranked := rankRecoveryCheckpoints(obs, known, ctx.knownLocations, ctx.catalog, ctx.unroutable)
		var selected *RecoveryCheckpointAssessment
		for i := range ranked {
			if ranked[i].Selected && ranked[i].Routable {
				selected = &ranked[i]
				break
			}
		}
		switch {
		case len(ranked) == 0:
			blocked = append(blocked, blockEvidence(ObjectiveFamilyRecovery, "no_known_center", nil, "", "pokemon_center"))
		case selected == nil:
			blocked = append(blocked, blockEvidence(ObjectiveFamilyRecovery, "route_unroutable", nil, ranked[0].Place, "live_route"))
		default:
			note := fmt.Sprintf("(selected recovery checkpoint; route cost %d + return cost %d = %d)",
				selected.CurrentCost, selected.ReturnCost, selected.TotalCost)
			if selected.ActiveCheckpoint {
				note += " (active cartridge checkpoint preference applied)"
			}
			if selected.FastTravel {
				note += fmt.Sprintf(" (uses legal %s fast travel)", selected.FastTravelMethod)
			}
			if ppExhausted {
				note += " (lead has no PP; Center restores PP without spending finite items)"
			}
			out = append(out,
				Objective{Kind: KindHeal, Place: selected.Place, Note: note},
				Objective{Kind: KindHeal, Place: selected.Place, Flee: true, Note: note},
			)
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
			if ok && it.Quantity > 0 {
				out = append(out, Objective{Kind: KindUseItem, Item: id, Slot: 0, Note: "(finite PP recovery; prefer a known Center when the detour is practical)"})
			}
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
		case combatLossRecorded(known, Objective{Kind: KindGym, Place: challenge.Place}):
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
				out = append(out, Objective{Kind: KindTrain, Level: uint8(target), Note: trainingChoiceNote(lead, obs.WildGrass, obs.Training)})
			}
		}
	}
	return objectiveProviderResult{Candidates: out, Blocked: blocked}
}

type economyObjectiveProvider struct{}

func (economyObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyEconomy }
func (economyObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	out := repelUseObjectives(ctx.obs)
	if ctx.catalog.Shop == nil {
		return objectiveProviderResult{Candidates: out}
	}
	obs := ctx.obs
	if len(obs.MartStock) == 0 {
		for _, item := range ctx.catalog.Shop.Items {
			obs.MartStock = append(obs.MartStock, item.Name)
		}
	}
	economy := EconomyContext(obs)
	if economy == nil {
		return objectiveProviderResult{Candidates: out}
	}
	if cap(out)-len(out) < len(economy.Purchases) {
		grown := make([]Objective, len(out), len(out)+len(economy.Purchases))
		copy(grown, out)
		out = grown
	}
	for _, advice := range economy.Purchases {
		if advice.ShouldBuy && advice.SuggestedQty > 0 {
			if item, ok := ctx.catalog.shopItem(advice.Item); ok {
				objective := Objective{Kind: KindBuy, Item: item, Qty: advice.SuggestedQty}
				if isRepelItemName(advice.Item) {
					objective.Intent = speedrunRepelBuyIntent
					objective.Note = "(speedrun encounter management: buy only bounded Repel coverage)"
				}
				out = append(out, objective)
			}
		}
	}
	return objectiveProviderResult{Candidates: out}
}

type explorationObjectiveProvider struct{}

func (explorationObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyExploration }
func (explorationObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	known := ctx.known
	// Legacy tests/tools that construct native-map-only observations keep the
	// historical zero-value objective shape. Production semantic observations
	// always carry Location, so durable coordinate-local identity is scoped.
	identityLocation := LocationID("")
	if ctx.obs.Location != "" {
		identityLocation = ctx.currentLocation
	}
	out := make([]Objective, 0, len(ctx.catalog.Interactables))
	for _, object := range ctx.catalog.Interactables {
		switch object.Kind {
		case CatalogInteractablePerson:
			if !known.Talked[ctx.currentLocation][[2]uint8{object.X, object.Y}] {
				out = append(out, Objective{Kind: KindTalk, Location: identityLocation, X: object.X, Y: object.Y})
			}
		case CatalogInteractableTrainer:
			challenge := Objective{Kind: KindTrainer, Location: identityLocation, X: object.X, Y: object.Y}
			if object.Challengeable && !object.Defeated && known.completionCount(challenge) == 0 {
				out = append(out, challenge)
			}
		case CatalogInteractableItem:
			if object.Item != "" {
				out = append(out, Objective{Kind: KindPickup, Location: identityLocation, X: object.X, Y: object.Y, Item: object.Item})
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
		name, place := string(destination.Place), destination.Place
		switch {
		case destination.Location == "" || !ctx.knownLocations[destination.Location]:
			continue
		case routePlaceBlocked(obs, place):
			blocked = append(blocked, blockEvidence(ObjectiveFamilyTravel, "route_prerequisite", nil, place, "route_requirement"))
			continue
		case destination.reached(ctx.currentLocation, obs.X, obs.Y):
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
		if len(routable) > 0 {
			placeNames = routable
		}
	}
	seekTrainingArea := trainingUnviableHere(obs)
	if preparation := combatPreparationFor(known, obs); preparation.Active && !obs.HasGrass {
		seekTrainingArea = true
	}
	trainingChoice, hasTrainingChoice := trainingAreaChoice{}, false
	if seekTrainingArea {
		trainingChoice, hasTrainingChoice = bestKnownTrainingPlace(obs, known, placeNames, ctx.catalog)
	}
	if len(known.Adjacency) > 0 && len(placeNames) > journeyPlaceLimit {
		placeNames = selectJourneyPlaces(placeNames, known, ctx.hops, ctx.catalog)
		if hasTrainingChoice {
			wanted := string(trainingChoice.Area.Place)
			found := false
			for _, name := range placeNames {
				if name == wanted {
					found = true
					break
				}
			}
			if !found {
				if len(placeNames) >= journeyPlaceLimit {
					placeNames[len(placeNames)-1] = wanted
				} else {
					placeNames = append(placeNames, wanted)
				}
				sort.Strings(placeNames)
			}
		}
	}
	out := make([]Objective, 0, 2*len(placeNames))
	for _, name := range placeNames {
		destination, ok := ctx.catalog.destination(PlaceID(name))
		if !ok {
			continue
		}
		plain := Objective{Kind: KindGoTo, Place: destination.Place}
		flee := Objective{Kind: KindGoTo, Place: destination.Place, Flee: true}
		if ctx.adjacentLocations[destination.Location] && !known.Visited[destination.Location] {
			plain.Note = "(unvisited adjacent map)"
			flee.Note = "(unvisited adjacent map)"
		}
		if hasTrainingChoice && destination.Place == trainingChoice.Area.Place {
			note := trainingAreaJourneyNote(trainingChoice)
			plain = appendObjectiveNote(plain, note)
			flee = appendObjectiveNote(flee, note)
		}
		out = append(out, plain, flee)
	}
	return objectiveProviderResult{Candidates: out, Blocked: blocked}
}

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
