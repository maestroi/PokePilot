package agent_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestMapObjectsFromROM checks the map-header object classification against
// the real ROM. No emulator: MapObjects is pure ROM data, so this runs in
// milliseconds and cannot flake.
func TestMapObjectsFromROM(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}

	// Viridian Forest (0x33): three items, including a POKE_BALL at (1,31).
	forest := agent.MapObjects(romData, 0x33)
	var items []agent.MapObject
	for _, o := range forest {
		if o.Kind == "item" {
			items = append(items, o)
		}
	}
	if len(items) != 3 {
		t.Fatalf("viridian forest items = %+v, want three", items)
	}
	found := false
	for _, it := range items {
		if it.X == 1 && it.Y == 31 && it.Item == "pokeball" {
			found = true
		}
	}
	if !found {
		t.Errorf("viridian forest items = %+v, want a pokeball at (1,31)", items)
	}

	// Pewter Gym (0x36): one person at (7,10) and two trainers. The trainers
	// are REPORTED here; the offered-list side of the split is in
	// TestOfferMapObjects.
	gym := agent.MapObjects(romData, 0x36)
	var persons, trainers int
	for _, o := range gym {
		switch o.Kind {
		case "person":
			persons++
			if o.X != 7 || o.Y != 10 {
				t.Errorf("pewter gym person at (%d,%d), want (7,10)", o.X, o.Y)
			}
		case "trainer":
			trainers++
		}
	}
	if persons != 1 {
		t.Errorf("pewter gym persons = %d, want 1 (%+v)", persons, gym)
	}
	if trainers != 2 {
		t.Errorf("pewter gym trainers = %d, want 2 (%+v)", trainers, gym)
	}
}

// TestObserveFreshBoot observes a freshly booted, controllable overworld and
// checks the basics: the game is controllable, the party is empty, and Map
// matches what state decodes independently.
func TestObserveFreshBoot(t *testing.T) {
	e := loadFixture(t)

	obs := agent.Observe(e, e.ROM())

	if !obs.Controllable {
		t.Fatalf("Controllable = false, want true at a fresh boot")
	}
	if obs.PartyCount != 0 {
		t.Fatalf("PartyCount = %d, want 0", obs.PartyCount)
	}
	if obs.BlackedOut {
		t.Fatalf("BlackedOut = true at a fresh boot, want false")
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	want := state.DecodePlayer(&mem).MapID
	if obs.Map != want {
		t.Fatalf("Map = %#04x, want %#04x (what state decodes)", obs.Map, want)
	}
	// A fresh boot lands on Reds House 2F (0x26). The planner sees the
	// name, not just the integer: an unnamed map would be "".
	if obs.MapName != "REDS_HOUSE_2F" {
		t.Fatalf("MapName = %q, want %q for map %#04x", obs.MapName, "REDS_HOUSE_2F", obs.Map)
	}
}

// TestObserveAfterStarter runs the KindStarter objective and checks the
// observation: one party mon, and the battled-rival event listed by name.
func TestObserveAfterStarter(t *testing.T) {
	e := loadFixture(t)

	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	obs := agent.Observe(e, e.ROM())

	if obs.PartyCount != 1 {
		t.Fatalf("PartyCount = %d, want 1", obs.PartyCount)
	}
	// The lead's moves are decoded from the ROM's move table: power and
	// type, no invented names. Charmander's level-1 learnset (pokered
	// base_stats) is SCRATCH + GROWL.
	if len(obs.LeadMoves) != 2 {
		t.Fatalf("LeadMoves = %+v, want the starter's two moves", obs.LeadMoves)
	}
	if obs.LeadMoves[0] != (agent.Move{Power: 40, Type: "normal"}) || obs.LeadMoves[1] != (agent.Move{Power: 0, Type: "normal"}) {
		t.Errorf("LeadMoves = %+v, want SCRATCH (power 40 normal) and GROWL (power 0 normal)", obs.LeadMoves)
	}
	// The bag is decoded too; at this point in the story Oak has not given
	// the pokeballs yet, so it is empty. (A populated bag round-trips in
	// TestObserveJSONRoundTrip.)
	if len(obs.Bag) != 0 {
		t.Errorf("Bag = %+v, want empty at this point in the story", obs.Bag)
	}
	// Observe alone decodes no run memory: dialogue and history are Run's.
	if len(obs.RecentDialogue) != 0 || len(obs.History) != 0 {
		t.Errorf("standalone Observe carries run memory: dialogue=%v history=%v", obs.RecentDialogue, obs.History)
	}
	if len(obs.Party) != 1 {
		t.Fatalf("len(Party) = %d, want 1", len(obs.Party))
	}
	wantEvent := state.EventBattledRivalInOaksLab.String()
	found := false
	for _, name := range obs.Events {
		if name == wantEvent {
			found = true
		}
	}
	if !found {
		t.Fatalf("Events = %v, want it to contain %q", obs.Events, wantEvent)
	}
	// The ROM map header permanently contains all three starter balls, but
	// the game hides the player's and rival's choices through the current
	// map's toggleable-object flags. Observe must expose the remaining ball
	// only, otherwise Offer manufactures a stale talk objective at (6,3).
	balls := map[[2]uint8]bool{}
	for _, object := range obs.MapObjects {
		if object.Y == 3 && object.X >= 6 && object.X <= 8 {
			balls[[2]uint8{object.X, object.Y}] = true
		}
	}
	if balls[[2]uint8{6, 3}] || balls[[2]uint8{7, 3}] {
		t.Fatalf("MapObjects still exposes hidden starter balls: %v", balls)
	}
	if !balls[[2]uint8{8, 3}] {
		t.Fatalf("MapObjects omitted the remaining starter ball: %v", balls)
	}
	// The starter comes out of the story healthy: the status field is
	// populated ("" for a healthy mon), not missing.
	if obs.Party[0].Status != "" {
		t.Errorf("Party[0].Status = %q, want \"\" (the starter is healthy)", obs.Party[0].Status)
	}
	if obs.BlackedOut {
		t.Errorf("BlackedOut = true after the starter, want false")
	}
}

// TestObserveJSONRoundTrip marshals a fully populated Observation and back,
// and asserts it is unchanged. That is what catches an unserializable field:
// anything encoding/json cannot carry across would show up as a difference.
func TestObserveJSONRoundTrip(t *testing.T) {
	obs := agent.Observation{
		Map:          0x00,
		MapName:      "PALLET_TOWN",
		X:            5,
		Y:            6,
		Facing:       "up",
		Controllable: true,
		InBattle:     false,
		PartyCount:   1,
		Party: []agent.PartyMon{
			{Species: 7, Level: 5, HP: 20, MaxHP: 20, Status: "poisoned"},
		},
		Badges:     []string{},
		Money:      5000,
		Events:     []string{"BattledRivalInOaksLab"},
		BlackedOut: true,
		LeadMoves: []agent.Move{
			{Power: 35, Type: "normal"},
			{Power: 0, Type: "normal"},
		},
		Bag:            []agent.Item{{Name: "pokeball", Quantity: 5}},
		RecentDialogue: []string{"OAK: Oh! You're awake!"},
		History:        []agent.RoundRecord{{Objective: "take the charmander starter", Outcome: "done"}},
	}

	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back agent.Observation
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(obs, back) {
		t.Fatalf("round trip changed the observation:\n in:  %+v\n out: %+v\n json: %s", obs, back, b)
	}
}

// TestObserveAndOfferNameTheMapsOwnWildSpecies is the whole chain the
// per-map catch rests on, driven for real: stand on Route 1, and the
// observation must name the species Route1WildMons holds — PIDGEY and
// RATTATA, and no CATERPIE, which is what the fixed menu used to offer
// here — with one catch objective per species.
func TestObserveAndOfferNameTheMapsOwnWildSpecies(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; not part of the -short gate")
	}
	e := fixture.Load(t, "post_starter")
	dest, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("Place(route 1) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 20); err != nil {
		t.Fatalf("travel to route 1: %v", err)
	}

	obs := agent.Observe(e, e.ROM())
	var names []string
	for _, w := range obs.WildGrass {
		names = append(names, w.Name)
		if w.MinLevel == 0 || w.MaxLevel < w.MinLevel || w.Slots < 1 {
			t.Errorf("%s: levels %d-%d in %d slots is not a usable band", w.Name, w.MinLevel, w.MaxLevel, w.Slots)
		}
	}
	if !reflect.DeepEqual(names, []string{"pidgey", "rattata"}) {
		t.Fatalf("Observe().WildGrass = %+v, want pidgey and rattata", obs.WildGrass)
	}

	// Balls in the bag, so the hunt's other precondition holds too.
	obs.Bag = append(obs.Bag, agent.Item{Name: "pokeball", Quantity: 5})
	var catches []string
	for _, o := range agent.Offer(obs, agent.NewKnowledge(nil)) {
		if o.Kind == agent.KindCatch {
			catches = append(catches, o.String())
		}
	}
	want := []string{"catch a PIDGEY here", "catch a RATTATA here"}
	if !reflect.DeepEqual(catches, want) {
		t.Fatalf("catch objectives = %v, want %v", catches, want)
	}
}
