package agent

import (
	"fmt"
	"strings"
)

// NaturalPlaySignal is the Adventure-specific context layered on top of the
// reusable drive score. It explains why an already-legal objective looks like
// something a normal player would naturally do now: consolidate, inspect a
// new frontier, talk to a new NPC, restock, improve the party, or take a cheap
// nearby reward. It never creates legality and never bypasses progression.
type NaturalPlaySignal struct {
	Bonus         float64
	RepeatPenalty float64
	Tags          []string
	Stall         StallFallbackSignal
}

func (s NaturalPlaySignal) Net() float64 { return s.Bonus - s.RepeatPenalty }

func naturalPlaySignal(obs Observation, o Objective, profile PlayStyleProfile) NaturalPlaySignal {
	if profile.Name != "adventure" {
		return NaturalPlaySignal{}
	}

	var s NaturalPlaySignal
	add := func(tag string, bonus float64) {
		if bonus <= 0 {
			return
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.Bonus += bonus
	}
	penalize := func(tag string, penalty float64) {
		if penalty <= 0 {
			return
		}
		s.Tags = appendNaturalTag(s.Tags, tag)
		s.RepeatPenalty += penalty
	}

	switch o.Kind {
	case KindHeal:
		if partyHurt(obs) || leadOutOfPP(obs) {
			add("consolidate-recovery", 0.30)
		} else {
			// Centers offer Heal even at full health. Adventure should not turn
			// that harmless mechanic into a repeated sightseeing objective.
			penalize("already-recovered", 0.35)
		}

	case KindTalk:
		// Offer only exposes a person until Knowledge.Talked records success,
		// so every KindTalk reaching the styled planner is a novel interaction.
		add("new-npc", 0.18)
		if distance, crossMap, located := objectiveDistance(obs, o); located && !crossMap && distance <= 3 {
			add("nearby", 0.08)
		}

	case KindBuy:
		// KindBuy exists only for EconomyContext purchases with ShouldBuy=true.
		add("needed-resupply", 0.25)

	case KindPickup:
		note := strings.ToLower(o.Note)
		switch {
		case strings.Contains(note, "high-value preparation"):
			add("high-value-item", 0.28)
		case strings.Contains(note, "useful preparation"):
			add("useful-item", 0.14)
		default:
			add("optional-item", 0.05)
		}
		if highImpactItem(o.Item) {
			add("durable-upgrade", 0.12)
		}
		if distance, crossMap, located := objectiveDistance(obs, o); located && !crossMap && distance <= 4 {
			add("nearby", 0.10)
		}

	case KindGoTo:
		lowNote := strings.ToLower(o.Note)
		lowPlace := strings.ToLower(string(o.Place))
		if strings.Contains(lowNote, "unvisited adjacent map") {
			add("frontier", 0.30)
		}
		if naturalCenterPlace(lowPlace) && (partyHurt(obs) || leadOutOfPP(obs)) {
			add("recovery-stop", 0.28)
		}
		if naturalMartPlace(lowPlace) {
			if economy := EconomyContext(obs); economy != nil && economy.ResupplyNeeded {
				add("resupply-stop", 0.24)
			}
		}

	case KindCatch:
		switch {
		case obs.PartyCount <= 2:
			add("build-party", 0.30)
		case obs.PartyCount == 3:
			add("build-party", 0.22)
		case obs.PartyCount == 4:
			add("team-option", 0.10)
		default:
			add("team-option", 0.03)
		}
		if o.Place == "" {
			add("local-encounter", 0.06)
		} else {
			add("known-habitat", 0.03)
		}

	case KindTrain:
		if mon, ok := naturalTrainingTarget(obs, o); ok && len(obs.Party) > 0 {
			gap := int(obs.Party[0].Level) - int(mon.Level)
			switch {
			case o.Species != "" && gap >= 4:
				add("catch-up-party", 0.28)
			case o.Species != "" && gap >= 2:
				add("catch-up-party", 0.20)
			case o.Species != "":
				add("develop-party", 0.10)
			default:
				add("prepare-lead", 0.05)
			}
		}

	case KindTrainer:
		if naturalUnderlevelledParty(obs) {
			add("useful-training", 0.12)
		} else {
			add("route-trainer", 0.05)
		}
	}

	// Optional actions should not become an endless rhythm merely because
	// they have a positive Adventure flavor. Exact repeats in the short run
	// history lose some appeal; one-shot NPCs/items/catches are normally
	// removed by Knowledge/game state before this is even needed.
	if repeats := recentNaturalObjectiveCount(obs, o); repeats > 0 && naturalRepeatSensitive(o) {
		penalty := float64(repeats) * 0.08
		if penalty > 0.24 {
			penalty = 0.24
		}
		penalize("recent-repeat", penalty)
	}

	// #275's fallback is deliberately layered after the ordinary Adventure
	// routine. It can temporarily favor plausible alternate discovery and
	// preparation, but it stays bounded and never removes the normal score.
	s.Stall = stallFallbackSignal(obs, o, profile)
	if s.Stall.Context.Active {
		s.Bonus += s.Stall.Bonus
		s.RepeatPenalty += s.Stall.Penalty
		for _, tag := range s.Stall.Tags {
			s.Tags = appendNaturalTag(s.Tags, tag)
		}
	}

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
