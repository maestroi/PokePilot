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
		mustNot []string
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
				"go to route 1",
				"go to route 1, fleeing wild battles",
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
				"deliver oak's parcel",
				"catch a PIDGEY here",
				"catch a RATTATA here",
				"train the lead to level 7",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
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
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to route 1",
				"go to route 1, fleeing wild battles",
			},
			mustNot: []string{"catch", "train"},
		},
		{
			name: "hurt party in the field: the walk back to a known center is one objective",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 4, MaxHP: 20}},
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
				"deliver oak's parcel",
				"heal the party at VIRIDIAN POKEMON CENTER",
				"heal the party at VIRIDIAN POKEMON CENTER, fleeing wild battles",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
				"go to viridian pokemon center",
				"go to viridian pokemon center, fleeing wild battles",
			},
		},
		{
			name: "healthy party in the field: no heal — a full heal is a round that changes nothing",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 20, MaxHP: 20}},
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
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
				"go to viridian pokemon center",
				"go to viridian pokemon center, fleeing wild battles",
			},
			mustNot: []string{"heal"},
		},
		{
			name: "hurt party but no center the run has been inside: no heal it cannot reach",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 4, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
			},
			mustNot: []string{"heal"},
		},
		{
			name: "hurt party with a potion in the bag: field healing joins without walking to a center",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 4, MaxHP: 20}},
				Bag:    []agent.Item{{Name: "potion", Quantity: 3}},
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
				"deliver oak's parcel",
				"heal the party at VIRIDIAN POKEMON CENTER",
				"heal the party at VIRIDIAN POKEMON CENTER, fleeing wild battles",
				"use a POTION on party slot 0",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
				"go to viridian pokemon center",
				"go to viridian pokemon center, fleeing wild battles",
			},
		},
		{
			name: "whole party with a potion in the bag: no use-item — a round that changes nothing",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 20, MaxHP: 20}},
				Bag:    []agent.Item{{Name: "potion", Quantity: 3}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
			},
			mustNot: []string{"use", "heal"},
		},
		{
			name: "hurt party, empty bag: no use-item to offer",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 4, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
			},
			mustNot: []string{"use"},
		},
		{
			name: "poisoned mon with an antidote: the status cure joins, though the HP is whole",
			obs: agent.Observation{
				Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("rhydon"), Level: 6, HP: 20, MaxHP: 20, Status: "poisoned"}},
				Bag:    []agent.Item{{Name: "antidote", Quantity: 1}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x00)
				k.SawMap(0x0c)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"use an ANTIDOTE on party slot 0",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
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
				"deliver oak's parcel",
				"heal the party",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to route 1",
				"go to route 1, fleeing wild battles",
				"go to viridian city",
				"go to viridian city, fleeing wild battles",
				"go to viridian pokemon center",
				"go to viridian pokemon center, fleeing wild battles",
			},
			mustNot: []string{"catch", "train", "gym"},
		},
		{
			name: "at the gym underlevelled: the gym is STILL offered — Offer never filters on wisdom",
			obs: agent.Observation{
				Map: 0x36, MapName: "PEWTER_GYM", X: 5, Y: 3, PartyCount: 1,
				Party:  []agent.PartyMon{{Species: agent.SpeciesID("nidoking"), Level: 5, HP: 1, MaxHP: 20}},
				Events: []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x36)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"beat the gym leader here",
				"go to pewter gym",
				"go to pewter gym, fleeing wild battles",
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
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to route 1",
				"go to route 1, fleeing wild battles",
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
				k.SawDialogue([]string{"An old man said: Pewter City lies to the east."}, "PALLET_TOWN", 5, 6)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"go to pallet town",
				"go to pallet town, fleeing wild battles",
				"go to pewter city",
				"go to pewter city, fleeing wild battles",
				"go to route 1",
				"go to route 1, fleeing wild battles",
			},
		},
		{
			name: "inside the viridian mart: one buy per item the shelf actually stocks, no POTION",
			obs: agent.Observation{
				Map: 0x2a, MapName: "VIRIDIAN_MART", X: 3, Y: 6, PartyCount: 1, Money: 10000,
				MartStock: []string{"pokeball", "antidote", "parlyz heal", "burn heal"},
				Events:    []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x2a)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"buy 10 POKEBALL",
				"go to viridian mart",
				"go to viridian mart, fleeing wild battles",
			},
			mustNot: []string{"POTION"},
		},
		{
			name: "inside a mart whose shelf is unreadable: no buy objective at all",
			obs: agent.Observation{
				Map: 0x2a, MapName: "VIRIDIAN_MART", X: 3, Y: 6, PartyCount: 1,
				MartStock: nil,
				Events:    []string{"BattledRivalInOaksLab"},
			},
			known: func() *agent.Knowledge {
				k := agent.NewKnowledge(adj)
				k.SawMap(0x2a)
				return k
			},
			want: []string{
				"deliver oak's parcel",
				"go to viridian mart",
				"go to viridian mart, fleeing wild battles",
			},
			mustNot: []string{"buy"},
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
				"heal the party",
				"go to viridian pokemon center",
				"go to viridian pokemon center, fleeing wild battles",
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

func TestKnowledgeDialogueMentions(t *testing.T) {
	k := agent.NewKnowledge(nil)
	k.SawDialogue([]string{
		"The sign reads: Route 22. Beware the rival.",
		"Nothing here names a place at all.",
	}, "ROUTE_22", 5, 5)
	if !k.Places["route 22"] {
		t.Errorf("dialogue named route 22; Knowledge does not know it")
	}
	if k.Places["route 2"] {
		t.Errorf("'Route 22' must not count as a mention of 'route 2'")
	}
	if len(k.Places) != 1 {
		t.Errorf("Knowledge.Places = %v, want exactly {route 22}", k.Places)
	}

	k.SawDialogue([]string{"In Pallet Town, Oak waits."}, "PALLET_TOWN", 5, 6)
	if !k.Places["pallet town"] {
		t.Errorf("dialogue named Pallet Town; Knowledge does not know it")
	}
}

func TestKnowledgeHarvestsStatedRequirements(t *testing.T) {
	k := agent.NewKnowledge(nil)
	guard := []string{
		"You can pass here\nonly if you have\nthe CASCADEBADGE!",
		"You don't have the\nCASCADEBADGE yet!",
		"You have to have\nit to get to\nPOKéMON LEAGUE!",
	}
	chatter := []string{
		"I'm raising #MON too!",
		"Time is money...",
		"Are you going to VIRIDIAN FOREST?\nBe careful, it's\na natural maze!",
	}

	k.SawDialogue(chatter, "ROUTE_23", 4, 57)
	if len(k.Requirements) != 0 {
		t.Fatalf("ordinary chatter was harvested: %v", k.Requirements)
	}

	k.SawDialogue(guard, "ROUTE_23", 4, 57)
	want := []agent.Requirement{
		{Text: "You don't have the\nCASCADEBADGE yet!", Place: "ROUTE_23", X: 4, Y: 57, Times: 1},
		{Text: "You can pass here\nonly if you have\nthe CASCADEBADGE!", Place: "ROUTE_23", X: 4, Y: 57, Times: 1},
	}
	if !reflect.DeepEqual(k.Requirements, want) {
		t.Fatalf("Requirements = %v, want %v", k.Requirements, want)
	}

	k.SawDialogue(guard, "ROUTE_23", 4, 57)
	want[0].Times, want[1].Times = 2, 2
	if !reflect.DeepEqual(k.Requirements, want) {
		t.Fatalf("re-heard lines duplicated or uncounted: %v", k.Requirements)
	}
}

func TestKnowledgeRequirementShapesAndCap(t *testing.T) {
	k := agent.NewKnowledge(nil)
	k.HeardRequirement("You can't go\nthrough here!", "VIRIDIAN_CITY", 19, 10)
	if len(k.Requirements) != 1 || !strings.Contains(k.Requirements[0].Text, "can't go") {
		t.Fatalf("the 'can't go through' shape was not caught: %v", k.Requirements)
	}

	k2 := agent.NewKnowledge(nil)
	for i := 0; i < 12; i++ {
		k2.HeardRequirement("You need key "+string(rune('a'+i))+" to pass.", "ROUTE_23", 4, 57)
	}
	if len(k2.Requirements) != 8 {
		t.Fatalf("cap not enforced: %d lines kept, want 8: %v", len(k2.Requirements), k2.Requirements)
	}
	if !strings.Contains(k2.Requirements[0].Text, "key l") {
		t.Errorf("newest line not first: %v", k2.Requirements)
	}
	if !strings.Contains(k2.Requirements[len(k2.Requirements)-1].Text, "key e") {
		t.Errorf("oldest surviving line not last: %v", k2.Requirements)
	}
}

func TestOfferJourneyVariants(t *testing.T) {
	adj := map[uint8][]uint8{0x0c: {0x00, 0x01, 0x29}}
	obs := agent.Observation{
		Map: 0x0c, MapName: "ROUTE_1", X: 5, Y: 14, PartyCount: 1,
		Party: []agent.PartyMon{{Level: 6, HP: 4, MaxHP: 20}},
	}
	known := agent.NewKnowledge(adj)
	known.SawMap(0x0c)
	known.SawMap(0x29)

	got := agent.Offer(obs, known)
	sameJourney := func(a, b agent.Objective) bool {
		return a.Kind == b.Kind && a.Place == b.Place && a.Flee == b.Flee
	}
	for i, o := range got {
		isJourney := o.Kind == agent.KindGoTo || (o.Kind == agent.KindHeal && o.Place != "")
		if !isJourney {
			continue
		}
		if !o.Flee {
			want := agent.Objective{Kind: o.Kind, Place: o.Place, Flee: true}
			if i+1 >= len(got) || !sameJourney(got[i+1], want) {
				t.Fatalf("journey %q at %d has no fleeing variant beside it:\n%v", o, i, got)
			}
		} else {
			plain := agent.Objective{Kind: o.Kind, Place: o.Place}
			if i == 0 || !sameJourney(got[i-1], plain) {
				t.Fatalf("fleeing variant %q at %d does not follow its plain one:\n%v", o, i, got)
			}
		}
	}
}

func TestOfferMapObjects(t *testing.T) {
	obs := agent.Observation{
		Map: 0x99, X: 5, Y: 6, PartyCount: 1,
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
		Party: []agent.PartyMon{{Level: 11, HP: 20, MaxHP: 20}},
	}
	trainTarget := func(obs agent.Observation) (uint8, bool) {
		for _, objective := range agent.Offer(obs, known) {
			if objective.Kind == agent.KindTrain {
				return objective.Level, true
			}
		}
		return 0, false
	}

	for _, tc := range []struct{ lead, want uint8 }{{5, 7}, {11, 13}, {12, 14}, {40, 42}} {
		obs.Party = []agent.PartyMon{{Level: tc.lead, HP: 20, MaxHP: 20}}
		got, ok := trainTarget(obs)
		if !ok {
			t.Fatalf("lead level %d: training not offered", tc.lead)
		}
		if got != tc.want {
			t.Errorf("lead level %d: offered target %d, want %d", tc.lead, got, tc.want)
		}
	}

	obs.Party = []agent.PartyMon{{Level: 100, HP: 20, MaxHP: 20}}
	if got, ok := trainTarget(obs); ok {
		t.Errorf("training offered at level %d for a level-100 lead; there is no rung above it", got)
	}
	obs.Party, obs.PartyCount = nil, 0
	if got, ok := trainTarget(obs); ok {
		t.Errorf("training offered to level %d with an empty party", got)
	}
}

func TestOfferWithholdsTrainBelowRetreatLine(t *testing.T) {
	known := agent.NewKnowledge(nil)
	mk := func(hp, maxHP uint16) agent.Observation {
		return agent.Observation{
			Map: 0x0c, MapName: "ROUTE_1", HasGrass: true, PartyCount: 1,
			Party: []agent.PartyMon{{Level: 5, HP: hp, MaxHP: maxHP}},
		}
	}
	menu := func(obs agent.Observation) (train, others int) {
		for _, o := range agent.Offer(obs, known) {
			if o.Kind == agent.KindTrain {
				train++
			} else {
				others++
			}
		}
		return
	}

	if train, _ := menu(mk(20, 20)); train != 1 {
		t.Fatalf("healthy lead on grass: train offered %d times, want 1", train)
	}
	if train, _ := menu(mk(10, 20)); train != 1 {
		t.Fatalf("lead at exactly the line: train offered %d times, want 1", train)
	}
	if train, _ := menu(mk(9, 20)); train != 0 {
		t.Errorf("lead below the line: train offered, want withheld")
	}
	if _, others := menu(mk(9, 20)); others == 0 {
		t.Errorf("lead below the line: the whole menu is empty, want the other objectives still offered")
	}

	obs := mk(9, 20)
	obs.HasGrass = false
	if train, _ := menu(obs); train != 0 {
		t.Errorf("no grass: train offered, want withheld")
	}
	if train, _ := menu(mk(0, 20)); train != 0 {
		t.Errorf("fainted lead: train offered, want withheld")
	}
}

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

	pewter.Party = []agent.PartyMon{{Level: 5, HP: 2, MaxHP: 20}}
	if !gymObjective(pewter) {
		t.Error("gym withheld from an underlevelled party; Offer must not filter on wisdom")
	}
}

func TestOfferWithholdsTrainBelowTheRetreatLine(t *testing.T) {
	known := agent.NewKnowledge(map[uint8][]uint8{})
	offersTrain := func(hp, maxHP uint16) bool {
		obs := agent.Observation{
			Map: 0x0c, HasGrass: true, PartyCount: 1,
			Party: []agent.PartyMon{{Level: 11, HP: hp, MaxHP: maxHP}},
		}
		for _, o := range agent.Offer(obs, known) {
			if o.Kind == agent.KindTrain {
				return true
			}
		}
		return false
	}
	for _, tc := range []struct {
		name      string
		hp, maxHP uint16
		want      bool
	}{
		{"healthy", 30, 30, true},
		{"exactly half is not below the line", 15, 30, true},
		{"one below half", 14, 30, false},
		{"fainted lead", 0, 30, false},
	} {
		if got := offersTrain(tc.hp, tc.maxHP); got != tc.want {
			t.Errorf("%s (%d/%d): offered train = %v, want %v", tc.name, tc.hp, tc.maxHP, got, tc.want)
		}
	}
}
