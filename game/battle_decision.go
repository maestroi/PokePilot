package game

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A battle decision is one turn's choice at the battle's actionable boundary
// (the main battle menu). The state is portable: it names species, moves,
// types and items by semantic id, and carries only what a fast System-1
// backend needs for the turn. Legality is decided by the game adapter that
// builds the state; the action set is derived from it deterministically, so a
// backend can only ever pick from what the adapter declared legal. Executing
// the chosen action (menu input, verification) stays with the adapter's
// battle skill.

var (
	ErrInvalidBattleState  = errors.New("game: invalid battle decision state")
	ErrIllegalBattleAction = errors.New("game: illegal battle action")
)

// MaxBattleRecent bounds the optional recent-context lines so a turn request
// stays compact enough for a fast decision call.
const MaxBattleRecent = 4

type BattleContext string

const (
	BattleContextWild    BattleContext = "wild"
	BattleContextTrainer BattleContext = "trainer"
)

type BattleActionKind string

const (
	BattleActionMove   BattleActionKind = "move"
	BattleActionSwitch BattleActionKind = "switch"
	BattleActionItem   BattleActionKind = "item"
	BattleActionRun    BattleActionKind = "run"
)

// Reasons an adapter reports for a visible option that is not legal this
// turn. They are shown to the backend as context but never become actions.
const (
	BattleUnusableNoPP     = "no_pp"
	BattleUnusableDisabled = "disabled"
	BattleUnusableFainted  = "fainted"
	BattleUnusableActive   = "active"
)

// BattleAction is one semantic turn action. Slot is a move slot for move and
// the party slot for switch and item targets.
type BattleAction struct {
	Kind BattleActionKind `json:"kind"`
	Slot int              `json:"slot,omitempty"`
	Item ItemID           `json:"item,omitempty"`
}

// ID is the stable wire form: move:<slot>, switch:<slot>,
// item:<item>:<party-slot>, or run.
func (a BattleAction) ID() string {
	switch a.Kind {
	case BattleActionMove, BattleActionSwitch:
		return fmt.Sprintf("%s:%d", a.Kind, a.Slot)
	case BattleActionItem:
		return fmt.Sprintf("item:%s:%d", CanonicalID(string(a.Item)), a.Slot)
	case BattleActionRun:
		return "run"
	default:
		return ""
	}
}

// ParseBattleAction decodes an action id. Parsing proves only that the id is
// well formed; BattleDecisionState.Legal decides whether it may execute.
func ParseBattleAction(id string) (BattleAction, error) {
	id = strings.TrimSpace(id)
	if id == "run" {
		return BattleAction{Kind: BattleActionRun}, nil
	}
	parts := strings.Split(id, ":")
	slot := func(s string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%w: bad slot in %q", ErrIllegalBattleAction, id)
		}
		return n, nil
	}
	switch {
	case len(parts) == 2 && (parts[0] == string(BattleActionMove) || parts[0] == string(BattleActionSwitch)):
		n, err := slot(parts[1])
		if err != nil {
			return BattleAction{}, err
		}
		return BattleAction{Kind: BattleActionKind(parts[0]), Slot: n}, nil
	case len(parts) == 3 && parts[0] == string(BattleActionItem) && strings.TrimSpace(parts[1]) != "":
		n, err := slot(parts[2])
		if err != nil {
			return BattleAction{}, err
		}
		return BattleAction{Kind: BattleActionItem, Item: ItemID(CanonicalID(parts[1])), Slot: n}, nil
	}
	return BattleAction{}, fmt.Errorf("%w: unrecognized action %q", ErrIllegalBattleAction, id)
}

// BattleMon is the compact view of one combatant or bench member.
type BattleMon struct {
	Species SpeciesID `json:"species"`
	Level   int       `json:"level"`
	HP      int       `json:"hp"`
	MaxHP   int       `json:"max_hp"`
	Status  string    `json:"status,omitempty"`
	Types   []string  `json:"types,omitempty"`
}

// BattleMoveOption is one of the active mon's known moves. Metadata fields
// are omitted when the adapter cannot measure them; Effectiveness is the
// damage multiplier against the current opponent (0 when unknown).
type BattleMoveOption struct {
	Slot          int     `json:"slot"`
	Move          MoveID  `json:"move"`
	Type          string  `json:"type,omitempty"`
	Power         int     `json:"power,omitempty"`
	Accuracy      int     `json:"accuracy,omitempty"`
	PP            int     `json:"pp"`
	Effectiveness float64 `json:"effectiveness,omitempty"`
	Unusable      string  `json:"unusable,omitempty"`
}

type BattleSwitchOption struct {
	Slot int `json:"slot"`
	BattleMon
	Unusable string `json:"unusable,omitempty"`
}

// BattleItemOption is a legal item use: the adapter lists only items the
// caller permitted, the bag holds, and that would have an effect on Target.
type BattleItemOption struct {
	Item     ItemID `json:"item"`
	Target   int    `json:"target"`
	Quantity int    `json:"quantity"`
}

type BattleDecisionState struct {
	Context    BattleContext        `json:"context"`
	ActiveSlot int                  `json:"active_slot"`
	Active     BattleMon            `json:"active"`
	Opponent   BattleMon            `json:"opponent"`
	Moves      []BattleMoveOption   `json:"moves"`
	Switches   []BattleSwitchOption `json:"switches,omitempty"`
	Items      []BattleItemOption   `json:"items,omitempty"`
	CanRun     bool                 `json:"can_run"`
	Recent     []string             `json:"recent,omitempty"`
}

// Actions derives the complete legal action set from the adapter-declared
// options, in a stable order: moves, switches, items, run.
func (s BattleDecisionState) Actions() []BattleAction {
	var out []BattleAction
	for _, mv := range s.Moves {
		if mv.Unusable == "" {
			out = append(out, BattleAction{Kind: BattleActionMove, Slot: mv.Slot})
		}
	}
	for _, sw := range s.Switches {
		if sw.Unusable == "" {
			out = append(out, BattleAction{Kind: BattleActionSwitch, Slot: sw.Slot})
		}
	}
	for _, it := range s.Items {
		out = append(out, BattleAction{Kind: BattleActionItem, Item: ItemID(CanonicalID(string(it.Item))), Slot: it.Target})
	}
	if s.CanRun {
		out = append(out, BattleAction{Kind: BattleActionRun})
	}
	return out
}

// MoveOnly narrows the state to the existing move-slot policy seam: the same
// turn with switching, items and RUN withheld.
func (s BattleDecisionState) MoveOnly() BattleDecisionState {
	s.Switches, s.Items, s.CanRun = nil, nil, false
	return s
}

// Validate rejects contradictory or unbounded states before any backend sees
// them. A state with no legal action is not an actionable decision.
func (s BattleDecisionState) Validate() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidBattleState, fmt.Sprintf(format, args...))
	}
	switch s.Context {
	case BattleContextWild, BattleContextTrainer:
	default:
		return bad("unknown context %q", s.Context)
	}
	if s.Context == BattleContextTrainer && s.CanRun {
		return bad("run declared legal in a trainer battle")
	}
	if len(s.Recent) > MaxBattleRecent {
		return bad("%d recent entries, max %d", len(s.Recent), MaxBattleRecent)
	}
	moves := map[int]bool{}
	for _, mv := range s.Moves {
		if mv.Slot < 0 || moves[mv.Slot] {
			return bad("duplicate or negative move slot %d", mv.Slot)
		}
		moves[mv.Slot] = true
		if mv.Unusable == "" && mv.PP <= 0 {
			return bad("move slot %d declared usable with %d PP", mv.Slot, mv.PP)
		}
	}
	if s.ActiveSlot < 0 {
		return bad("negative active slot %d", s.ActiveSlot)
	}
	party := map[int]bool{s.ActiveSlot: true}
	listed := map[int]bool{}
	for _, sw := range s.Switches {
		if sw.Slot < 0 || listed[sw.Slot] {
			return bad("duplicate or negative switch slot %d", sw.Slot)
		}
		listed[sw.Slot] = true
		if sw.Slot == s.ActiveSlot && sw.Unusable == "" {
			return bad("switch to the active slot %d declared legal", sw.Slot)
		}
		if sw.HP <= 0 && sw.Unusable == "" {
			return bad("switch to fainted slot %d declared legal", sw.Slot)
		}
		party[sw.Slot] = true
	}
	for _, it := range s.Items {
		if CanonicalID(string(it.Item)) == "" || strings.Contains(string(it.Item), ":") {
			return bad("item id %q", it.Item)
		}
		if it.Quantity <= 0 {
			return bad("item %q declared with quantity %d", it.Item, it.Quantity)
		}
		if !party[it.Target] {
			return bad("item %q targets unknown party slot %d", it.Item, it.Target)
		}
	}
	actions := s.Actions()
	if len(actions) == 0 {
		return bad("no legal action")
	}
	seen := make(map[string]bool, len(actions))
	for _, a := range actions {
		if seen[a.ID()] {
			return bad("duplicate action %s", a.ID())
		}
		seen[a.ID()] = true
	}
	return nil
}

// Legal resolves a backend-selected id against this turn's legal set. It is
// the final gate before execution: well-formed but undeclared actions are
// rejected with ErrIllegalBattleAction.
func (s BattleDecisionState) Legal(id string) (BattleAction, error) {
	action, err := ParseBattleAction(id)
	if err != nil {
		return BattleAction{}, err
	}
	for _, legal := range s.Actions() {
		if legal.ID() == action.ID() {
			return legal, nil
		}
	}
	return BattleAction{}, fmt.Errorf("%w: %q is not in this turn's legal set", ErrIllegalBattleAction, id)
}
