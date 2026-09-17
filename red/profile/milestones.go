package profile

import "github.com/maestroi/pokepilot/red/state"

// MajorMilestoneLabels returns completed story beats for live UI telemetry.
// Badges stay on Player.Badges; this list is the remaining major gates.
func MajorMilestoneLabels(facts state.StoryFacts) []string {
	type beat struct {
		done  bool
		label string
	}
	beats := []beat{
		{facts.PokedexAcquired, "Pokédex"},
		{facts.MtMoonFossilAcquired, "Mt. Moon fossil"},
		{facts.SSTicketAcquired, "S.S. Ticket"},
		{facts.HM01Acquired, "HM01"},
		{facts.SilphScopeAcquired, "Silph Scope"},
		{facts.PokeFluteAcquired, "Poké Flute"},
		{facts.HM03Acquired, "HM03 Surf"},
		{facts.HM04Acquired, "HM04 Strength"},
		{facts.CardKeyOwned, "Card Key"},
		{facts.SilphRescueComplete, "Silph rescue"},
		{facts.SecretKeyOwned, "Secret Key"},
		{facts.LeagueChampionDefeated, "Champion"},
		{facts.MainStoryComplete, "Hall of Fame"},
	}
	var out []string
	for _, b := range beats {
		if b.done {
			out = append(out, b.label)
		}
	}
	return out
}
