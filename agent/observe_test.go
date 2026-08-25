package agent_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/red/state"
)

// TestObserveFreshBoot observes a freshly booted, controllable overworld and
// checks the basics: the game is controllable, the party is empty, and Map
// matches what state decodes independently.
func TestObserveFreshBoot(t *testing.T) {
	e := loadFixture(t)

	obs := agent.Observe(e)

	if !obs.Controllable {
		t.Fatalf("Controllable = false, want true at a fresh boot")
	}
	if obs.PartyCount != 0 {
		t.Fatalf("PartyCount = %d, want 0", obs.PartyCount)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	want := state.DecodePlayer(&mem).MapID
	if obs.Map != want {
		t.Fatalf("Map = %#04x, want %#04x (what state decodes)", obs.Map, want)
	}
}

// TestObserveAfterStarter runs the KindStarter objective and checks the
// observation: one party mon, and the battled-rival event listed by name.
func TestObserveAfterStarter(t *testing.T) {
	e := loadFixture(t)

	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	obs := agent.Observe(e)

	if obs.PartyCount != 1 {
		t.Fatalf("PartyCount = %d, want 1", obs.PartyCount)
	}
	if len(obs.Party) != 1 {
		t.Fatalf("len(Party) = %d, want 1", len(obs.Party))
	}
	wantEvent := state.EventBattledRivalInOaksLab.String()
	for _, name := range obs.Events {
		if name == wantEvent {
			return
		}
	}
	t.Fatalf("Events = %v, want it to contain %q", obs.Events, wantEvent)
}

// TestObserveJSONRoundTrip marshals a fully populated Observation and back,
// and asserts it is unchanged. That is what catches an unserializable field:
// anything encoding/json cannot carry across would show up as a difference.
func TestObserveJSONRoundTrip(t *testing.T) {
	obs := agent.Observation{
		Map:          0x00,
		MapName:      "",
		X:            5,
		Y:            6,
		Facing:       "up",
		Controllable: true,
		InBattle:     false,
		PartyCount:   1,
		Party: []agent.PartyMon{
			{Species: 7, Level: 5, HP: 20, MaxHP: 20},
		},
		Badges: []string{},
		Money:  5000,
		Events: []string{"BattledRivalInOaksLab"},
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
