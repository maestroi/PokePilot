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
	// RespawnPlace is the map a blackout sends the player to, decoded from
	// wLastBlackoutMap. It is NOT "the nearest town": only healing at a
	// Pokemon Center writes it (SetLastBlackoutMap is called from one place,
	// DisplayPokemonCenterDialogue_, on YES to the nurse), and its zeroed
	// new-game value is PALLET_TOWN. So a run that walks to Pewter and
	// loses to Brock without ever using a Center wakes up in Pallet Town
	// with half its money — the whole journey to redo, and the shop money
	// gone. Reported so the planner can see what a loss would cost before
	// it picks the fight, and see a Center as the checkpoint it is.
	RespawnPlace string
	Events       []string // names of the story events currently set
	// BlackedOut says a blackout just happened: the party was wiped out
	// (a lost battle, or poison fainted the last mon out of it) and the
	// game is mid-respawn. The respawn fully heals the party, HALVES the
	// money (ResetStatusAndHalveMoneyOnBlackout, home/overworld.asm:767)
	// and lands the player on RespawnPlace, so the party is healthy
	// again by the time the player is controllable. The bit this reads
	// (wStatusFlags4 bit 5) is cleared on every map entry, so it is live
	// only while that transition is in flight; Run carries the fact across
	// the round that follows a blackout failure: when Execute reports
	// skill.ErrBlackedOut, Run sets this on the observation the next plan
	// sees, because by then the respawn map entry has already cleared the
	// bit.
	BlackedOut bool

	// LeadMoves are the lead's moves, decoded from the ROM's move table and
	// paired with the live current PP from party RAM. Empty while the party
	// is empty. No move names: the ROM's table stores no name strings, and
	// inventing them would be data the player cannot see.
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
	// It SCROLLS: only the last historyCap rounds are here.
	History []RoundRecord
	// Failures is the tally History scrolls past: every objective the run
	// has tried and failed, with how many times and the last error, kept
	// across the whole run by Knowledge and set here by Run. Most-failed
	// first. An objective that later succeeds leaves the list. It is what
	// tells a planner the difference between an idea it has not tried and
	// one it has walked into eight times.
	Failures []Failure

	// Round is which round of the run this is, 1-based, and RoundsLeft is
	// how many rounds the budget still allows INCLUDING this one — 1 means
	// this is the last objective the run will ever pick. Set by Run, like
	// History: a budget is run bookkeeping, not game state.
	//
	// The prompt has always said the run has a limited number of rounds
	// while never saying how many were left, so the model could not ration:
	// it spent rounds on a round trip to a place it had just come from with
	// the same weight whether the budget was fresh or nearly gone. Naming
	// the remainder is what makes "is this worth a round?" answerable.
	Round      int
	RoundsLeft int

	// Intent is the sentence the planner most recently attached to its
	// choice — what that choice was in service of. Set by Run from the
	// planner's own words, never written, edited or summarised by Run: a
	// model that keeps re-choosing an objective it has chosen before sees
	// here why it chose it, at temperature 0 where nothing else changes.
	// Empty until the planner says one.
	Intent string
	// IntentAge is how many rounds the carried intent has gone unchanged:
	// 0 on the round after it was (re)set, counting up while the planner
	// leaves it alone. Age is what lets a model notice it has been chasing
	// the same thing for thirty rounds.
	IntentAge int

	// WildGrass names the species this map's tall grass can actually roll,
	// with the level band each appears at and how many of the ten encounter
	// slots it holds (rarity, in the only form the ROM states it). Decoded
	// from the map's own wild data (skill.WildGrass), so it is the game's
	// answer to "what is catchable here", not a list we chose. Empty on a
	// map with no grass encounters.
	WildGrass []WildSpecies

	// HasGrass says the current map has walkable tall grass — the
	// precondition for training. A map feature decoded from the ROM's
	// tileset table (skill.HasGrass), like LeadMoves: geometry the player
	// can see on screen, reported so a planner does not have to guess it.
	HasGrass bool

	// MartStock is the item names this map's mart clerk actually stocks,
	// in shelf order, decoded from the clerk's own text script
	// (rom.MartItems) — the game's answer to "what can I buy here", not a
	// list we chose. Empty on a map whose shelf cannot be read: Offer
	// treats that as "offer nothing", never as a reason to guess — an
	// objective that cannot succeed is worse than an absent one.
	MartStock []string

	// MapObjects are the current map's objects read from the ROM map header,
	// NOT from sprite RAM: sprite RAM is screen-local (MEASURED: zero sprites
	// visible standing seven tiles from an NPC), so it cannot tell a planner
	// that a person worth talking to exists across the map.
	MapObjects []MapObject

	// Requirements are the walls the game has stated, kept across rounds by
	// Knowledge and set here by Run (like RecentDialogue): a wall stated
	// once must still be visible ten rounds later, or the run keeps walking
	// into the same wall to remember it. Each carries the game's exact
	// words — nothing is parsed out of them — plus where it was heard and
	// how many times, so a run can see it is repeating itself. Newest
	// first. Empty until the game says one.
	Requirements []Requirement
}

// MapObject is one object of the current map in the form a planner may see
// it: where it stands and what kind of thing it is. Item is the item name
// for Kind "item" only; an unknown id says so rather than vanishing.
type MapObject struct {
	X, Y uint8
	Kind string // "person" | "item" | "trainer"
	Item string // item name, "item" only; "" otherwise
}

// Move is one move the lead knows, in the form a planner may see it. PP is
// the current remaining PP from party RAM, with PP-Up count bits stripped by
// state.DecodeParty. Zero therefore means genuinely exhausted.
type Move struct {
	Power uint8  // 0 = deals no damage
	Type  string // "normal", "fire", ...; "" when the byte is not a known type
	PP    uint8
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

// WildSpecies is one species the current map's grass can roll. Named, not
// indexed: an index is a number the planner cannot reason about, and the
// name is the same vocabulary it uses to ask for a catch.
type WildSpecies struct {
	Name     string
	MinLevel uint8
	MaxLevel uint8
	Slots    int // of the map's ten encounter slots
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
	state.EventBeatChampionRival,
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
		RespawnPlace:   state.MapName(mem.U8(sym.LastBlackoutMap)),
		Party:          make([]PartyMon, len(gs.Party.Mons)),
		Badges:         []string{},
		Events:         []string{},
		LeadMoves:      []Move{},
		Bag:            []Item{},
		RecentDialogue: []string{},
		History:        []RoundRecord{},
		Failures:       []Failure{},
		Requirements:   []Requirement{},
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
		lead := gs.Party.Mons[0]
		for slot, id := range lead.Moves {
			if id == 0 {
				continue
			}
			mv, err := rom.LookupMove(romData, id)
			if err != nil {
				// An id not in the table is not a move; never invent one.
				continue
			}
			obs.LeadMoves = append(obs.LeadMoves, Move{Power: mv.Power, Type: moveTypeNames[mv.Type], PP: lead.PP[slot]})
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
	obs.WildGrass = []WildSpecies{}
	if wild, err := skill.WildGrass(romData, obs.Map); err == nil {
		for _, w := range wild {
			name, ok := SpeciesName(w.ID)
			if !ok {
				continue // an index outside the roster; report nothing rather than a number
			}
			obs.WildGrass = append(obs.WildGrass, WildSpecies{
				Name: name, MinLevel: w.MinLevel, MaxLevel: w.MaxLevel, Slots: w.Slots,
			})
		}
	}
	obs.MartStock = []string{}
	if items, err := rom.MartItems(romData, obs.Map); err == nil {
		for _, id := range items {
			if name, ok := ItemName(id); ok {
				obs.MartStock = append(obs.MartStock, name)
			}
		}
	}
	objects := MapObjects(romData, obs.Map)
	hidden := state.HiddenObjectIDs(&mem)
	obs.MapObjects = make([]MapObject, 0, len(objects))
	for i, object := range objects {
		// Map object constants are 1-based indexes in header order, which is
		// also what wToggleableObjectList stores for the current map.
		if !hidden[uint8(i+1)] {
			obs.MapObjects = append(obs.MapObjects, object)
		}
	}
	return obs
}

// MapObjects decodes one map's objects from the ROM map header — static,
// map-wide data whose coordinates are already de-biased. It is deliberately
// not the sprite-RAM decoder: that one's IMAGEINDEX == $ff filter is
// screen-local, so standing seven tiles from an NPC it returns zero sprites
// (measured on the viridian_city fixture), and a planner offered only
// visible sprites could never decide to cross a map to reach someone. Live
// sprite RAM stays for blockers; this is the offering's data source.
func MapObjects(romData []byte, mapID uint8) []MapObject {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return []MapObject{}
	}
	out := make([]MapObject, 0, len(h.Objects))
	for _, o := range h.Objects {
		mo := MapObject{X: o.X, Y: o.Y}
		switch {
		case o.TextID&0x80 != 0:
			mo.Kind = "item"
			if name, ok := ItemName(o.ItemID); ok {
				mo.Item = name
			} else {
				mo.Item = fmt.Sprintf("item %d", o.ItemID)
			}
		case o.TextID&0x40 != 0:
			mo.Kind = "trainer"
		default:
			mo.Kind = "person"
		}
		out = append(out, mo)
	}
	return out
}
