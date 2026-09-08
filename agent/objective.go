package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Kind is what an objective does.
type Kind uint8

const (
	KindGoTo               Kind = iota // walk to a named place
	KindTalk                           // face and talk to something at a coordinate
	KindStarter                        // complete the opening story and take a chosen starter
	KindErrand                         // deliver Oak's parcel (Viridian Mart -> Oak's lab)
	KindTrain                          // battle in grass until the lead reaches Level
	KindHeal                           // heal the party at a center; Place names one to travel to first
	KindGym                            // fight the leader of whichever gym the player is in
	KindCatch                          // hunt tall grass for a wanted species and catch it
	KindBuy                            // buy Item x Qty from the mart clerk
	KindPickup                         // pick up the item at a coordinate; the bag must rise
	KindUseItem                        // use one bag item on one party member, out in the field
	KindRocketHideout                  // clear the Celadon Rocket Hideout and obtain the Silph Scope
	KindPokemonTower                   // clear Pokemon Tower and obtain the Poke Flute
	KindFuchsiaProgression             // reach Fuchsia, beat Koga, and obtain Surf + Strength
)

// Objective is one unit of intent a planner can choose. Planner-facing game
// entities are semantic identifiers: no Red ROM species/item byte crosses this
// contract. A concrete adapter resolves those identifiers immediately before
// executing its game-specific skill.
type Objective struct {
	Kind    Kind
	Place   PlaceID   // KindGoTo/KindHeal/KindGym: semantic place identity; "" means heal where standing
	X, Y    uint8     // KindTalk, KindPickup: the tile to face
	Starter SpeciesID // KindStarter: semantic starter species
	Level   uint8     // KindTrain: the level the lead should reach
	Species SpeciesID // KindCatch: semantic species identity
	Item    ItemID    // KindBuy, KindPickup, KindUseItem: semantic item identity
	Slot    int       // KindUseItem: 0-based party slot
	Qty     int       // KindBuy: how many
	Flee    bool      // KindGoTo, KindHeal-with-Place: run from wild encounters
	Note    string    // human-readable, shown to a planner; never parsed
	// Intent is the sentence the planner attached to this choice: what it is
	// in service of. It is run memory, not an argument — Validate and
	// Execute ignore it, String() does not render it, and Run carries it
	// verbatim onto the next round's Observation (never edited or summarised).
	Intent string
}

// Validate checks game-independent shape/range invariants. Whether a semantic
// species/item/place exists in a concrete title is adapter-owned validation;
// keeping that lookup out of this method is what lets the same Objective shape
// cross a Crystal/Emerald adapter later without pretending to use Red indexes.
func (o Objective) Validate() error {
	switch o.Kind {
	case KindGoTo:
		if strings.TrimSpace(string(o.Place)) == "" {
			return fmt.Errorf("agent: %s: empty place id", o)
		}
	case KindStarter:
		if strings.TrimSpace(string(o.Starter)) == "" {
			return fmt.Errorf("agent: %s: empty starter species id", o)
		}
	case KindTrain:
		if o.Level < 1 || o.Level > 100 {
			return fmt.Errorf("agent: %s: level %d out of range 1..100", o, o.Level)
		}
	case KindCatch:
		if strings.TrimSpace(string(o.Species)) == "" {
			return fmt.Errorf("agent: %s: empty species id", o)
		}
	case KindPickup:
		if strings.TrimSpace(string(o.Item)) == "" {
			return fmt.Errorf("agent: %s: empty item id", o)
		}
	case KindUseItem:
		if strings.TrimSpace(string(o.Item)) == "" {
			return fmt.Errorf("agent: %s: empty item id", o)
		}
		if o.Slot < 0 || o.Slot > 5 {
			return fmt.Errorf("agent: %s: party slot %d out of range 0..5", o, o.Slot)
		}
	case KindBuy:
		if o.Qty < 1 || o.Qty > 99 {
			return fmt.Errorf("agent: %s: quantity %d out of range 1..99", o, o.Qty)
		}
		if strings.TrimSpace(string(o.Item)) == "" {
			return fmt.Errorf("agent: %s: empty item id", o)
		}
	}
	return nil
}

// String renders a short, plain, stable one-line description of the objective.
// Semantic identifiers are already the planner vocabulary, so rendering never
// needs to reverse-map a numeric game id.
func (o Objective) String() string {
	switch o.Kind {
	case KindGoTo:
		if o.Flee {
			return "go to " + string(o.Place) + ", fleeing wild battles"
		}
		return "go to " + string(o.Place)
	case KindTalk:
		return fmt.Sprintf("talk at (%d,%d)", o.X, o.Y)
	case KindStarter:
		return "take the " + strings.ToLower(string(o.Starter)) + " starter"
	case KindErrand:
		return "deliver oak's parcel"
	case KindTrain:
		return fmt.Sprintf("train the lead to level %d", o.Level)
	case KindHeal:
		if o.Place != "" {
			if o.Flee {
				return "heal the party at " + strings.ToUpper(string(o.Place)) + ", fleeing wild battles"
			}
			return "heal the party at " + strings.ToUpper(string(o.Place))
		}
		return "heal the party"
	case KindGym:
		return "beat the gym leader here"
	case KindCatch:
		return "catch a " + strings.ToUpper(string(o.Species)) + " here"
	case KindPickup:
		return fmt.Sprintf("pick up the %s at (%d,%d)", strings.ToUpper(string(o.Item)), o.X, o.Y)
	case KindUseItem:
		name := string(o.Item)
		return fmt.Sprintf("use %s %s on party slot %d", article(name), strings.ToUpper(name), o.Slot)
	case KindRocketHideout:
		return "clear the Rocket Hideout and get the SILPH SCOPE"
	case KindPokemonTower:
		return "clear Pokemon Tower and get the POKE FLUTE"
	case KindFuchsiaProgression:
		return "reach Fuchsia, beat Koga, and get HM03 SURF + HM04 STRENGTH"
	case KindBuy:
		return fmt.Sprintf("buy %d %s", o.Qty, strings.ToUpper(string(o.Item)))
	}
	return fmt.Sprintf("unknown kind %d", int(o.Kind))
}

// gymOutcomeErr renders a gym battle result as the objective's diagnostic.
func gymOutcomeErr(o Objective, outcome state.BattleResult) error {
	if outcome == state.ResultWon {
		return nil
	}
	return fmt.Errorf("agent: %s: lost to the gym leader (blacked out to the center)", o)
}

func catchOutcomeName(o skill.CatchOutcome) string {
	switch o {
	case skill.OutcomeCaught:
		return "caught"
	case skill.OutcomeFled:
		return "the target ran away"
	case skill.OutcomeOutOfBalls:
		return "out of balls"
	case skill.OutcomeTargetFainted:
		return "the target fainted"
	}
	return fmt.Sprintf("outcome %d", int(o))
}

func article(name string) string {
	if name == "" {
		return "a"
	}
	switch name[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

// The following tables are Pokémon Red adapter vocabulary: they translate
// stable semantic names to Red's internal ROM indexes. They remain here during
// the incremental adapter migration, but planner-facing Objective/Observation
// values never carry these numbers.
var speciesTable = map[string]uint8{
	"rhydon": 0x01, "kangaskhan": 0x02, "nidoran♂": 0x03, "clefairy": 0x04,
	"spearow": 0x05, "voltorb": 0x06, "nidoking": 0x07, "slowbro": 0x08,
	"ivysaur": 0x09, "exeggutor": 0x0A, "lickitung": 0x0B, "exeggcute": 0x0C,
	"grimer": 0x0D, "gengar": 0x0E, "nidoran♀": 0x0F, "nidoqueen": 0x10,
	"cubone": 0x11, "rhyhorn": 0x12, "lapras": 0x13, "arcanine": 0x14,
	"mew": 0x15, "gyarados": 0x16, "shellder": 0x17, "tentacool": 0x18,
	"gastly": 0x19, "scyther": 0x1A, "staryu": 0x1B, "blastoise": 0x1C,
	"pinsir": 0x1D, "tangela": 0x1E, "growlithe": 0x21, "onix": 0x22,
	"fearow": 0x23, "pidgey": 0x24, "slowpoke": 0x25, "kadabra": 0x26,
	"graveler": 0x27, "chansey": 0x28, "machoke": 0x29, "mr.mime": 0x2A,
	"hitmonlee": 0x2B, "hitmonchan": 0x2C, "arbok": 0x2D, "parasect": 0x2E,
	"psyduck": 0x2F, "drowzee": 0x30, "golem": 0x31, "magmar": 0x33,
	"electabuzz": 0x35, "magneton": 0x36, "koffing": 0x37, "mankey": 0x39,
	"seel": 0x3A, "diglett": 0x3B, "tauros": 0x3C, "farfetch'd": 0x40,
	"venonat": 0x41, "dragonite": 0x42, "doduo": 0x46, "poliwag": 0x47,
	"jynx": 0x48, "moltres": 0x49, "articuno": 0x4A, "zapdos": 0x4B,
	"ditto": 0x4C, "meowth": 0x4D, "krabby": 0x4E, "vulpix": 0x52,
	"ninetales": 0x53, "pikachu": 0x54, "raichu": 0x55, "dratini": 0x58,
	"dragonair": 0x59, "kabuto": 0x5A, "kabutops": 0x5B, "horsea": 0x5C,
	"seadra": 0x5D, "sandshrew": 0x60, "sandslash": 0x61, "omanyte": 0x62,
	"omastar": 0x63, "jigglypuff": 0x64, "wigglytuff": 0x65, "eevee": 0x66,
	"flareon": 0x67, "jolteon": 0x68, "vaporeon": 0x69, "machop": 0x6A,
	"zubat": 0x6B, "ekans": 0x6C, "paras": 0x6D, "poliwhirl": 0x6E,
	"poliwrath": 0x6F, "weedle": 0x70, "kakuna": 0x71, "beedrill": 0x72,
	"dodrio": 0x74, "primeape": 0x75, "dugtrio": 0x76, "venomoth": 0x77,
	"dewgong": 0x78, "caterpie": 0x7B, "metapod": 0x7C, "butterfree": 0x7D,
	"machamp": 0x7E, "golduck": 0x80, "hypno": 0x81, "golbat": 0x82,
	"mewtwo": 0x83, "snorlax": 0x84, "magikarp": 0x85, "muk": 0x88,
	"kingler": 0x8A, "cloyster": 0x8B, "electrode": 0x8D, "clefable": 0x8E,
	"weezing": 0x8F, "persian": 0x90, "marowak": 0x91, "haunter": 0x93,
	"abra": 0x94, "alakazam": 0x95, "pidgeotto": 0x96, "pidgeot": 0x97,
	"starmie": 0x98, "bulbasaur": 0x99, "venusaur": 0x9A, "tentacruel": 0x9B,
	"goldeen": 0x9D, "seaking": 0x9E, "ponyta": 0xA3, "rapidash": 0xA4,
	"rattata": 0xA5, "raticate": 0xA6, "nidorino": 0xA7, "nidorina": 0xA8,
	"geodude": 0xA9, "porygon": 0xAA, "aerodactyl": 0xAB, "magnemite": 0xAD,
	"charmander": 0xB0, "squirtle": 0xB1, "charmeleon": 0xB2, "wartortle": 0xB3,
	"charizard": 0xB4, "oddish": 0xB9, "gloom": 0xBA, "vileplume": 0xBB,
	"bellsprout": 0xBC, "weepinbell": 0xBD, "victreebel": 0xBE,
}

var itemTable = map[string]uint8{
	"pokeball": 0x04, "great ball": 0x03,
	"potion": 0x14, "super potion": 0x13, "hyper potion": 0x12, "max potion": 0x11,
	"antidote": 0x0B, "burn heal": 0x0C, "ice heal": 0x0D,
	"awakening": 0x0E, "parlyz heal": 0x0F, "full restore": 0x10,
	"repel": 0x1E, "escape rope": 0x1D,
	"silph scope": 0x48, "poke flute": 0x49,
	"hm03": 0xC6, "hm04": 0xC7,
}

var speciesByID = func() map[uint8]string {
	m := make(map[uint8]string, len(speciesTable))
	for name, id := range speciesTable {
		m[id] = name
	}
	return m
}()

var itemByID = func() map[uint8]string {
	m := make(map[uint8]string, len(itemTable))
	for name, id := range itemTable {
		m[id] = name
	}
	return m
}()

func SpeciesCount() int { return len(speciesTable) }

// SpeciesName/SpeciesByName are Red adapter helpers retained for code that is
// explicitly inspecting Red state. Planner objectives use SpeciesID instead.
func SpeciesName(id uint8) (string, bool) {
	name, ok := speciesByID[id]
	return name, ok
}

func SpeciesByName(name string) (uint8, bool) {
	id, ok := speciesTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}

// ItemName/ItemByName are Red adapter helpers retained for Red-state tooling.
// Planner objectives use ItemID instead.
func ItemName(id uint8) (string, bool) {
	name, ok := itemByID[id]
	return name, ok
}

func ItemByName(name string) (uint8, bool) {
	id, ok := itemTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}
