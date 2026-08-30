package agent_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

// The table below is the whole point of Offer: the menu CHANGES with the
// situation. Every case is a synthetic observation — no ROM, no model — and
// the expected list is exact, in Offer's order (starters, places by name,
// then the verbs), so a place that slips onto the menu it should not be on
// fails the case.
//
// Map facts used here come from skill's place table: pallet town 0x00,
// route 1 0x0c, viridian city 0x01, viridian pokemon center 0x29,
// viridian mart 0x2a, pewter city 0x02, pewter gym 0x36.
func TestOfferTable(t *testing.T) {
	adj := map[uint8][]uint8{
		0x00: {0x0c},
		0x0c: {0x00, 0x01},
		0x01: {0x0c, 0x29},
	}

	cases := []struct {
		name    string
		obs     agent.Observation
		known   func() *agent.Knowledge
		want    []string
		mustNot []string // extra negative assertions, for the cases that matter
	}{
		{
			name: "fresh boot at pallet: starters, one step out, nothing else",
			obs:  agent.Observation{Map: 0x00, MapName: "PALLET_TOWN", X: 5, Y: 6, PartyCount: 0},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				return k
			},
			want: []string{
				"take the charmander starter",
				"take the squirtle starter",
				"take the bulbasaur starter",
				"go to route 1", // the door of where you stand
			},
			mustNot: []string{"pewter", "heal", "catch", "train", "gym", "parcel"},
		},
		{
			name: "on route 1 with a party, balls and grass: one catch per species the map rolls",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:    []agent.PartyMon{{Level: 5, HP: 20, MaxHP: 20}},
				Bag:      []agent.Item{{Name: "pokeball", Quantity: 5}},
				HasGrass: true,
				// Route1WildMons, as the ROM has it: no CATERPIE anywhere
				// on this map, which the old fixed menu offered anyway.
				WildGrass: []agent.WildSpecies{
					{Name: "pidgey", MinLevel: 2, MaxLevel: 5, Slots: 6},
					{Name: "rattata", MinLevel: 2, MaxLevel: 4, Slots: 4},
				},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"go to pallet town",
				"go to viridian city", // one step out of route 1
				"deliver oak's parcel",
				"catch a PIDGEY here",
				"catch a RATTATA here",
				"train the lead to level 7",
			},
			mustNot: []string{"starter", "heal", "CATERPIE"},
		},
		{
			name: "balls but the map rolls nothing: catch stays off — the hunt needs a wild table",
			obs: agent.Observation{
				Map: 0x00, MapName: "PALLET_TOWN", X: 4, Y: 7, PartyCount: 1,
				Bag:    []agent.Item{{Name: "pokeball", Quantity: 5}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				return k
			},
			want: []string{
				"go to pallet town",
				"go to route 1",
				"deliver oak's parcel",
			},
			mustNot: []string{"catch", "train"},
		},
		{
			name: "hurt party in the field: the walk back to a known center is one objective",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: 1, Level: 6, HP: 4, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				for _, m := range []uint8{0x00, 0x0c, 0x01, 0x29} {
					k.SawMap(m) // the run has been inside the Viridian center
				}
				return k
			},
			want: []string{
				"go to pallet town",
				"go to viridian city",
				"go to viridian pokemon center",
				"deliver oak's parcel",
				"heal the party at VIRIDIAN POKEMON CENTER",
			},
		},
		{
			name: "healthy party in the field: no heal — a full heal is a round that changes nothing",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: 1, Level: 6, HP: 20, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				for _, m := range []uint8{0x00, 0x0c, 0x01, 0x29} {
					k.SawMap(m)
				}
				return k
			},
			want: []string{
				"go to pallet town",
				"go to viridian city",
				"go to viridian pokemon center",
				"deliver oak's parcel",
			},
			mustNot: []string{"heal"},
		},
		{
			name: "hurt party but no center the run has been inside: no heal it cannot reach",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: 1, Level: 6, HP: 4, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"go to pallet town",
				"go to viridian city",
				"deliver oak's parcel",
			},
			mustNot: []string{"heal"},
		},
		{
			name: "inside a center: heal joins; no balls, so no catch",
			obs: agent.Observation{
				Map: 0x29, MapName: "VIRIDIAN_POKECENTER", X: 4, Y: 5, PartyCount: 1,
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				for _, m := range []uint8{0x00, 0x0c, 0x01, 0x29} {
					k.SawMap(m)
				}
				return k
			},
			want: []string{
				"go to pallet town",
				"go to route 1",
				"go to viridian city",
				"go to viridian pokemon center",
				"deliver oak's parcel",
				"heal the party",
			},
			mustNot: []string{"catch", "train", "gym"},
		},
		{
			name: "at the gym underlevelled: the gym is STILL offered — Offer never filters on wisdom",
			obs: agent.Observation{
				Map: 0x36, MapName: "PEWTER_GYM", X: 5, Y: 3, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: 7, Level: 5, HP: 1, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x36)
				return k
			},
			want: []string{
				"go to pewter gym",
				"deliver oak's parcel",
				"beat the gym leader here",
			},
		},
		{
			name: "unvisited and unmentioned places stay off the menu",
			obs: agent.Observation{
				Map: 0x00, MapName: "PALLET_TOWN", X: 4, Y: 7, PartyCount: 1,
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				return k
			},
			want: []string{
				"go to pallet town",
				"go to route 1",
				"deliver oak's parcel",
			},
			mustNot: []string{"pewter city", "viridian city", "forest", "lab"},
		},
		{
			name: "a place the game named in dialogue joins the menu",
			obs: agent.Observation{
				Map: 0x00, MapName: "PALLET_TOWN", X: 4, Y: 7, PartyCount: 1,
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawDialogue([]string{"An old man said: Pewter City lies to the east."})
				return k
			},
			want: []string{
				"go to pallet town",
				"go to pewter city",
				"go to route 1",
				"deliver oak's parcel",
			},
		},
		{
			name: "inside the mart: buy joins",
			obs: agent.Observation{
				Map: 0x2a, MapName: "VIRIDIAN_MART", X: 3, Y: 6, PartyCount: 1,
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x2a)
				return k
			},
			want: []string{
				"go to viridian mart",
				"deliver oak's parcel",
				"buy 3 POTION",
			},
		},
		{
			name: "a completed one-shot stays off; repeatable verbs do not",
			obs: agent.Observation{
				Map: 0x29, MapName: "VIRIDIAN_POKECENTER", X: 4, Y: 5, PartyCount: 1,
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x29)
				k.Done(agent.Objective{Kind: agent.KindErrand})
				return k
			},
			want: []string{
				"go to viridian pokemon center",
				"heal the party", // a completed heal is no reason to stop offering heals
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agent.Offer(tc.obs, tc.known())
			var gotNames []string
			for _, o := range got {
				gotNames = append(gotNames, o.String())
			}
			if !reflect.DeepEqual(gotNames, tc.want) {
				t.Fatalf("Offer = %v\nwant    %v", gotNames, tc.want)
			}
			joined := strings.Join(gotNames, "\n")
			for _, bad := range tc.mustNot {
				if strings.Contains(joined, bad) {
					t.Errorf("menu must not contain %q:\n%s", bad, joined)
				}
			}
		})
	}
}

// TestOfferWithholdsParcelUntilStarterStoryComplete catches the exact live
// failure where the model delivered Oak's parcel on round one, before taking a
// starter. PartyCount alone is not the story postcondition: the rival battle in
// Oak's lab must have completed before the parcel skill's preconditions hold.
func TestOfferWithholdsParcelUntilStarterStoryComplete(t *testing.T) {
	known := agent.NewKnowledge(nil)
	before := agent.Observation{Map: 0x00, MapName: "PALLET_TOWN", X: 5, Y: 6, PartyCount: 1}
	after := before
	after.Events = []string{"BattledRivalInOaksLab"}

	hasParcel := func(obs agent.Observation) bool {
		for _, objective := range agent.Offer(obs, known) {
			if objective.Kind == agent.KindErrand {
				return true
			}
		}
		return false
	}

	if hasParcel(before) {
		t.Fatal("parcel offered before the starter story and rival battle completed")
	}
	if !hasParcel(after) {
		t.Fatal("parcel not offered after the starter story and rival battle completed")
	}
}

// TestOfferMenuChangesWithSituation is the regression this task exists to
// pin: the same run, two rounds apart, sees two different menus. A menu
// built once at startup offers the identical list forever; Offer must not.
func TestOfferMenuChangesWithSituation(t *testing.T) {
	adj := map[uint8][]uint8{0x00: {0x0c}, 0x0c: {0x00, 0x01}}

	fresh := agent.Observation{Map: 0x00, MapName: "PALLET_TOWN", X: 5, Y: 6, PartyCount: 0}
	later := agent.Observation{
		Map: 0x29, MapName: "VIRIDIAN_POKECENTER", X: 4, Y: 5, PartyCount: 1,
		Bag: []agent.Item{{Name: "pokeball", Quantity: 3}},
	}

	k := agent.NewKnowledge(adj)
	k.SawMap(0x00)
	first := agent.Offer(fresh, k)

	k.SawMap(0x0c)
	k.SawMap(0x01)
	k.SawMap(0x29)
	k.Done(agent.Objective{Kind: agent.KindErrand})
	second := agent.Offer(later, k)

	firstSet := map[string]bool{}
	for _, o := range first {
		firstSet[o.String()] = true
	}
	secondSet := map[string]bool{}
	for _, o := range second {
		secondSet[o.String()] = true
	}
	if reflect.DeepEqual(firstSet, secondSet) {
		t.Fatalf("menu did not change with the situation: %v", firstSet)
	}
	// The menu shrank where it should have and grew where it should have.
	if firstSet["heal the party"] {
		t.Errorf("heal offered at pallet town, which has no center")
	}
	if !secondSet["heal the party"] {
		t.Errorf("heal not offered inside a center")
	}
	if secondSet["take the charmander starter"] {
		t.Errorf("starter offered to a player who already has a Pokemon")
	}
	if !firstSet["take the charmander starter"] {
		t.Errorf("starter not offered while the party is empty")
	}
}

// TestKnowledgeDialogueMentions: names enter Knowledge only when the game
// said them, and matching respects word boundaries — "Route 22" must not
// plant a false memory of "route 2".
func TestKnowledgeDialogueMentions(t *testing.T) {
	k := agent.NewKnowledge(nil)
	k.SawDialogue([]string{
		"The sign reads: Route 22. Beware the rival.",
		"Nothing here names a place at all.",
	})
	if !k.Places["route 22"] {
		t.Errorf("dialogue named route 22; Knowledge does not know it")
	}
	if k.Places["route 2"] {
		t.Errorf("'Route 22' must not count as a mention of 'route 2'")
	}
	if len(k.Places) != 1 {
		t.Errorf("Knowledge.Places = %v, want exactly {route 22}", k.Places)
	}

	k.SawDialogue([]string{"In Pallet Town, Oak waits."})
	if !k.Places["pallet town"] {
		t.Errorf("dialogue named Pallet Town; Knowledge does not know it")
	}
}

// TestOfferMapObjects: people and items on the map become objectives;
// trainers are reported in MapObjects but NOT offered — there is no fight
// verb behind them yet, so offering one would manufacture a guaranteed
// failed objective every round. Synthetic observation: the split lives in
// Offer, not in Observe.
func TestOfferMapObjects(t *testing.T) {
	obs := agent.Observation{
		Map: 0x99, X: 5, Y: 6, PartyCount: 1, // a map no place names
		Events: []string{"BattledRivalInOaksLab"},
		MapObjects: []agent.MapObject{
			{X: 7, Y: 10, Kind: "person"},
			{X: 2, Y: 4, Kind: "trainer"},
			{X: 9, Y: 2, Kind: "trainer"},
			{X: 5, Y: 6, Kind: "item", Item: "pokeball"},
		},
	}
	out := agent.Offer(obs, agent.NewKnowledge(nil))

	var got []string
	for _, o := range out {
		got = append(got, o.String())
	}
	want := []string{
		"deliver oak's parcel",
		"talk at (7,10)",
		"pick up the POKEBALL at (5,6)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Offer = %v, want %v", got, want)
	}
	for _, s := range got {
		if strings.Contains(s, "(2,4)") || strings.Contains(s, "(9,2)") {
			t.Errorf("a trainer is on the offered list: %q; no fight verb exists behind it", s)
		}
	}
}

func TestOfferDoesNotRepeatCompletedTalk(t *testing.T) {
	obs := agent.Observation{
		Map: 0x28,
		MapObjects: []agent.MapObject{
			{X: 8, Y: 3, Kind: "person"},
			{X: 5, Y: 2, Kind: "person"},
		},
	}
	known := agent.NewKnowledge(nil)
	known.TalkedTo(0x28, 8, 3)

	got := agent.Offer(obs, known)
	for _, objective := range got {
		if objective == (agent.Objective{Kind: agent.KindTalk, X: 8, Y: 3}) {
			t.Fatalf("Offer repeated completed objective %q", objective)
		}
	}
	want := agent.Objective{Kind: agent.KindTalk, X: 5, Y: 2}
	found := false
	for _, objective := range got {
		if objective == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("Offer omitted untried objective %q: %v", want, got)
	}
}

func TestOfferTrainingTargetTracksTheLead(t *testing.T) {
	known := agent.NewKnowledge(nil)
	obs := agent.Observation{
		Map: 0x0c, HasGrass: true, PartyCount: 1,
		Party: []agent.PartyMon{{Level: 11}},
	}
	trainTarget := func(obs agent.Observation) (uint8, bool) {
		for _, objective := range agent.Offer(obs, known) {
			if objective.Kind == agent.KindTrain {
				return objective.Level, true
			}
		}
		return 0, false
	}

	// The target is always a step the run has not taken. Under the old
	// fixed target of 12 a level-12 lead was offered no training at all,
	// whatever it was about to walk into.
	for _, tc := range []struct{ lead, want uint8 }{{5, 7}, {11, 13}, {12, 14}, {40, 42}} {
		obs.Party = []agent.PartyMon{{Level: tc.lead}}
		got, ok := trainTarget(obs)
		if !ok {
			t.Fatalf("lead level %d: training not offered", tc.lead)
		}
		if got != tc.want {
			t.Errorf("lead level %d: offered target %d, want %d", tc.lead, got, tc.want)
		}
	}

	// A lead at the ceiling has no next rung, and no party has nothing to
	// train: both are objectives that could only fail.
	obs.Party = []agent.PartyMon{{Level: 100}}
	if got, ok := trainTarget(obs); ok {
		t.Errorf("training offered at level %d for a level-100 lead; there is no rung above it", got)
	}
	obs.Party, obs.PartyCount = nil, 0
	if got, ok := trainTarget(obs); ok {
		t.Errorf("training offered to level %d with an empty party", got)
	}
}

// TestOfferGymIsNotPewterOnly: the gym challenge follows the player into
// whichever gym they are standing in, and stops being offered once that
// gym's badge is in hand — that leader will not rebattle, so the challenge
// could only fail. Under the Pewter-only gate, standing in the Cerulean Gym
// offered no gym objective at all.
func TestOfferGymIsNotPewterOnly(t *testing.T) {
	gymObjective := func(obs agent.Observation) bool {
		known := agent.NewKnowledge(nil)
		known.SawMap(obs.Map)
		for _, o := range agent.Offer(obs, known) {
			if o.Kind == agent.KindGym {
				return true
			}
		}
		return false
	}

	cerulean := agent.Observation{
		Map: 0x41, MapName: "CERULEAN_GYM", X: 4, Y: 3, PartyCount: 1,
		Party:  []agent.PartyMon{{Level: 20, HP: 40, MaxHP: 40}},
		Badges: []string{"Boulder"},
	}
	if !gymObjective(cerulean) {
		t.Error("no gym objective in the Cerulean Gym; the verb was Pewter-only")
	}

	pewter := cerulean
	pewter.Map, pewter.MapName, pewter.X, pewter.Y = 0x36, "PEWTER_GYM", 4, 2
	if gymObjective(pewter) {
		t.Error("gym offered in Pewter with the Boulder Badge already held; Brock will not rebattle")
	}
	pewter.Badges = nil
	if !gymObjective(pewter) {
		t.Error("gym not offered in Pewter without the badge")
	}

	// Underlevelled is still offered: Offer reports what is possible, and
	// losing stays the planner's mistake to make.
	pewter.Party = []agent.PartyMon{{Level: 5, HP: 2, MaxHP: 20}}
	if !gymObjective(pewter) {
		t.Error("gym withheld from an underlevelled party; Offer must not filter on wisdom")
	}
}
