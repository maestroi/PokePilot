package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Kind is what an objective does.
type Kind uint8

const (
	KindGoTo    Kind = iota // walk to a named place
	KindTalk                // face and talk to something at a coordinate
	KindStarter             // complete the opening story and take a starter
	KindErrand              // deliver Oak's parcel (Viridian Mart -> Oak's lab)
	KindTrain               // battle in grass until the lead reaches Level
	KindHeal                // heal the party (the player must be at a center)
	KindGym                 // fight the Pewter Gym leader, Brock
)

// Objective is one unit of intent a planner can choose.
type Objective struct {
	Kind    Kind
	Place   string // KindGoTo: a name accepted by skill.Place
	X, Y    uint8  // KindTalk: the tile to face and talk to
	Level   uint8  // KindTrain: the level the lead should reach
	Starter string // KindStarter: charmander, squirtle, bulbasaur; empty is squirtle
	Note    string // human-readable, shown to a planner; never parsed
}

// starterOf maps KindStarter's name onto a table ball. Empty and unknown
// names keep the historic default (Squirtle) so existing offered lists
// that omit Starter stay bit-identical.
func starterOf(o Objective) skill.Starter {
	switch o.Starter {
	case "charmander":
		return skill.StarterCharmander
	case "bulbasaur":
		return skill.StarterBulbasaur
	default:
		return skill.StarterSquirtle
	}
}

// Execute carries out one objective against the emulator. Every error it
// returns names the objective, so a failure deep in a long loop is
// attributable. An unknown Kind is an error, not a no-op.
func Execute(m *emu.Emu, romData []byte, o Objective) error {
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
		res, err := skill.Travel(m, romData, dest, skill.StatAwareMove(romData), 20)
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if res.BlackedOut {
			// Losing is a typed outcome, not an error — but an unattended
			// run log must say it happened.
			fmt.Printf("  blacked out on the way (%d battles), resumed from a Pokemon Center\n", res.Battles)
		}
		return nil
	case KindTalk:
		if err := skill.Face(m, o.X, o.Y); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if _, err := skill.Talk(m); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindStarter:
		// GetStarter is idempotent: it returns nil immediately when the
		// rival-battle event is already set, so no guard is needed here.
		if err := skill.GetStarter(m, romData, starterOf(o), skill.StatAwareMove(romData)); err != nil {
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
		if res.BlackedOut {
			fmt.Printf("  blacked out training, resumed from a Pokemon Center\n")
		}
		return nil
	case KindHeal:
		// Heal talks to the nurse on the current map; a center elsewhere
		// is a separate KindGoTo objective, and the planner sequences them.
		if err := skill.Heal(m); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindGym:
		// The result is an outcome, not an error: a loss blackouts the
		// player to the Pewter center and the run can resume (train, heal,
		// come back), but an unattended run log must say it happened.
		outcome, err := skill.Gym(m, romData, skill.StatAwareMove(romData))
		if err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if outcome == state.ResultWon {
			fmt.Printf("  beat Brock: Boulder Badge set\n")
		} else {
			fmt.Printf("  lost to Brock, blacked out to the Pewter center\n")
		}
		return nil
	}
	return fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

// String renders a short, plain, stable one-line description of the
// objective. It is shown to a planner, never parsed.
func (o Objective) String() string {
	switch o.Kind {
	case KindGoTo:
		return "go to " + o.Place
	case KindTalk:
		return fmt.Sprintf("talk at (%d,%d)", o.X, o.Y)
	case KindStarter:
		return "take a starter"
	case KindErrand:
		return "deliver oak's parcel"
	case KindTrain:
		return fmt.Sprintf("train the lead to level %d", o.Level)
	case KindHeal:
		return "heal the party"
	case KindGym:
		return "beat the pewter gym leader"
	}
	return fmt.Sprintf("unknown kind %d", int(o.Kind))
}
