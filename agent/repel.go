package agent

import "strings"

const (
	speedrunRepelUseIntent = "speedrun-repel"
	speedrunRepelBuyIntent = "speedrun-repel-supply"
	targetRepelSteps        = 300
)

var repelDurations = map[string]int{
	"repel":       100,
	"super repel": 200,
	"max repel":   250,
}

func isRepelItemName(name string) bool {
	_, ok := repelDurations[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func repelCoverageSteps(obs Observation) int {
	total := obs.RepelSteps
	for _, item := range obs.Bag {
		if steps, ok := repelDurations[strings.ToLower(strings.TrimSpace(item.Name))]; ok {
			total += steps * item.Quantity
		}
	}
	return total
}

func preferredRepelInBag(obs Observation) (ItemID, bool) {
	// Once the item is already owned, the longer effect saves more menu
	// transitions. Purchase policy separately optimizes yen per protected step.
	for _, name := range []string{"max repel", "super repel", "repel"} {
		if bagQuantity(obs, name) > 0 {
			return ItemID(name), true
		}
	}
	return "", false
}

func isSpeedrunRepelObjective(o Objective) bool {
	return o.Intent == speedrunRepelUseIntent || o.Intent == speedrunRepelBuyIntent
}

type repelObjectiveProvider struct{}

func (repelObjectiveProvider) Family() ObjectiveFamily { return ObjectiveFamilyEconomy }

func (repelObjectiveProvider) Provide(ctx *objectiveOfferContext) objectiveProviderResult {
	obs := ctx.obs
	if obs.InBattle || obs.RepelSteps > 0 || len(obs.WildGrass) == 0 {
		return objectiveProviderResult{}
	}
	item, ok := preferredRepelInBag(obs)
	if !ok {
		return objectiveProviderResult{}
	}
	return objectiveProviderResult{Candidates: []Objective{{
		Kind:   KindUseItem,
		Item:   item,
		Slot:   -1,
		Intent: speedrunRepelUseIntent,
		Note:   "(speedrun encounter management: avoid unnecessary wild-battle transitions on this encounter map)",
	}}}
}

func filterRepelForPlayStyle(offered []Objective, profile PlayStyleProfile) []Objective {
	if profile.Name == PlayStyleSpeedrun {
		return append([]Objective(nil), offered...)
	}
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		if !isSpeedrunRepelObjective(o) {
			out = append(out, o)
		}
	}
	return out
}
