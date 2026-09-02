package agent

import (
	"fmt"
	"strings"
)

// CoreEvalCases is the small, checked-in ROM-free decision corpus used by
// cmd/agent-eval. Each case is deliberately about information already present
// in Observation and an exact offered menu: no emulator state, hidden ROM
// knowledge, or free-form chain-of-thought is required to score it.
func CoreEvalCases() []EvalCase {
	healHere := Objective{Kind: KindHeal}
	train10 := Objective{Kind: KindTrain, Level: 10}
	train12 := Objective{Kind: KindTrain, Level: 12}
	gym := Objective{Kind: KindGym}
	goPallet := Objective{Kind: KindGoTo, Place: "pallet town"}
	goViridian := Objective{Kind: KindGoTo, Place: "viridian city"}
	goPewter := Objective{Kind: KindGoTo, Place: "pewter city"}
	goCerulean := Objective{Kind: KindGoTo, Place: "cerulean city"}

	return []EvalCase{
		{
			Name: "injured party heals before training",
			Obs: Observation{
				MapName: "Pewter City", Round: 4, RoundsLeft: 12,
				PartyCount: 1, Party: []PartyMon{{Level: 9, HP: 3, MaxHP: 28}},
			},
			Offered: []Objective{train12, healHere},
			Accept:  []string{healHere.String()},
		},
		{
			Name: "healthy party trains in grass",
			Obs: Observation{
				MapName: "Route 2", Round: 3, RoundsLeft: 14, HasGrass: true,
				PartyCount: 1, Party: []PartyMon{{Level: 8, HP: 25, MaxHP: 25}},
			},
			Offered: []Objective{goPallet, train10},
			Accept:  []string{train10.String()},
		},
		{
			Name: "healthy party at gym challenges leader",
			Obs: Observation{
				MapName: "Pewter Gym", Round: 7, RoundsLeft: 9,
				PartyCount: 1, Party: []PartyMon{{Level: 13, HP: 35, MaxHP: 35}},
			},
			Offered: []Objective{healHere, gym},
			Accept:  []string{gym.String()},
		},
		{
			Name: "last round favors immediate gym attempt",
			Obs: Observation{
				MapName: "Pewter Gym", Round: 20, RoundsLeft: 1,
				PartyCount: 1, Party: []PartyMon{{Level: 12, HP: 31, MaxHP: 34}},
			},
			Offered: []Objective{goViridian, gym},
			Accept:  []string{gym.String()},
		},
		{
			Name: "fresh travel intent can continue",
			Obs: Observation{
				MapName: "Route 1", Round: 2, RoundsLeft: 18,
				Intent: "reach viridian city", IntentAge: 0,
				PartyCount: 1, Party: []PartyMon{{Level: 7, HP: 22, MaxHP: 22}},
			},
			Offered: []Objective{goPallet, goViridian},
			Accept:  []string{goViridian.String()},
		},
		{
			Name: "old stalled travel intent changes approach",
			Obs: Observation{
				MapName: "Route 2", Round: 9, RoundsLeft: 10, HasGrass: true,
				Intent: "reach pewter city", IntentAge: 6,
				PartyCount: 1, Party: []PartyMon{{Level: 8, HP: 24, MaxHP: 24}},
				History: []RoundRecord{
					{Objective: goPewter.String(), Outcome: "failed: blocked"},
					{Objective: goPewter.String(), Outcome: "failed: blocked"},
				},
			},
			Offered: []Objective{goPewter, train10},
			Accept:  []string{train10.String()},
		},
		{
			Name: "recent repeated failure is not retried immediately",
			Obs: Observation{
				MapName: "Viridian City", Round: 6, RoundsLeft: 12,
				PartyCount: 1, Party: []PartyMon{{Level: 8, HP: 24, MaxHP: 24}},
				History: []RoundRecord{
					{Objective: goPewter.String(), Outcome: "failed: route blocked"},
					{Objective: goPewter.String(), Outcome: "failed: route blocked"},
				},
			},
			Offered: []Objective{goPewter, goPallet},
			Accept:  []string{goPallet.String()},
		},
		{
			Name: "badge progress does not backtrack to old city",
			Obs: Observation{
				MapName: "Cerulean City", Round: 15, RoundsLeft: 8,
				Badges:     []string{"Boulder"},
				PartyCount: 1, Party: []PartyMon{{Level: 18, HP: 45, MaxHP: 45}},
			},
			Offered: []Objective{goPewter, goCerulean},
			Accept:  []string{goCerulean.String()},
		},
		{
			Name: "hurt party heals instead of backtracking",
			Obs: Observation{
				MapName: "Viridian City", Round: 5, RoundsLeft: 11,
				PartyCount: 1, Party: []PartyMon{{Level: 9, HP: 1, MaxHP: 27}},
			},
			Offered: []Objective{goPallet, healHere},
			Accept:  []string{healHere.String()},
		},
		{
			Name: "training target already met is not chosen",
			Obs: Observation{
				MapName: "Route 2", Round: 8, RoundsLeft: 10, HasGrass: true,
				PartyCount: 1, Party: []PartyMon{{Level: 12, HP: 34, MaxHP: 34}},
			},
			Offered: []Objective{train12, goPewter},
			Accept:  []string{goPewter.String()},
		},
	}
}

// ValidateEvalCases rejects corpus mistakes before a planner is scored. It is
// intentionally stricter than EvaluatePlanner's generic API: a checked-in
// benchmark case must name at least one accepted offered objective, or
// explicitly allow ErrDone.
func ValidateEvalCases(cases []EvalCase) error {
	seen := make(map[string]bool, len(cases))
	for i, tc := range cases {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			return fmt.Errorf("agent: eval case %d has an empty name", i)
		}
		if seen[name] {
			return fmt.Errorf("agent: duplicate eval case name %q", name)
		}
		seen[name] = true
		if len(tc.Offered) == 0 && !tc.AllowDone {
			return fmt.Errorf("agent: eval case %q offers nothing and does not allow done", name)
		}
		if len(tc.Accept) == 0 && !tc.AllowDone {
			return fmt.Errorf("agent: eval case %q has no accepted outcome", name)
		}
		offered := make(map[string]bool, len(tc.Offered))
		for _, o := range tc.Offered {
			offered[o.String()] = true
		}
		for _, want := range tc.Accept {
			if !offered[want] {
				return fmt.Errorf("agent: eval case %q accepts %q, which is not offered", name, want)
			}
		}
	}
	return nil
}
