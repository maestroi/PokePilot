package agent

import "strings"

const (
	speedrunRepelUseIntent = "speedrun-repel"
	speedrunRepelBuyIntent = "speedrun-repel-supply"
	targetRepelSteps       = 300
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

func repelUseObjectives(obs Observation) []Objective {
	if obs.InBattle || obs.RepelSteps > 0 || len(obs.WildGrass) == 0 {
		return nil
	}
	item, ok := preferredRepelInBag(obs)
	if !ok {
		return nil
	}
	return []Objective{{
		Kind:   KindUseItem,
		Item:   item,
		Slot:   -1,
		Intent: speedrunRepelUseIntent,
		Note:   "(speedrun encounter management: avoid unnecessary wild-battle transitions on this encounter map)",
	}}
}

func filterRepelForPlayStyle(obs Observation, offered []Objective, profile PlayStyleProfile) []Objective {
	if profile.Name == "" {
		return append([]Objective(nil), offered...)
	}
	if profile.Name == PlayStyleSpeedrun {
		out := append([]Objective(nil), offered...)
		if strings.Contains(strings.ToUpper(obs.MapName), "SAFARI") {
			return out
		}
		for i := range out {
			if out[i].Kind != KindGoTo || !out[i].Flee {
				continue
			}
			_, crossMap, located := objectiveDistance(obs, out[i])
			if crossMap && located {
				out[i].RepelBeforeTravel = true
			}
		}
		return out
	}
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		if !isSpeedrunRepelObjective(o) {
			out = append(out, o)
		}
	}
	return out
}
