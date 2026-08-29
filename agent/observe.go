package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
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
	Party        []PartyMon // Species, Level, HP, MaxHP, Status
	Badges       []string
	Money        uint32
	Events       []string // names of the story events currently set
	// BlackedOut says a blackout just happened: the party was wiped out
	// (a lost battle, or poison fainted the last mon out of it) and the
	// game is mid-respawn. The respawn fully heals the party and lands the
	// player on the last town's fly-warp spot (a Route 1 blackout lands on
	// Pallet Town, which has no center at all), so the party is healthy
	// again by the time the player is controllable. The bit this reads
	// (wStatusFlags4 bit 5) is cleared on every map entry, so it is live
	// only while that transition is in flight; Run carries the fact across
	// the round that follows a blackout failure: when Execute reports
	// skill.ErrBlackedOut, Run sets this on the observation the next plan
	// sees, because by then the respawn map entry has already cleared the
	// bit.
	BlackedOut bool

	// LeadMoves are the lead's moves, decoded from the ROM's move table
	// (the same table skill/policy.go reads for power and effect). Empty
	// while the party is empty. No move names: the ROM's table stores no
	// name strings, and inventing them would be data the player cannot see.
	LeadMoves []Move
	// Bag is the decoded bag (state.DecodeInventory), named. Only entries
	// with a quantity; an unknown item ID says so rather than vanishing.
	Bag []Item
	// RecentDialogue is what the game has said recently, oldest first: NPC
	// lines are the game's own hints (the gym guide says what Brock uses).
	// Set by Run from its sample tape; Observe leaves it empty because a
	// single snapshot cannot see lines that opened and closed earlier.
	RecentDialogue []string
	// History is the last few objectives and how each turned out. Set by
	// Run, not decoded from RAM: outcomes are run memory, not game state.
	History []RoundRecord

	// HasGrass says the current map has walkable tall grass — the
	// precondition for training. A map feature decoded from the ROM's
	// tileset table (skill.HasGrass), like LeadMoves: geometry the player
	// can see on screen, reported so a planner does not have to guess it.
	HasGrass bool
}

// Move is one move the lead knows, in the form a planner may see it.
type Move struct {
	Power uint8  // 0 = deals no damage
	Type  string // "normal", "fire", ...; "" when the byte is not a known type
}

// Item is one bag entry in the form a planner may see it.
type Item struct {
	Name     string
	Quantity int
}

// RoundRecord is one line of run history: what was attempted and how it
// turned out. Outcome is "done" or "failed: <reason>".
type RoundRecord struct {
	Objective string
	Outcome   string
}

// moveTypeNames maps the ROM's move-table type byte to a name. The bytes
// are the TypeNames indexes of pokered/constants/type_constants.asm (the
// vendored decomp): 0-8 are the physical types, $14-$1A the special ones,
// and nothing in between exists. BIRD is shown as FLYING in game.
var moveTypeNames = map[uint8]string{
	0x00: "normal",   // NORMAL
	0x01: "fighting", // FIGHTING
	0x02: "flying",   // FLYING
	0x03: "poison",   // POISON
	0x04: "ground",   // GROUND
	0x05: "rock",     // ROCK
	0x06: "flying",   // BIRD
	0x07: "bug",      // BUG
	0x08: "ghost",    // GHOST
	0x14: "fire",     // FIRE
	0x15: "water",    // WATER
	0x16: "grass",    // GRASS
	0x17: "electric", // ELECTRIC
	0x18: "psychic",  // PSYCHIC_TYPE
	0x19: "ice",      // ICE
	0x1a: "dragon",   // DRAGON
}

// PartyMon is one party member in the form a planner is allowed to see it.
type PartyMon struct {
	Species uint8
	Level   uint8
	HP      uint16
	MaxHP   uint16
	// Status is the mon's status as a name ("poisoned", "asleep", ...),
	// never the raw byte: "" when healthy.
	Status string
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

// Observe decodes the current Observation from the emulator. romData is
// needed for the lead's moves, which live in the ROM's move table rather
// than RAM; Run has it and passes it through.
func Observe(m *emu.Emu, romData []byte) Observation {
	var mem state.Mem
	gs := state.Read(m, &mem)

	obs := Observation{
		Map:            gs.Player.MapID,
		MapName:        state.MapName(gs.Player.MapID),
		X:              gs.Player.X,
		Y:              gs.Player.Y,
		Facing:         gs.Player.Facing.String(),
		Controllable:   state.Controllable(&mem),
		InBattle:       gs.Battle != nil,
		PartyCount:     int(gs.Party.Count),
		Money:          gs.Inventory.Money,
		Party:          make([]PartyMon, len(gs.Party.Mons)),
		Badges:         []string{},
		Events:         []string{},
		LeadMoves:      []Move{},
		Bag:            []Item{},
		RecentDialogue: []string{},
		History:        []RoundRecord{},
	}
	for i, mon := range gs.Party.Mons {
		obs.Party[i] = PartyMon{Species: mon.Species, Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.StatusName()}
	}
	// wStatusFlags4 bit 5 (BIT_BATTLE_OVER_OR_BLACKOUT): set when a battle
	// ends and when the poison blackout box closes, cleared on every map
	// entry and inside HandleBlackOut before the respawn warp. Read here it
	// is live only while a blackout transition is in flight; the loop-set
	// value from the typed outcome covers the passes after it.
	obs.BlackedOut = mem.U8(sym.StatusFlags4)&(1<<5) != 0
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
	if len(gs.Party.Mons) > 0 {
		for _, id := range gs.Party.Mons[0].Moves {
			if id == 0 {
				continue
			}
			mv, err := rom.LookupMove(romData, id)
			if err != nil {
				// An id not in the table is not a move; never invent one.
				continue
			}
			obs.LeadMoves = append(obs.LeadMoves, Move{Power: mv.Power, Type: moveTypeNames[mv.Type]})
		}
	}
	for _, it := range gs.Inventory.Items {
		if it.ID == 0 || it.Quantity == 0 {
			continue
		}
		name, ok := ItemName(it.ID)
		if !ok {
			name = fmt.Sprintf("item %d", it.ID)
		}
		obs.Bag = append(obs.Bag, Item{Name: name, Quantity: int(it.Quantity)})
	}
	if grass, err := skill.HasGrass(romData, obs.Map); err == nil {
		obs.HasGrass = grass
	}
	return obs
}
