package agent

import (
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// FailureCauseID is the typed/root-cause vocabulary used by farm failure
// fingerprints. Values are semantic controller/runtime identities, never error
// message fragments.
type FailureCauseID string

type FailureObjective struct {
	Kind     string
	Place    PlaceID
	X, Y     uint8
	Starter  string
	Progress ProgressID
	Level    uint8
	Species  SpeciesID
	Item     ItemID
	Slot     int
	Qty      int
	Flee     bool
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
		Kind:     failureKindName(o.Kind),
		Place:    o.Place,
		X:        o.X,
		Y:        o.Y,
		Starter:  starterName(o.Starter),
		Progress: o.Progress,
		Level:    o.Level,
		Species:  o.Species,
		Item:     o.Item,
		Slot:     o.Slot,
		Qty:      o.Qty,
		Flee:     o.Flee,
	}
}

func failureKindName(k Kind) string {
	switch k {
	case KindGoTo:
		return "go_to"
	case KindTalk:
		return "talk"
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

func failureCauseFor(err error) (FailureCauseID, []string) {
	if err == nil {
		return "", nil
	}

	var routeBlocked *world.RouteBlockedError
	if errors.As(err, &routeBlocked) {
		missing := routeBlocked.MissingCapabilities()
		ctx := make([]string, 0, len(missing))
		for _, id := range missing {
			ctx = append(ctx, string(id))
		}
		sort.Strings(ctx)
		return "route_prerequisite_missing", ctx
	}
	if errors.Is(err, ErrObjectiveBoundaryChoice) {
		return "objective_boundary_choice", nil
	}
	var choice *skill.ErrDialogueChoice
	if errors.As(err, &choice) {
		return "dialogue_choice_required", nil
	}
	if errors.Is(err, skill.ErrFieldItemPrompt) {
		return "field_item_prompt", nil
	}
	if errors.Is(err, ErrObjectivePostconditionFailed) {
		return "objective_postcondition_failed", nil
	}
	if errors.Is(err, ErrObjectivePostconditionUnavailable) {
		return "objective_postcondition_unavailable", nil
	}
	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return "objective_boundary_dirty", nil
	}
	if errors.Is(err, skill.ErrShopStabilization) {
		return "shop_stabilization_failed", nil
	}
	if errors.Is(err, emu.ErrFrameDeadline) {
		return "frame_deadline", nil
	}
	if errors.Is(err, skill.ErrNavigationStalled) {
		return "navigation_stalled", nil
	}
	if errors.Is(err, skill.ErrShopMenuTimeout) {
		return "shop_menu_timeout", nil
	}
	if errors.Is(err, skill.ErrShopControllerStalled) {
		return "shop_controller_stalled", nil
	}
	if errors.Is(err, skill.ErrReplanExhausted) {
		return "route_replan_exhausted", nil
	}
	if errors.Is(err, skill.ErrMenuStuck) {
		return "menu_stuck", nil
	}
	if errors.Is(err, skill.ErrCutsceneTimeout) {
		return "cutscene_timeout", nil
	}
	if errors.Is(err, skill.ErrForcedChoiceStuck) {
		return "forced_choice_stuck", nil
	}
	if errors.Is(err, skill.ErrPickupMenu) {
		return "pickup_menu_stuck", nil
	}
	if errors.Is(err, skill.ErrBattle) || errors.Is(err, skill.ErrBattleInterrupted) {
		return "battle_escaped_owner", nil
	}
	if errors.Is(err, skill.ErrFieldItemNoEffect) {
		return "field_item_no_effect", nil
	}
	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return "objective_boundary_dirty", nil
	}
	if errors.Is(err, skill.ErrBlackedOut) {
		return "blacked_out", nil
	}
	if errors.Is(err, skill.ErrCatchBlackout) {
		return "catch_blackout", nil
	}
	if errors.Is(err, skill.ErrCatchHuntExhausted) {
		return "catch_hunt_exhausted", nil
	}
	if errors.Is(err, ErrTrainingInefficient) {
		return "training_inefficient_area", nil
	}
	if errors.Is(err, skill.ErrTrainRetreat) {
		return "train_retreat", nil
	}
	if errors.Is(err, skill.ErrTrainProgress) {
		return "train_progress_shortfall", nil
	}
	if errors.Is(err, skill.ErrCantAfford) {
		return "cant_afford", nil
	}
	if errors.Is(err, skill.ErrNotInStock) {
		return "not_in_stock", nil
	}
	if errors.Is(err, skill.ErrBagNotRisen) {
		return "bag_not_risen", nil
	}
	if errors.Is(err, skill.ErrFieldRosterNoBalls) {
		return "no_pokeball", nil
	}
	if errors.Is(err, world.ErrNoPath) {
		return "no_path", nil
	}
	if errors.Is(err, world.ErrNoRoute) {
		return "no_route", nil
	}
	if errors.Is(err, skill.ErrLegUnwalkable) {
		return "leg_unwalkable", nil
	}
	if errors.Is(err, skill.ErrNoDialogue) {
		return "no_dialogue", nil
	}
	if errors.Is(err, skill.ErrDialogueInterrupted) {
		return "dialogue_interrupted", nil
	}
	if errors.Is(err, skill.ErrFieldMovePrerequisite) {
		return "field_move_prerequisite_missing", nil
	}
	var gate *skill.ErrRouteGateClosed
	if errors.As(err, &gate) {
		return "route_gate_closed", nil
	}
	var blocked *skill.ErrBlocked
	if errors.As(err, &blocked) {
		return "blocked_step", nil
	}
	if t := firstSpecificErrorType(err); t != "" {
		return FailureCauseID("type:" + t), nil
	}
	return "unknown_error", nil
}

// firstSpecificErrorType is the prose-free fallback for a failure not yet in
// the typed cause vocabulary. Standard wrapping/join/errorString
// implementations carry no stable semantic identity and are skipped.
func firstSpecificErrorType(err error) string {
	if err == nil {
		return ""
	}
	t := reflect.TypeOf(err)
	if t != nil {
		name := t.String()
		if name != "*fmt.wrapError" && name != "*errors.joinError" && name != "*errors.errorString" && name != "errors.errorString" {
			return name
		}
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range multi.Unwrap() {
			if name := firstSpecificErrorType(child); name != "" {
				return name
			}
		}
	}
	if one, ok := err.(interface{ Unwrap() error }); ok {
		return firstSpecificErrorType(one.Unwrap())
	}
	return ""
}
