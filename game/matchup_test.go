package game

import (
	"slices"
	"testing"
)

// mapChart is a tiny synthetic chart: unspecified pairs are neutral.
type mapChart map[[2]TypeID]int

func (c mapChart) AttackTypes() []TypeID {
	seen := map[TypeID]bool{}
	var out []TypeID
	for k := range c {
		if !seen[k[0]] {
			seen[k[0]] = true
			out = append(out, k[0])
		}
	}
	slices.Sort(out)
	return out
}

func (c mapChart) Effectiveness(attack TypeID, defend []TypeID) int {
	e := NeutralEffect
	for i, d := range defend {
		if i > 0 && d == defend[0] {
			continue
		}
		if v, ok := c[[2]TypeID{attack, d}]; ok {
			e = e * v / NeutralEffect
		}
	}
	return e
}

func TestAssessMatchupFlagsImmuneAndPrefersSuperEffective(t *testing.T) {
	chart := mapChart{
		{"electric", "ground"}: 0,
		{"electric", "water"}:  20,
		{"water", "ground"}:    20,
		{"water", "rock"}:      20,
		{"normal", "rock"}:     5,
		{"grass", "ground"}:    20,
		{"grass", "poison"}:    5,
		{"fire", "water"}:      5,
	}
	// A Ground-heavy team with one neutral Normal mon (Giovanni's shape).
	team := []ChallengeOpponent{
		{Species: "dugtrio", Level: 42, Types: []TypeID{"ground"}},
		{Species: "persian", Level: 42, Types: []TypeID{"normal"}},
		{Species: "nidoqueen", Level: 44, Types: []TypeID{"poison", "ground"}},
		{Species: "rhydon", Level: 50, Types: []TypeID{"ground", "rock"}},
	}
	m := AssessMatchup(chart, team)
	if m.MaxLevel != 50 {
		t.Fatalf("max level = %d, want 50", m.MaxLevel)
	}
	if !m.IsUseless("electric") {
		t.Fatalf("electric not useless against a ground team: %+v", m)
	}
	if len(m.Preferred) == 0 || m.Preferred[0] != "water" {
		t.Fatalf("preferred = %v, want water first", m.Preferred)
	}
	if !m.Prefers("grass") {
		t.Fatalf("grass (super-effective on three, neutral on one) not preferred: %v", m.Preferred)
	}
	// Resisted on one mon only is not useless; neutral elsewhere is not preferred.
	if m.IsUseless("normal") || m.Prefers("normal") || m.Prefers("fire") {
		t.Fatalf("normal/fire misclassified: %+v", m)
	}
}

func TestAssessMatchupUnknownTeamIsUnknown(t *testing.T) {
	if m := AssessMatchup(mapChart{}, nil); m.Known() || len(m.Preferred) != 0 {
		t.Fatalf("empty team produced a matchup: %+v", m)
	}
}
