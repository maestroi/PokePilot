package agent

import (
	"fmt"
	"sort"
	"strings"
)

// StallContext is safe, explicit telemetry about recent lack of progress. It
// is reconstructed from the observation's bounded six-round history plus
// persistent failures and route/prerequisite evidence; it is not hidden model
// reasoning and it does not make an objective legal.
type StallContext struct {
	Active       bool
	Level        int
	Pressure     float64
	Navigation   bool
	Prerequisite bool
	Battle       bool
	Loop         bool
	Reasons      []string
}

// StallFallbackSignal is the candidate-specific adjustment used while
// Adventure is in an alternate-search phase. Bonus rewards plausible ways to
// learn or satisfy a missing prerequisite; Penalty suppresses hammering the
// same failed/repeated alternative forever.
type StallFallbackSignal struct {
	Bonus   float64
	Penalty float64
	Tags    []string
	Context StallContext
}

func (s StallFallbackSignal) Net() float64 { return s.Bonus - s.Penalty }

func detectStallContext(obs Observation) StallContext {
	var ctx StallContext
	if len(obs.History) == 0 {
		return ctx
	}

	// A completed named progression objective or gym is direct evidence that
	// the prior uncertainty resolved. Old Knowledge.Failures intentionally do
	// not keep fallback pressure alive after that fresh progress.
	latest := obs.History[len(obs.History)-1]
	if latest.Outcome == "done" && strongProgressObjective(latest.Objective) {
		return ctx
	}

	recentFailed := map[string]int{}
	failedRecords := 0
	battleFailures := 0
	navigationFailures := 0
	for _, r := range obs.History {
		if r.Outcome == "done" {
			continue
		}
		failedRecords++
		recentFailed[r.Objective]++
		low := strings.ToLower(r.Objective + " " + r.Outcome)
		if looksBattleFailure(low) {
			battleFailures++
		}
		if looksNavigationFailure(low) {
			navigationFailures++
		}
	}

	maxPersistentFailure := 0
	for _, f := range obs.Failures {
		if recentFailed[f.Objective] == 0 {
			continue // stale failure outside the bounded current episode
		}
		if f.Times > maxPersistentFailure {
			maxPersistentFailure = f.Times
		}
	}

	if maxPersistentFailure >= 2 {
		ctx.Pressure += minFloat(0.40, 0.20+float64(maxPersistentFailure-1)*0.08)
		ctx.Reasons = appendStallReason(ctx.Reasons, "repeat-failure")
	}
	if failedRecords >= 2 {
		ctx.Pressure += minFloat(0.22, float64(failedRecords-1)*0.08)
		ctx.Reasons = appendStallReason(ctx.Reasons, "blocked-rounds")
	}

	if recentObjectiveLoop(obs.History) {
		ctx.Loop = true
		ctx.Pressure += 0.32
		ctx.Reasons = appendStallReason(ctx.Reasons, "route-loop")
	}

	if obs.IntentAge >= 3 && (failedRecords > 0 || ctx.Loop) {
		ctx.Pressure += 0.08
		ctx.Reasons = appendStallReason(ctx.Reasons, "stale-intent")
	}

	if navigationFailures > 0 || len(obs.Unroutable) > 0 {
		ctx.Navigation = true
		ctx.Reasons = appendStallReason(ctx.Reasons, "navigation")
	}
	if battleFailures > 0 {
		ctx.Battle = true
		ctx.Reasons = appendStallReason(ctx.Reasons, "battle-gate")
	}
	if len(obs.Requirements) > 0 {
		ctx.Prerequisite = true
		ctx.Reasons = appendStallReason(ctx.Reasons, "known-requirement")
	}
	for _, blockage := range obs.RouteBlockages {
		if len(blockage.Missing) == 0 {
			continue
		}
		ctx.Navigation = true
		ctx.Prerequisite = true
		ctx.Reasons = appendStallReason(ctx.Reasons, "missing-capability")
		break
	}

	// One transient failure is normal gameplay, not a mode switch.
	if ctx.Pressure < 0.20 {
		return StallContext{}
	}

	// A successful non-progression discovery immediately after the failed
	// episode is evidence that the search is producing information. Decay the
	// pressure instead of snapping it to zero; if it revealed a requirement,
	// the planner can still act on that new fact next.
	if latest.Outcome == "done" && discoveryObjective(latest.Objective) && !ctx.Loop {
		ctx.Pressure *= 0.55
		ctx.Reasons = appendStallReason(ctx.Reasons, "discovery-decay")
	}
	if ctx.Pressure < 0.20 {
		return StallContext{}
	}
	if ctx.Pressure > 1 {
		ctx.Pressure = 1
	}
	ctx.Active = true
	switch {
	case ctx.Pressure >= 0.75:
		ctx.Level = 3
	case ctx.Pressure >= 0.45:
		ctx.Level = 2
	default:
		ctx.Level = 1
	}
	return ctx
}

func stallFallbackSignal(obs Observation, o Objective, profile PlayStyleProfile) StallFallbackSignal {
	if profile.Name != "adventure" {
		return StallFallbackSignal{}
	}
	ctx := detectStallContext(obs)
	if !ctx.Active {
		return StallFallbackSignal{}
	}

	s := StallFallbackSignal{Context: ctx}
	add := func(tag string, value float64) {
		if value <= 0 {
			return
		}
		s.Bonus += value
		s.Tags = appendStallTag(s.Tags, tag)
	}
	penalize := func(tag string, value float64) {
		if value <= 0 {
			return
		}
		s.Penalty += value
		s.Tags = appendStallTag(s.Tags, tag)
	}

	recentRepeats := recentObjectiveCount(obs.History, o.String())
	failedTimes := recentFailureTimes(obs, o.String())
	if failedTimes >= 2 {
		penalize("stalled-retry", 0.20+ctx.Pressure*0.25)
	}
	if recentRepeats >= 2 && fallbackAlternative(o) {
		penalize("exhausted-alternative", minFloat(0.36, float64(recentRepeats-1)*0.12))
	}

	switch o.Kind {
	case KindTalk:
		// Unvisited people are a high-quality search target for hidden story
		// permissions and hints; Offer removes them after successful dialogue.
		add("search-npc", 0.18+ctx.Pressure*0.24)
		if ctx.Prerequisite {
			add("prerequisite-npc", 0.12)
		}

	case KindGoTo:
		frontier := strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map")
		if frontier && (ctx.Navigation || ctx.Loop || ctx.Prerequisite) {
			add("search-frontier", 0.20+ctx.Pressure*0.30)
		}
		if !frontier && ctx.Navigation && recentRepeats == 0 {
			// A different already-known route is a plausible alternate entrance,
			// but deliberately worth less than a genuinely unexplored frontier.
			add("alternate-route", 0.08+ctx.Pressure*0.10)
		}

	case KindPickup:
		if ctx.Prerequisite && highImpactItem(o.Item) {
			add("prerequisite-item", 0.20+ctx.Pressure*0.22)
		} else if ctx.Prerequisite && pickupLooksUseful(o) {
			add("useful-clue-item", 0.10+ctx.Pressure*0.10)
		}

	case KindUseItem:
		if ctx.Prerequisite && highImpactItem(o.Item) {
			add("prepare-capability", 0.22+ctx.Pressure*0.24)
		}

	case KindHeal:
		if (ctx.Battle || ctx.Navigation) && (partyHurt(obs) || leadOutOfPP(obs)) {
			add("recover-before-retry", 0.14+ctx.Pressure*0.12)
		}

	case KindBuy:
		if economy := EconomyContext(obs); economy != nil && economy.ResupplyNeeded {
			add("resupply-before-retry", 0.12+ctx.Pressure*0.12)
		}

	case KindTrain:
		if ctx.Battle {
			add("prepare-battle-gate", 0.20+ctx.Pressure*0.24)
		}

	case KindCatch:
		if ctx.Battle && obs.PartyCount < 4 {
			add("recruit-for-battle", 0.12+ctx.Pressure*0.16)
		}

	case KindTrainer:
		if ctx.Battle && naturalUnderlevelledParty(obs) {
			add("trainer-preparation", 0.10+ctx.Pressure*0.12)
		}
	}

	// The fallback is intentionally bounded. It can redirect a few choices,
	// not overwhelm story scoring forever. Repetition penalties use the same
	// cap so repeatedly testing one alternative eventually loses to another.
	if s.Bonus > 0.65 {
		s.Bonus = 0.65
	}
	if s.Penalty > 0.65 {
		s.Penalty = 0.65
	}
	return s
}

func recentFailureTimes(obs Observation, objective string) int {
	for _, f := range obs.Failures {
		if f.Objective == objective {
			return f.Times
		}
	}
	return 0
}

func recentObjectiveCount(history []RoundRecord, objective string) int {
	count := 0
	for _, r := range history {
		if r.Objective == objective {
			count++
		}
	}
	return count
}

func recentObjectiveLoop(history []RoundRecord) bool {
	if len(history) < 4 {
		return false
	}
	window := history[len(history)-4:]
	unique := map[string]int{}
	for _, r := range window {
		low := strings.ToLower(r.Objective)
		if !strings.HasPrefix(low, "go to ") && !strings.HasPrefix(low, "progress ") {
			return false
		}
		unique[r.Objective]++
	}
	if len(unique) > 2 {
		return false
	}
	for _, n := range unique {
		if n < 2 {
			return false
		}
	}
	return true
}

func strongProgressObjective(objective string) bool {
	low := strings.ToLower(objective)
	return strings.HasPrefix(low, "progress ") || strings.HasPrefix(low, "beat the gym leader")
}

func discoveryObjective(objective string) bool {
	low := strings.ToLower(objective)
	return strings.HasPrefix(low, "talk at ") ||
		strings.HasPrefix(low, "pick up ") ||
		strings.HasPrefix(low, "catch a ") ||
		strings.HasPrefix(low, "go to ")
}

func looksBattleFailure(low string) bool {
	return strings.Contains(low, "gym") || strings.Contains(low, "trainer") ||
		strings.Contains(low, "blacked out") || strings.Contains(low, "lost to") ||
		strings.HasPrefix(low, "train ")
}

func looksNavigationFailure(low string) bool {
	return strings.HasPrefix(low, "go to ") || strings.Contains(low, "no route") ||
		strings.Contains(low, "no path") || strings.Contains(low, "unreachable") ||
		strings.Contains(low, "route gate") || strings.Contains(low, "navigation")
}

func pickupLooksUseful(o Objective) bool {
	low := strings.ToLower(o.Note)
	return strings.Contains(low, "high-value preparation") || strings.Contains(low, "useful preparation")
}

func fallbackAlternative(o Objective) bool {
	switch o.Kind {
	case KindTalk, KindPickup, KindUseItem, KindTrain, KindCatch, KindTrainer, KindBuy:
		return true
	case KindGoTo:
		return strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map")
	default:
		return false
	}
}

func appendStallReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func appendStallTag(tags []string, tag string) []string {
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func stallFallbackSummary(s StallFallbackSignal) string {
	if !s.Context.Active {
		return ""
	}
	reasons := append([]string(nil), s.Context.Reasons...)
	sort.Strings(reasons)
	tags := append([]string(nil), s.Tags...)
	sort.Strings(tags)
	adjustment := s.Net()
	label := fmt.Sprintf("stall L%d %+.2f", s.Context.Level, adjustment)
	if len(tags) > 0 {
		label += " " + strings.Join(tags, "+")
	}
	if len(reasons) > 0 {
		label += " due " + strings.Join(reasons, "+")
	}
	return label
}
