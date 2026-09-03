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
		// Travel, not GoTo: GoTo aborts on the first wild battle by
		// design, which on any route through grass means the run stops at
		// the first Pidgey. maxBattles bounds it; 20 is the cap the
		// fixtures use for the Pallet -> Viridian legs, which measured 1.
		// Flee is the planner's call (a fled wild is XP the run did not
		// get), so the default stays fight: TravelFlee only when asked.
		var err error
		if o.Flee {
			_, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
		} else {
			_, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
		}
		if err != nil {
			// A blackout ends the journey with ErrBlackedOut: skill.Travel
			// pairs res.BlackedOut with that error, so no blackout reaches
			// the nil return below. The journey genuinely ended — the party
			// is healed and standing in a town — and Run exempts
			// ErrBlackedOut from the failure budget for exactly that reason.
			// The error is the channel the planner reads; the exemption is
			// the recoverable half. (The old res.BlackedOut print branch was
			// dead: the err check above already returned every blackout.)
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindTalk:
		if _, err := skill.TalkAt(m, romData, o.X, o.Y, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindStarter:
		// GetStarter is idempotent: it returns nil immediately when the
		// rival-battle event is already set, so no guard is needed here.
		// Which ball is the planner's choice, not this layer's: the three
		// starters are offered as three objectives and the model picks.
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
		// 20 battles is the same cap the fixtures use for the Pallet ->
		// Viridian legs; a lead far below Level will report it as a
		// reached-level shortfall rather than burn an unattended run.
		res, err := skill.Train(m, romData, int(o.Level), skill.StatAwareMove(romData), 20)
		if err != nil {
			return fmt.Errorf("agent: %s: %v (battles=%d, reached=%v, blackedOut=%v)", o, err, res.Battles, res.Reached, res.BlackedOut)
		}
		if res.Reached {
			return nil
		}
		if res.Retreated {
			// The session stopped while the party could still walk: a
			// shortfall with a reason. It is its own outcome, not a blackout,
			// so Run exempts it from the failure accounting the way it
			// exempts one — and the level is in the text on purpose: two
			// retreats that end at the same level read as the same failure,
			// which is exactly the case the exemption must absorb. The hurt
			// lead itself is in the next observation's party HP, where the
			// planner reads it and decides whether to heal.
			return fmt.Errorf("agent: %s: %w (ended level %d)", o, skill.ErrTrainRetreat, res.EndLevel)
		}
		if res.BlackedOut {
			return fmt.Errorf("agent: %s: %w before reaching level %d (ended level %d after %d battles)",
				o, skill.ErrBlackedOut, o.Level, res.EndLevel, res.Battles)
		}
		return fmt.Errorf("agent: %s: target level %d not reached (ended level %d after %d battles)",
			o, o.Level, res.EndLevel, res.Battles)
	case KindHeal:
		// Heal talks to the nurse on the current map. With a Place, the
		// walk there is part of the objective: a hurt party in the field
		// would otherwise have to spend one round walking and another
		// healing, and Offer only names a center the run has already been
		// inside, so this is a way back, never a way to somewhere new.
		if o.Place != "" {
			dest, ok := skill.Place(o.Place)
			if !ok {
				return fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
			}
			// Travel, not GoTo, for the same reason KindGoTo uses it: the
			// way back to a center runs through grass, and a hurt party is
			// exactly the one that meets a wild Pokemon on the way. A
			// blackout here is a FAILURE, not an outcome: the objective said
			// "heal the party at X" and the respawn put the player at the
			// last center they used, not at X. It is returned wrapped in
			// ErrBlackedOut, which Run exempts from the failure budget: the
			// party is healed and the world changed, so the round is
			// recoverable, the same exemption as a lost battle on any
			// journey.
			var err error
			if o.Flee {
				_, err = skill.TravelFlee(m, romData, dest, skill.StatAwareMove(romData), 20)
			} else {
				_, err = skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
			}
			if err != nil {
				// A blackout on the way comes back as ErrBlackedOut: the
				// skill pairs res.BlackedOut with that error, so it is caught
				// here and the journey's TravelResult carries no blackout the
				// error does not already say (the old res.BlackedOut re-check
				// was dead, as KindGoTo's print branch was).
				return fmt.Errorf("agent: %s: %w", o, err)
			}
		}
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindGym:
		// The postcondition is the badge: skill.Gym returns ResultWon only
		// when the badge bit is set in RAM. A loss is the objective NOT
		// having done what it said ("beat the gym leader here"), so it is a
		// failure, not a done round: History would say "done" for a loss and
		// the menu would grow a "(done 1x)" on a gym the run lost to. The
		// loss blackouts the player to a center, so the run stays
		// recoverable — the planner reads the failure tally and the quoted
		// error, heals and trains, and comes back. (A blackout on the way IN
		// is skill.ErrBlackedOut, which Run exempts from the failure budget;
		// a loss to the leader is not exempt: the objective failed.)
		outcome, err := skill.Gym(m, romData, skill.StatAwareMove(romData))
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if outcome == state.ResultWon {
			fmt.Printf("  beat Brock: Boulder Badge set\n")
		}
		return gymOutcomeErr(o, outcome)
	case KindCatch:
		// A missed hunt is the game answering, not a defect: the outcome
		// is reported and the planner decides what to do next. Five balls
		// is the cap the catch fixtures use.
		res, err := skill.Catch(m, romData, []uint8{o.Species}, skill.StatAwareMove(romData), 5)
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		name, _ := SpeciesName(o.Species) // Validate already checked it
		switch res.Outcome {
		case skill.OutcomeCaught:
			fmt.Printf("  caught %s (balls=%d, encounters=%d)\n", strings.ToUpper(name), res.BallsThrown, res.Encounters)
			return nil
		default:
			// A hunt that ended without the party having grown is the
			// objective NOT having done what it said ("catch a X here"), so
			// it is a failure: the planner reads the failure tally and the
			// game's answer in the error text, which a recorded "done" round
			// never gave it. (A blackout inside the hunt comes back as
			// skill.ErrCatchBlackout on the err path above, and Run exempts
			// ErrBlackedOut from the failure budget for the usual reason.)
			return fmt.Errorf("agent: %s: no %s caught (outcome %s, balls=%d, encounters=%d)",
				o, strings.ToUpper(name), catchOutcomeName(res.Outcome), res.BallsThrown, res.Encounters)
		}
	case KindPickup:
		// The proof is inside Pickup: it returns nil only when the bag's
		// count for Item rose by one. A text box opening is not evidence.
		if err := skill.Pickup(m, romData, o.X, o.Y, o.Item, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindUseItem:
		// The proof is inside UseFieldItem: it returns nil only when the
		// target's HP rose or its status cleared, read from RAM before and
		// after — a closed menu is not evidence (S8-5).
		if err := skill.UseFieldItem(m, o.Item, o.Slot); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindRocketHideout:
		// This story verb is atomic for the same reason Gym owns Surge's
		// exterior Cut and trash switches: the intermediate interactions
		// are prerequisites the planner cannot express as standalone
		// objectives. The skill succeeds only once the Silph Scope is in
		// the bag, so a Giovanni win without the pickup is not recorded as
		// a completed round.
		if err := skill.RocketHideout(m, romData, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindBuy:
		err := skill.Buy(m, o.Item, o.Qty)
		if err != nil {
			// CantAfford and NotInStock are game outcomes the planner can
			// react to (earn money, go elsewhere) rather than setup
			// failures — but they are still FAILURES, and they used to
			// return nil. A purchase that did not happen was recorded as a
			// completed round: history said "done", Knowledge counted it in
			// Completed, and the menu line grew a "(done 1x)" for something
			// the clerk had refused. The planner then had no reason to stop
			// choosing it. Returning the error puts it in the failure tally
			// instead, which is where the run can see it and where the
			// game's own words ("the clerk does not stock...") are quoted
			// back on the next round.
			item, _ := ItemName(o.Item) // Validate already checked it
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
		// "" means the center the player is standing in. A named one must
		// resolve AND be a center: travelling to a mart and asking a clerk
		// for a heal is a guaranteed failed round.
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
		// The party caps at six (state.DecodeParty) and the slot is 0-based,
		// the same addressing skill.UseFieldItem takes: no second scheme.
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
		// "here" and not the leader's name: Offer puts this on the menu
		// only while the player is standing in a gym, and naming Brock on
		// the Cerulean menu would be a lie the planner cannot check.
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
	case KindBuy:
		if name, ok := ItemName(o.Item); ok {
			return fmt.Sprintf("buy %d %s", o.Qty, strings.ToUpper(name))
		}
		return fmt.Sprintf("buy %d of item %d", o.Qty, o.Item)
	}
	return fmt.Sprintf("unknown kind %d", int(o.Kind))
}

// gymOutcomeErr renders a gym battle result as the objective's error: nil
// when the badge is in RAM (skill.Gym's win postcondition), the loss
// otherwise. It is the KindGym branch's whole decision, separated so a
// test pins it without a full journey to the gym map: the win side of the
// journey is pinned by skill.TestGymBoulderBadge, and a loss side measured
// on 2026-08-31 panics inside the vendored emulator's APU (apu.sample
// index out of range) before the fight ends, which is a defect of that
// emulator, not of this branch.
func gymOutcomeErr(o Objective, outcome state.BattleResult) error {
	if outcome == state.ResultWon {
		return nil
	}
	return fmt.Errorf("agent: %s: lost to the gym leader (blacked out to the center)", o)
}

// catchOutcomeName renders a skill.CatchOutcome for the error text the
// planner reads. The enum has no String() of its own, and a bare number in
// a failure the planner has to act on is not an answer.
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

// article renders the indefinite article for an item name: "a POTION",
// "an ANTIDOTE". The check is on the first letter, which is all the
// table's names need (no TH-word among them).
func article(name string) string {
	switch name[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

// starterName renders a skill.Starter for String(). An out-of-range value
// says so rather than silently picking a ball.
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

// The species and item tables are the argument vocabulary. A value the
// planner can name is a value it can aim at; anything else is a typed error
// upstream, before a skill ever sees it. Indices come from
// pokered/constants/pokemon_constants.asm and item_constants.asm (ROM
// pokemon / bag-item indexes, NOT pokedex numbers).

// speciesTable is decoded from the ROM's own MonsterNames table (bank 07,
// 0x421E; 10 bytes per entry, indexed by internal species index minus one,
// as home/names.asm GetMonName reads it) and written down here so Validate
// and String stay pure functions with no ROM to carry. All 151 are present:
// a hand-picked subset silently caps what the planner is allowed to want,
// and the map's own wild table (skill.WildGrass) now names species no
// hand-written list would have contained. The 28-entry list this replaces
// agreed with the ROM on 27 of its entries; the 28th said "farow" for what
// the game calls FEAROW, which is the argument for decoding rather than
// typing.
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
	"silph scope": 0x48,
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

// SpeciesCount is how many species the table names. It exists so a test can
// pin "the whole roster" without listing it.
func SpeciesCount() int { return len(speciesTable) }

// SpeciesName returns the lowercase display name of a ROM species index.
func SpeciesName(id uint8) (string, bool) {
	name, ok := speciesByID[id]
	return name, ok
}

// SpeciesByName resolves a model-supplied species name to its ROM index.
// Matching is exact after trimming and lowercasing: "Caterpie" works,
// "caterpy" does not, and neither does anything fuzzy.
func SpeciesByName(name string) (uint8, bool) {
	id, ok := speciesTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}

// ItemName returns the lowercase display name of a bag item ID.
func ItemName(id uint8) (string, bool) {
	name, ok := itemByID[id]
	return name, ok
}

// ItemByName resolves a model-supplied item name to its bag item ID, with
// the same exact-match rule as SpeciesByName.
func ItemByName(name string) (uint8, bool) {
	id, ok := itemTable[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}
