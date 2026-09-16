package agent

import (
	"fmt"
	"strings"
)

// NaturalPlaySignal is reusable context layered on top of the drive score. It
// explains why an already-legal objective looks like something a player would
// naturally do now. Profile scales decide how much exploration/optional work or
// party development matters; this layer never creates legality or bypasses
// progression.
type NaturalPlaySignal struct {
	Bonus         float64
	RepeatPenalty float64
	Tags          []string
	Stall         StallFallbackSignal
}

func (s NaturalPlaySignal) Net() float64 { return s.Bonus - s.RepeatPenalty }

func naturalPlaySignal(obs Observation, o Objective, profile PlayStyleProfile) NaturalPlaySignal {
	if profile.NaturalPlayScale <= 0 {
		return NaturalPlaySignal{}
	}

	var s NaturalPlaySignal
	addScaled := func(tag string, bonus, scale float64) {
		bonus *= profile.NaturalPlayScale * scale
		if bonus <= 0 {
			return
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.Bonus += bonus
	}
	add := func(tag string, bonus float64) { addScaled(tag, bonus, 1) }
	penalize := func(tag string, penalty float64) {
		if penalty <= 0 {
			return
		}
		// Repetition remains bounded across profiles. Completionist may value
		// more optional work, but it still must not retry the same interaction
		// forever; Team Builder has the same rule for grinding one target.
		scale := profile.NaturalPlayScale
		if scale < 0.80 {
			scale = 0.80
		}
		if scale > 1.20 {
			scale = 1.20
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.RepeatPenalty += penalty * scale
	}

	switch o.Kind {
	case KindHeal:
		if partyHurt(obs) || leadOutOfPP(obs) {
			add("consolidate-recovery", 0.30)
		} else {
			penalize("already-recovered", 0.35)
		}

	case KindTalk:
		// Offer only exposes a person until Knowledge.Talked records success,
		// so every KindTalk reaching the styled planner is novel.
		addScaled("new-npc", 0.18, profile.ExplorationScale)
		if distance, crossMap, located := objectiveDistance(obs, o); located && !crossMap && distance <= 3 {
			addScaled("nearby", 0.08, profile.ExplorationScale)
		}

	case KindBuy:
		// KindBuy exists only for EconomyContext purchases with ShouldBuy=true.
		add("needed-resupply", 0.25)
		if spec, ok := ItemEconomy(string(o.Item)); ok && spec.Category == InventoryCapture && len(obs.Dex.Targets) > 0 {
			// Capture stock is completion infrastructure while obtainable Dex
			// entries remain. Scale this like exploration rather than party power:
			// a boxed new species is still durable Completionist progress.
			addScaled("dex-supplies", 0.30, profile.ExplorationScale)
		}

	case KindPickup:
		note := strings.ToLower(o.Note)
		switch {
		case strings.Contains(note, "high-value preparation"):
			addScaled("high-value-item", 0.28, profile.ExplorationScale)
		case strings.Contains(note, "useful preparation"):
			addScaled("useful-item", 0.14, profile.ExplorationScale)
		default:
			addScaled("optional-item", 0.05, profile.ExplorationScale)
		}
		if highImpactItem(o.Item) {
			addScaled("durable-upgrade", 0.12, profile.PartyScale)
		}
		if distance, crossMap, located := objectiveDistance(obs, o); located && !crossMap && distance <= 4 {
			addScaled("nearby", 0.10, profile.ExplorationScale)
		}

	case KindGoTo:
		lowNote := strings.ToLower(o.Note)
		lowPlace := strings.ToLower(string(o.Place))
		if strings.Contains(lowNote, "unvisited adjacent map") {
			addScaled("frontier", 0.30, profile.ExplorationScale)
		}
		if naturalCenterPlace(lowPlace) && (partyHurt(obs) || leadOutOfPP(obs)) {
			add("recovery-stop", 0.28)
		}
		if naturalMartPlace(lowPlace) {
			if len(obs.Dex.Targets) > 0 && normalBallStock(obs) < minimumCaptureStock && captureResupplyAffordable(obs) {
				// EconomyContext intentionally asks for capture resupply only while
				// standing near wild encounters. A completion run must also be able
				// to decide to visit a Mart before leaving town with no balls, but
				// only when at least one normal ball is actually affordable after
				// current progression reserves.
				addScaled("capture-resupply", 0.35, profile.ExplorationScale)
			} else if economy := EconomyContext(obs); economy != nil && economy.ResupplyNeeded {
				add("resupply-stop", 0.24)
			}
		}

	case KindCatch:
		if o.Species != "" && !pokedexOwnedSet(obs)[o.Species] {
			// Dex ownership, not party usefulness or party capacity, is the
			// completion signal. Catch execution can box a species when needed.
			addScaled("new-dex-entry", 0.35, profile.ExplorationScale)
		}
		var bonus float64
		switch {
		case obs.PartyCount <= 2:
			bonus = 0.30
		case obs.PartyCount == 3:
			bonus = 0.22
		case obs.PartyCount == 4:
			bonus = 0.10
		default:
			bonus = 0.03
		}
		tag := "team-option"
		if obs.PartyCount <= 3 {
			tag = "build-party"
		}
		addScaled(tag, bonus, profile.PartyScale)
		if o.Place == "" {
			addScaled("local-encounter", 0.06, profile.PartyScale)
		} else {
			addScaled("known-habitat", 0.03, profile.PartyScale)
		}

	case KindTrain:
		if mon, ok := naturalTrainingTarget(obs, o); ok && len(obs.Party) > 0 {
			gap := int(obs.Party[0].Level) - int(mon.Level)
			switch {
			case o.Species != "" && gap >= 4:
				addScaled("catch-up-party", 0.28, profile.PartyScale)
			case o.Species != "" && gap >= 2:
				addScaled("catch-up-party", 0.20, profile.PartyScale)
			case o.Species != "":
				addScaled("develop-party", 0.10, profile.PartyScale)
			default:
				addScaled("prepare-lead", 0.05, profile.PartyScale)
			}
		}

	case KindTrainer:
		if naturalUnderlevelledParty(obs) {
			addScaled("useful-training", 0.12, profile.PartyScale)
		} else {
			addScaled("route-trainer", 0.05, profile.PartyScale)
		}
	}

	if repeats := recentNaturalObjectiveCount(obs, o); repeats > 0 && naturalRepeatSensitive(o) {
		penalty := float64(repeats) * 0.08
		if penalty > 0.24 {
			penalty = 0.24
		}
		penalize("recent-repeat", penalty)
	}

	// Adventure owns the targeted stalled-progression recovery policy from
	// #275. Other profiles still share the same legal menu and scoring core;
	// they can opt into this recovery policy later without another planner.
	s.Stall = stallFallbackSignal(obs, o, profile)
	if s.Stall.Context.Active {
		s.Bonus += s.Stall.Bonus
		s.RepeatPenalty += s.Stall.Penalty
		for _, tag := range s.Stall.Tags {
			s.Tags = appendNaturalTag(s.Tags, tag)
		}
	}

	s = mergeNaturalPlaySignal(s, completionistCoverageSignal(obs, o, profile))
	return s
}

func appendNaturalTag(tags []string, tag string) []string {
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func naturalCenterPlace(place string) bool {
	return strings.Contains(place, "pokemon center") || strings.Contains(place, "pokecenter")
}

func naturalMartPlace(place string) bool { return strings.Contains(place, "mart") }

func captureResupplyAffordable(obs Observation) bool {
	economy := EconomyContext(obs)
	if economy == nil {
		return false
	}
	ball, ok := ItemEconomy("pokeball")
	return ok && economy.SpendableMoney >= ball.UnitPrice
}

func naturalTrainingTarget(obs Observation, o Objective) (PartyMon, bool) {
	if o.Kind != KindTrain || len(obs.Party) == 0 {
		return PartyMon{}, false
	}
	if o.Species != "" {
		for _, mon := range obs.Party {
			if mon.Species == o.Species {
				return mon, true
			}
		}
	}
	if o.Slot >= 0 && o.Slot < len(obs.Party) {
		return obs.Party[o.Slot], true
	}
	return obs.Party[0], true
}

func naturalUnderlevelledParty(obs Observation) bool {
	if len(obs.Party) < 2 {
		return false
	}
	lead := int(obs.Party[0].Level)
	for _, mon := range obs.Party[1:] {
		if mon.HP > 0 && lead-int(mon.Level) >= 3 {
			return true
		}
	}
	return false
}

func recentNaturalObjectiveCount(obs Observation, o Objective) int {
	want := o.String()
	count := 0
	for i := len(obs.History) - 1; i >= 0 && i >= len(obs.History)-6; i-- {
		if obs.History[i].Objective == want {
			count++
		}
	}
	return count
}

func naturalRepeatSensitive(o Objective) bool {
	switch o.Kind {
	case KindHeal, KindTalk, KindTrainer, KindCatch, KindBuy, KindPickup, KindTrain:
		return true
	case KindGoTo:
		return strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") || naturalMartPlace(strings.ToLower(string(o.Place)))
	default:
		return false
	}
}

func naturalPlaySummary(s NaturalPlaySignal) string {
	if len(s.Tags) == 0 && s.Bonus == 0 && s.RepeatPenalty == 0 {
		return ""
	}
	labels := strings.Join(s.Tags, "+")
	var base string
	switch {
	case s.Bonus > 0 && s.RepeatPenalty > 0:
		base = fmt.Sprintf("natural %+.2f-%0.2f %s", s.Bonus, s.RepeatPenalty, labels)
	case s.Bonus > 0:
		base = fmt.Sprintf("natural %+.2f %s", s.Bonus, labels)
	default:
		base = fmt.Sprintf("natural -%.2f %s", s.RepeatPenalty, labels)
	}
	if stall := stallFallbackSummary(s.Stall); stall != "" {
		base += "; " + stall
	}
	return base
}
