package game

import "sort"

// TypeID is a portable, lower-case type name ("water", "steel"). The set of
// types is never listed here: it is whatever the cartridge's own type chart
// contains, so a generation that adds types needs no change in this package.
type TypeID string

// NeutralEffect is an ordinary-damage multiplier in tenths (x1.0). Every
// generation's chart stores multipliers this way.
const NeutralEffect = 10

// TypeChart is a cartridge's type-effectiveness table, read from its ROM by
// the game adapter.
type TypeChart interface {
	// AttackTypes lists every attacking type the chart knows.
	AttackTypes() []TypeID
	// Effectiveness is the damage multiplier in tenths for an attack of type
	// attack against a defender with the given types.
	Effectiveness(attack TypeID, defend []TypeID) int
}

// ChallengeOpponent is one opposing Pokémon in a scripted challenge.
type ChallengeOpponent struct {
	Species SpeciesID `json:"species"`
	Level   int       `json:"level"`
	Types   []TypeID  `json:"types"`
}

// Matchup is the portable type assessment of one challenge's opposing team.
type Matchup struct {
	Opponents []ChallengeOpponent `json:"opponents,omitempty"`
	MaxLevel  int                 `json:"max_level,omitempty"`
	// Preferred attack types, strongest first: super-effective against some
	// opponent, immune for none, and better than neutral over the whole team
	// once each opponent is weighted by its level.
	Preferred []TypeID `json:"preferred,omitempty"`
	// Useless attack types deal under half of neutral damage over the
	// level-weighted team (immunities, or resistance everywhere).
	Useless []TypeID `json:"useless,omitempty"`
}

// AssessMatchup scores every attack type in chart against opponents.
func AssessMatchup(chart TypeChart, opponents []ChallengeOpponent) Matchup {
	out := Matchup{Opponents: append([]ChallengeOpponent(nil), opponents...)}
	if chart == nil || len(opponents) == 0 {
		return out
	}
	neutral := 0
	for _, opp := range opponents {
		if opp.Level > out.MaxLevel {
			out.MaxLevel = opp.Level
		}
		neutral += weight(opp) * NeutralEffect
	}
	type scored struct {
		t     TypeID
		score int
	}
	var preferred []scored
	for _, t := range chart.AttackTypes() {
		score, super, immune := 0, false, false
		for _, opp := range opponents {
			e := chart.Effectiveness(t, opp.Types)
			score += weight(opp) * e
			super = super || e > NeutralEffect
			immune = immune || e == 0
		}
		if super && !immune && score > neutral {
			preferred = append(preferred, scored{t, score})
		}
		if 2*score < neutral {
			out.Useless = append(out.Useless, t)
		}
	}
	sort.Slice(preferred, func(i, j int) bool {
		if preferred[i].score != preferred[j].score {
			return preferred[i].score > preferred[j].score
		}
		return preferred[i].t < preferred[j].t
	})
	for _, p := range preferred {
		out.Preferred = append(out.Preferred, p.t)
	}
	sort.Slice(out.Useless, func(i, j int) bool { return out.Useless[i] < out.Useless[j] })
	return out
}

// weight makes a level-0 (unknown level) opponent still count once.
func weight(opp ChallengeOpponent) int {
	if opp.Level < 1 {
		return 1
	}
	return opp.Level
}

// Prefers reports whether t is one of the matchup's preferred attack types.
func (m Matchup) Prefers(t TypeID) bool { return containsType(m.Preferred, t) }

// IsUseless reports whether t is one of the matchup's useless attack types.
func (m Matchup) IsUseless(t TypeID) bool { return containsType(m.Useless, t) }

// Known reports whether the matchup was assessed from real opponent data.
func (m Matchup) Known() bool { return len(m.Opponents) > 0 }

func containsType(list []TypeID, t TypeID) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}
