package agent

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// Observation is the whole view a planner gets of the game. It is a
// decoded summary, never raw memory: no addresses, no frame data, no
// pixels. If something is not in here, a planner cannot know it.
//
// The field names are the JSON names S4-5 sends to a model. A field that
// renames itself between runs silently changes the prompt, so treat them
// as a stable contract.
type Observation struct {
	Map          uint8
	MapName      string // "" when unknown; never fail on an unnamed map
	X, Y         uint8
	Facing       string // "up" / "down" / "left" / "right"
	Controllable bool
	InBattle     bool
	PartyCount   int
	Party        []PartyMon // Species, Level, HP, MaxHP
	Badges       []string
	Money        uint32
	Events       []string // names of the story events currently set
}

// PartyMon is one party member in the form a planner is allowed to see it.
type PartyMon struct {
	Species uint8
	Level   uint8
	HP      uint16
	MaxHP   uint16
}

// knownEvents is the full set of story events red/state decodes today, in
// declaration order. Observe lists the subset currently set, by name.
var knownEvents = []state.Event{
	state.EventFollowedOakIntoLab,
	state.EventOakAskedToChooseMon,
	state.EventGotStarter,
	state.EventBattledRivalInOaksLab,
	state.EventGotPokeballsFromOak,
	state.EventGotPokedex,
	state.EventOakAppearedInPallet,
}

// Observe decodes the current Observation from the emulator.
func Observe(m *emu.Emu) Observation {
	var mem state.Mem
	gs := state.Read(m, &mem)

	obs := Observation{
		Map:          gs.Player.MapID,
		X:            gs.Player.X,
		Y:            gs.Player.Y,
		Facing:       gs.Player.Facing.String(),
		Controllable: state.Controllable(&mem),
		InBattle:     gs.Battle != nil,
		PartyCount:   int(gs.Party.Count),
		Money:        gs.Inventory.Money,
		Party:        make([]PartyMon, len(gs.Party.Mons)),
		Badges:       []string{},
		Events:       []string{},
	}
	for i, mon := range gs.Party.Mons {
		obs.Party[i] = PartyMon{Species: mon.Species, Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP}
	}
	for b := state.BadgeBoulder; b <= state.BadgeEarth; b++ {
		if gs.Progress.Has(b) {
			obs.Badges = append(obs.Badges, b.String())
		}
	}
	for _, e := range knownEvents {
		if state.HasEvent(&mem, e) {
			obs.Events = append(obs.Events, e.String())
		}
	}
	return obs
}
