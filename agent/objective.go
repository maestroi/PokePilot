package agent

import (
	"fmt"
	"strings"

	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

type Kind uint8

const (
	KindGoTo Kind = iota
	KindTalk
	KindStarter
	_ // legacy KindErrand numeric slot; retained so existing generic kind values do not move
	KindTrain
	KindHeal
	KindGym
	KindCatch
	KindBuy
	KindPickup
	KindUseItem
	_ // legacy KindRocketHideout numeric slot
	_ // legacy KindPokemonTower numeric slot
	_ // legacy KindFuchsiaProgression numeric slot
	KindProgress
	KindTrainer
)

// Objective carries semantic planner arguments. Game-specific encodings stay
// behind the adapter boundary: Place/Species/Item/Progress are semantic IDs,
// never Red ROM/RAM bytes or named campaign verbs.
type Objective struct {
	Kind     Kind
	Place    PlaceID
	X, Y     uint8
	Starter  skill.Starter
	Progress ProgressID
	Level    uint8
	Species  SpeciesID
	Item     ItemID
	Slot     int
	Qty      int
	Flee     bool
	Note     string
	Intent   string
}

// Validate checks only portable shape/range invariants. Concrete-game name and
// progression-goal resolution is adapter-owned.
func (o Objective) Validate() error {
	switch o.Kind {
	case KindGoTo:
		if strings.TrimSpace(o.Place) == "" {
			return fmt.Errorf("agent: %s: empty place id", o)
		}
	case KindStarter:
		if o.Starter > skill.StarterBulbasaur {
			return fmt.Errorf("agent: %s: unknown starter %d", o, int(o.Starter))
		}
	case KindProgress:
		if strings.TrimSpace(string(o.Progress)) == "" {
			return fmt.Errorf("agent: %s: empty progression id", o)
		}
	case KindTrain:
		if o.Level < 1 || o.Level > 100 {
			return fmt.Errorf("agent: %s: level %d out of range 1..100", o, o.Level)
		}
		if o.Slot < 0 || o.Slot > 5 {
			return fmt.Errorf("agent: %s: party slot %d out of range 0..5", o, o.Slot)
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

func (o Objective) String() string {
	switch o.Kind {
	case KindGoTo:
		if o.Flee {
			return "go to " + o.Place + ", fleeing wild battles"
		}
		return "go to " + o.Place
	case KindTalk:
		return fmt.Sprintf("talk at (%d,%d)", o.X, o.Y)
	case KindTrainer:
		return fmt.Sprintf("challenge trainer at (%d,%d)", o.X, o.Y)
	case KindStarter:
		return "take the " + starterName(o.Starter) + " starter"
	case KindProgress:
		return "progress " + string(o.Progress)
	case KindTrain:
		if o.Species != "" {
			return fmt.Sprintf("train %s to level %d", strings.ToUpper(string(o.Species)), o.Level)
		}
		if o.Slot > 0 {
			return fmt.Sprintf("train party slot %d to level %d", o.Slot, o.Level)
		}
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
		return "catch a " + strings.ToUpper(string(o.Species)) + " here"
	case KindPickup:
		return fmt.Sprintf("pick up the %s at (%d,%d)", strings.ToUpper(string(o.Item)), o.X, o.Y)
	case KindUseItem:
		name := string(o.Item)
		return fmt.Sprintf("use %s %s on party slot %d", article(name), strings.ToUpper(name), o.Slot)
	case KindBuy:
		return fmt.Sprintf("buy %d %s", o.Qty, strings.ToUpper(string(o.Item)))
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
	if name == "" {
		return "a"
	}
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

// Red adapter vocabulary is centralized in red/data so the profile and the
// executor cannot silently diverge on raw species/item indexes.
func SpeciesCount() int { return reddata.SpeciesCount() }

func SpeciesName(id uint8) (string, bool) { return reddata.SpeciesName(id) }

// SpeciesByName is planner-facing and returns the semantic identity. Use
// redSpeciesID when a Red executor needs the ROM byte.
func SpeciesByName(name string) (SpeciesID, bool) { return semanticSpecies(name) }

func ItemName(id uint8) (string, bool) { return reddata.ItemName(id) }

// ItemByName is planner-facing and returns the semantic identity. Use
// redItemID/resolveItemID at the Red boundary for a native bag byte.
func ItemByName(name string) (ItemID, bool) { return semanticItem(name) }
