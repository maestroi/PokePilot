package agent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// Kind is the small, typed action vocabulary the model is allowed to choose.
type Kind uint8

const (
	KindGoTo Kind = iota + 1
	KindTalk
	KindTrainer
	KindStarter
	KindProgress
	KindTrain
	KindHeal
	KindGym
	KindCatch
	KindPickup
	KindUseItem
	KindBuy
)

type Objective struct {
	Kind Kind
	// Place and semantic arguments deliberately use game-independent ids.
	// Red translates them to native map/item/species bytes only at the
	// execution/catalog boundary.
	Place    PlaceID
	X, Y     uint8
	Starter  skill.Starter
	Progress ProgressID
	Species  SpeciesID
	Item     ItemID
	Slot     int
	Level    uint8
	Qty      int
	Flee     bool
	Intent   string
	Note     string
}

func (o Objective) Validate() error {
	switch o.Kind {
	case KindGoTo:
		if strings.TrimSpace(string(o.Place)) == "" {
			return fmt.Errorf("agent: %s: empty place id", o)
		}
	case KindTalk, KindTrainer:
	case KindStarter:
		if o.Species == "" {
			switch o.Starter {
			case skill.StarterCharmander, skill.StarterSquirtle, skill.StarterBulbasaur:
			default:
				return fmt.Errorf("agent: %s: invalid starter %d", o, int(o.Starter))
			}
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
		if o.Species != "" {
			return "take the " + string(o.Species) + " starter"
		}
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
				return "heal the party at " + strings.ToUpper(string(o.Place)) + ", fleeing wild battles"
			}
			return "heal the party at " + strings.ToUpper(string(o.Place))
		}
		return "heal the party"
	case KindGym:
		return "beat the gym leader here"
	case KindCatch:
		species := strings.ToUpper(string(o.Species))
		switch o.Intent {
		case dexVirtualTradebackIntent:
			return "trade a party Pokemon through the virtual Cable Club and trade it back to obtain " + species
		case dexVirtualVersionIntent:
			return "trade through the virtual Cable Club for version-exclusive " + species
		case dexVirtualPokedexIntent:
			return "trade through the virtual Cable Club for otherwise unavailable " + species
		default:
			return "catch a " + species + " here"
		}
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
	return fmt.Errorf("agent: %s: %w (blacked out to the center)", o, errGymLeaderLost)
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

// A few Red-owned agent modules still register item vocabulary during package
// init (economy catalog, PP restorers and TM/HMs). Keep their legacy map names
// as aliases to red/data's canonical maps so those registrations remain one
// source of truth during the incremental profile migration.
var itemTable, itemByID = data.LegacyMutableItemTables()

// Red adapter vocabulary is centralized in red/data so the profile and the
// executor cannot silently diverge on raw species/item indexes.
func SpeciesCount() int { return data.SpeciesCount() }

func SpeciesName(id uint8) (string, bool) { return data.SpeciesName(id) }

// SpeciesByName is planner-facing and returns the semantic identity. Use
// redSpeciesID when a Red executor needs the ROM byte.
func SpeciesByName(name string) (SpeciesID, bool) { return semanticSpecies(name) }

func ItemName(id uint8) (string, bool) { return data.ItemName(id) }

// ItemByName is planner-facing and returns the semantic identity. Use
// redItemID/resolveItemID at the Red boundary for a native bag byte.
func ItemByName(name string) (ItemID, bool) { return semanticItem(name) }

func answerInt(reply string) (string, bool) {
	fields := strings.Fields(reply)
	for i := len(fields) - 1; i >= 0; i-- {
		v := strings.Trim(fields[i], "`.,:;!?()[]{}\"'")
		if _, err := strconv.Atoi(v); err == nil {
			return v, true
		}
	}
	return "", false
}

func gymLossFailureName(o Objective, err error) (string, bool) {
	if o.Kind != KindGym || !errors.Is(err, errGymLeaderLost) {
		return "", false
	}
	return gymLossFailureKey(o.Place), true
}
