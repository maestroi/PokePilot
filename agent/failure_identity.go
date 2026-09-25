package agent

import (
	"sort"
	"strings"
)

// FailureCauseID is the stable cause vocabulary used by failure fingerprints.
// Concrete game/controller errors are normalized by the game adapter before
// generic recovery policy sees them.
type FailureCauseID string

type FailureObjective struct {
	Kind            string
	Place           PlaceID
	X, Y            uint8
	Starter         string
	Progress        ProgressID
	FieldCapability CapabilityID
	Level           uint8
	Species         SpeciesID
	Item            ItemID
	Slot            int
	Qty             int
	Flee            bool
}

type FailurePartyMember struct {
	Species    SpeciesID
	Level      uint8
	Experience uint32
	HP         uint16
	MaxHP      uint16
	Status     string
}

type FailureInventoryItem struct {
	ID       ItemID
	Quantity int
}

type FailureCapability struct {
	ID         CapabilityID
	BadgeOwned bool
	HMOwned    bool
	Learned    bool
	Usable     bool
}

type FailureProgressFact struct {
	ID       ProgressID
	Complete bool
	Value    int
}

// FailureState is the compact semantic subset of Observation that may change
// whether an objective succeeds. Raw Red map bytes, RAM fields, dialogue and
// run history are deliberately absent.
type FailureState struct {
	Location     PlaceID
	X, Y         uint8
	Controllable bool
	InBattle     bool
	Money        uint32
	Party        []FailurePartyMember
	Inventory    []FailureInventoryItem
	Badges       []string
	Capabilities []FailureCapability
	Progress     []FailureProgressFact
}

func FailureObjectiveFor(o Objective) FailureObjective {
	return FailureObjective{
		Kind:            failureKindName(o.Kind),
		Place:           o.Place,
		X:               o.X,
		Y:               o.Y,
		Starter:         starterName(o.Starter),
		Progress:        o.Progress,
		FieldCapability: o.FieldCapability,
		Level:           o.Level,
		Species:         o.Species,
		Item:            o.Item,
		Slot:            o.Slot,
		Qty:             o.Qty,
		Flee:            o.Flee,
	}
}

func failureKindName(k Kind) string {
	switch k {
	case KindGoTo:
		return "go_to"
	case KindTalk:
		return "talk"
	case KindTrainer:
		return "trainer"
	case KindStarter:
		return "starter"
	case KindTrain:
		return "train"
	case KindHeal:
		return "heal"
	case KindGym:
		return "gym"
	case KindCatch:
		return "catch"
	case KindBuy:
		return "buy"
	case KindPickup:
		return "pickup"
	case KindUseItem:
		return "use_item"
	case KindProgress:
		return "progress"
	case KindRepairFieldCapability:
		return "repair_field_capability"
	default:
		return "unknown"
	}
}

func FailureStateFor(obs Observation) FailureState {
	out := FailureState{
		Location:     obs.Location,
		X:            obs.X,
		Y:            obs.Y,
		Controllable: obs.Controllable,
		InBattle:     obs.InBattle,
		Money:        obs.Money,
		Party:        make([]FailurePartyMember, 0, len(obs.Party)),
		Inventory:    make([]FailureInventoryItem, 0, len(obs.Bag)),
		Badges:       append([]string(nil), obs.Badges...),
		Capabilities: make([]FailureCapability, 0, len(obs.FieldCapabilities)),
		Progress:     make([]FailureProgressFact, 0, len(obs.Story)),
	}
	for _, mon := range obs.Party {
		out.Party = append(out.Party, FailurePartyMember{
			Species:    mon.Species,
			Level:      mon.Level,
			Experience: mon.Experience,
			HP:         mon.HP,
			MaxHP:      mon.MaxHP,
			Status:     strings.ToLower(strings.TrimSpace(mon.Status)),
		})
	}
	for _, item := range obs.Bag {
		out.Inventory = append(out.Inventory, FailureInventoryItem{
			ID:       ItemID(strings.ToLower(strings.TrimSpace(item.Name))),
			Quantity: item.Quantity,
		})
	}
	for _, cap := range obs.FieldCapabilities {
		out.Capabilities = append(out.Capabilities, FailureCapability{
			ID:         cap.Name,
			BadgeOwned: cap.BadgeOwned,
			HMOwned:    cap.HMOwned,
			Learned:    cap.Learned,
			Usable:     cap.Usable,
		})
	}
	for _, fact := range obs.Story {
		out.Progress = append(out.Progress, FailureProgressFact{
			ID:       fact.ID,
			Complete: fact.Complete,
			Value:    fact.Value,
		})
	}
	sort.Strings(out.Badges)
	sort.Slice(out.Inventory, func(i, j int) bool { return out.Inventory[i].ID < out.Inventory[j].ID })
	sort.Slice(out.Capabilities, func(i, j int) bool { return out.Capabilities[i].ID < out.Capabilities[j].ID })
	sort.Slice(out.Progress, func(i, j int) bool { return out.Progress[i].ID < out.Progress[j].ID })
	return out
}
