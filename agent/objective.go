package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Kind is what an objective does.
type Kind uint8

const (
	KindGoTo          Kind = iota // walk to a named place
	KindTalk                      // face and talk to something at a coordinate
	KindStarter                   // complete the opening story and take a chosen starter
	KindErrand                    // deliver Oak's parcel (Viridian Mart -> Oak's lab)
	KindTrain                     // battle in grass until the lead reaches Level
	KindHeal                      // heal the party at a center; Place names one to travel to first
	KindGym                       // fight the leader of whichever gym the player is in
	KindCatch                     // hunt tall grass for a wanted species and catch it
	KindBuy                       // buy Item x Qty from the mart clerk
	KindPickup                    // pick up the item at a coordinate; the bag must rise
	KindUseItem                   // use one bag item on one party member, out in the field
	KindRocketHideout             // clear the Celadon Rocket Hideout and obtain the Silph Scope
	KindPokemonTower              // clear Pokemon Tower and obtain the Poke Flute
	KindFuchsiaProgression        // reach Fuchsia, beat Koga, and obtain Surf + Strength
)

// Objective is one unit of intent a planner can choose. The argument fields
// are per-kind: Place for KindGoTo and KindHeal, X/Y for KindTalk, Starter for
// KindStarter, Level for KindTrain, Species for KindCatch, Item and Qty for
// KindBuy, X/Y and Item for KindPickup. Every argument is checked by Validate before it reaches a skill;
// an out-of-range value is a typed error that stops the round, never a
// clamp or a best match.
type Objective struct {
	Kind    Kind
	Place   string        // KindGoTo: a name accepted by skill.Place; KindHeal: a center to travel to first, "" for the one you are standing in
	X, Y    uint8         // KindTalk, KindPickup: the tile to face
	Starter skill.Starter // KindStarter: which ball to take
	Level   uint8         // KindTrain: the level the lead should reach
	Species uint8         // KindCatch: the ROM species index to hunt
	Item    uint8         // KindBuy, KindPickup, KindUseItem: the bag item ID
	Slot    int           // KindUseItem: the party slot to use it on (0-based, as skill.UseFieldItem takes it)
	Qty     int           // KindBuy: how many
	Flee    bool          // KindGoTo, KindHeal-with-Place: resolve wild encounters by running, not fighting
	Note    string        // human-readable, shown to a planner; never parsed
	// Intent is the sentence the planner attached to this choice: what it is
	// in service of. It is run memory, not an argument — Validate and
	// Execute ignore it, String() does not render it, and Run carries it
	// verbatim onto the next round's Observation (never edited or
	// summarised). WithArgs is the only writer; a model that says nothing
	// leaves it empty.
	Intent string
}

// Execute carries out one objective against the emulator. Every error it
// returns names the objective, so a failure deep in a long loop is
// attributable. An unknown Kind is an error, not a no-op, and an argument
// outside its stated range is rejected before any input is sent.
//
// The rule for a typed outcome a skill reports: an objective succeeds only
// when the WORLD NOW MATCHES WHAT THE OBJECTIVE SAID IT WOULD DO (its
// String()), not when the skill exited tidily. A hunt that ended without
// the party growing, a gym the run lost to, a purchase the clerk refused —
// each is returned as an error, because a recorded "done" round would put
// the objective in Knowledge.Completed (the menu's "(done Nx)") and out of
// the failure tally, telling the planner it had done the thing it just
// failed to do. The recoverable game outcomes (a blackout that ends a
// journey, a train retreat) are errors too: Run exempts their typed
// sentinels (ErrBlackedOut, ErrTrainRetreat) from the failure budget, and
// that exemption — not a nil return — is what keeps them from ending a
// run.
func Execute(m *emu.Emu, romData []byte, o Objective) (retErr error) {
	if err := o.Validate(); err != nil {
		return err
	}
	// Preserve the exact emulator memory at the objective-error boundary.
	// Run may recover dialogue immediately after this function returns, so
	// capturing later can erase the transient menu/transition state we need
	// to understand. Forensics is best-effort: its own failure is logged and
	// never replaces the gameplay error the planner must see.
	defer func() {
		if retErr == nil {
			return
		}
		if err := captureObjectiveFailure(m, o, retErr); err != nil {
			fmt.Printf("  ram forensics: %v\n", err)
		}
	}()

	switch o.Kind {
	case KindGoTo:
		dest, ok := skill.Place(o.Place)
		if !ok {
			return fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
		}
		var err error
		if o.Flee {
			_, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
		} else {
			_, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
		}
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindTalk:
		if _, err := skill.TalkAt(m, romData, o.X, o.Y, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindStarter:
		if err := skill.GetStarter(m, romData, o.Starter, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindErrand:
		if err := skill.OaksParcel(m, romData, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindTrain:
		res, err := skill.Train(m, romData, int(o.Level), skill.StatAwareMove(romData), 20)
		if err != nil {
			return fmt.Errorf("agent: %s: %v (battles=%d, reached=%v, blackedOut=%v)", o, err, res.Battles, res.Reached, res.BlackedOut)
		}
		if res.Reached {
			return nil
		}
		if res.Retreated {
			return fmt.Errorf("agent: %s: %w (ended level %d)", o, skill.ErrTrainRetreat, res.EndLevel)
		}
		if res.BlackedOut {
			return fmt.Errorf("agent: %s: %w before reaching level %d (ended level %d after %d battles)",
				o, skill.ErrBlackedOut, o.Level, res.EndLevel, res.Battles)
		}
		return fmt.Errorf("agent: %s: target level %d not reached (ended level %d after %d battles)",
			o, o.Level, res.EndLevel, res.Battles)
	case KindHeal:
		if o.Place != "" {
			dest, ok := skill.Place(o.Place)
			if !ok {
				return fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
			}
			var err error
			if o.Flee {
				_, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
			} else {
				_, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
			}
			if err != nil {
				return fmt.Errorf("agent: %s: %w", o, err)
			}
		}
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindGym:
		outcome, err := skill.Gym(m, romData, skill.StatAwareMove(romData))
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if outcome == state.ResultWon {
			fmt.Printf("  beat gym leader: badge set\n")
		}
		return gymOutcomeErr(o, outcome)
	case KindCatch:
		res, err := skill.Catch(m, romData, []uint8{o.Species}, skill.StatAwareMove(romData), 5)
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		name, _ := SpeciesName(o.Species)
		switch res.Outcome {
		case skill.OutcomeCaught:
			fmt.Printf("  caught %s (balls=%d, encounters=%d)\n", strings.ToUpper(name), res.BallsThrown, res.Encounters)
			return nil
		default:
			return fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)",
				o, strings.ToUpper(name), catchOutcomeName(res.Outcome), res.BallsThrown, res.Encounters)
		}
	case KindPickup:
		if err := skill.Pickup(m, romData, o.X, o.Y, o.Item, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindUseItem:
		if err := skill.UseFieldItem(m, o.Item, o.Slot); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindRocketHideout:
		if err := skill.RocketHideout(m, romData, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindPokemonTower:
		if err := skill.PokemonTower(m, romData, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindFuchsiaProgression:
		// The post-Tower story slice owns the Snorlax gate, Koga, and the
		// finite Safari/Warden handoffs atomically. The skill returns nil only
		// after Soul Badge + HM03 + HM04 are all positively present.
		if err := skill.FuchsiaProgression(m, romData, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindBuy:
		err := skill.Buy(m, o.Item, o.Qty)
		if err != nil {
			item, _ := ItemName(o.Item)
			fmt.Printf("  buy failed: %s (%d x %s)\n", err, o.Qty, strings.ToUpper(item))
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		item, _ := ItemName(o.Item)
		fmt.Printf("  bought %d x %s\n", o.Qty, strings.ToUpper(item))
		return nil
	}
	return fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

// Validate checks every argument against its stated range and returns a
// typed error naming the offending value. It is the safety mechanism for
// model-supplied arguments: the reply schema (when a server honors it) only
// makes malformed replies less likely, so this runs on EVERY objective,
// from every planner, before any input reaches the emulator. Values are
// never clamped and never best-matched: a level of 101 is an error, not 100.
func (o Objective) Validate() error {
	switch o.Kind {
	case KindStarter:
		if o.Starter > skill.StarterBulbasaur {
			return fmt.Errorf("agent: %s: unknown starter %d (want charmander, squirtle or bulbasaur)", o, int(o.Starter))
		}
	case KindTrain:
		if o.Level < 1 || o.Level > 100 {
			return fmt.Errorf("agent: %s: level %d out of range 1..100", o, o.Level)
		}
	case KindCatch:
		if _, ok := SpeciesName(o.Species); !ok {
			return fmt.Errorf("agent: %s: unknown species %d", o, o.Species)
		}
	case KindPickup:
		if _, ok := ItemName(o.Item); !ok {
			return fmt.Errorf("agent: %s: unknown item %d", o, int(o.Item))
		}
	case KindHeal:
		if o.Place != "" {
			d, ok := skill.Place(o.Place)
			if !ok {
				return fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
			}
			if !isCenter(state.MapName(d.Map)) {
				return fmt.Errorf("agent: %s: %q is not a Pokemon Center", o, o.Place)
			}
		}
	case KindUseItem:
		if _, ok := ItemName(o.Item); !ok {
			return fmt.Errorf("agent: %s: unknown item %d", o, int(o.Item))
		}
		if o.Slot < 0 || o.Slot > 5 {
			return fmt.Errorf("agent: %s: party slot %d out of range 0..5", o, o.Slot)
		}
	case KindBuy:
		if o.Qty < 1 || o.Qty > 99 {
			return fmt.Errorf("agent: %s: quantity %d out of range 1..99", o, o.Qty)
		}
		if _, ok := ItemName(o.Item); !ok {
			return fmt.Errorf("agent: %s: unknown item %d", o, o.Item)
		}
	}
	return nil
}

// String renders a short, plain, stable one-line description of the
// objective. It is shown to a planner, never parsed. Arguments are part of
// the sentence: "train the lead to level 7", "catch a PIDGEY here" — the
// model reads what it would be choosing.
func (o Objective) String() string {
	switch o.Kind {
	case KindGoTo:
		if o.Flee {
			return "go to " + o.Place + ", fleeing wild battles"
		}
		return "go to " + o.Place
	case KindTalk:
		return fmt.Sprintf("talk at (%d,%d)", o.X, o.Y)
	case KindStarter:
		return "take the " + starterName(o.Starter) + " starter"
	case KindErrand:
		return "deliver oak's parcel"
	case KindTrain:
		return fmt.Sprintf("train the lead to level %d", o.Level)
	case KindHeal:
		if o.Place != "" {
			if o.Flee {
				return "heal the party at " + strings.ToUpper(o.Place) + ", fleeing wild battles"
			}
			return "heal the party at " + strings.ToUpper(o.Place)
		}
		return "heal the party"
	case KindGym:
		return "beat the gym leader here"
	case KindCatch:
		if name, ok := SpeciesName(o.Species); ok {
			return "catch a " + strings.ToUpper(name) + " here"
		}
		return fmt.Sprintf("catch species %d here", o.Species)
	case KindPickup:
		if name, ok := ItemName(o.Item); ok {
			return fmt.Sprintf("pick up the %s at (%d,%d)", strings.ToUpper(name), o.X, o.Y)
		}
		return fmt.Sprintf("pick up item %d at (%d,%d)", int(o.Item), o.X, o.Y)
	case KindUseItem:
		if name, ok := ItemName(o.Item); ok {
			return fmt.Sprintf("use %s %s on party slot %d", article(name), strings.ToUpper(name), o.Slot)
		}
		return fmt.Sprintf("use item %d on party slot %d", int(o.Item), o.Slot)
	case KindRocketHideout:
		return "clear the Rocket Hideout and get the SILPH SCOPE"
	case KindPokemonTower:
		return "clear Pokemon Tower and get the POKE FLUTE"
	case KindFuchsiaProgression:
		return "reach Fuchsia, beat Koga, and get HM03 SURF + HM04 STRENGTH"
	case KindBuy:
		if name, ok := ItemName(o.Item); ok {
			return fmt.Sprintf("buy %d %s", o.Qty, strings.ToUpper(name))
		}
		return fmt.Sprintf("buy %d of item %d", o.Qty, o.Item)
	}
	return fmt.Sprintf("unknown kind %d", int(o.Kind))
}

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
	switch name[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

func starterName(s skill.Starter) string {
	switch s {
	case skill.StarterCharmander:
		return "charmander"
	case skill.StarterSquirtle:
		return "squirtle"
	case skill.StarterBulbasaur:
		return "bulbasaur"
	}
	return fmt.Sprintf("unknown starter %d", int(s))
}

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

func SpeciesName(id uint8) (string, bool) {
	name, ok := speciesByID[id]
	return name, ok
}

func SpeciesByName(name string) (uint8, bool) {
	id, ok := speciesTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}

func ItemName(id uint8) (string, bool) {
	name, ok := itemByID[id]
	return name, ok
}

func ItemByName(name string) (uint8, bool) {
	id, ok := itemTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}
