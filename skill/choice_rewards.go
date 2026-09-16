package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// ChoiceReward describes a Red NPC whose YES/NO interaction grants one
// deterministic item. These are gameplay transactions, not ordinary dialogue:
// generic Talk must never guess the answer, while Pickup can own the item
// postcondition just like it does for an overworld item ball.
type ChoiceReward struct {
	Place    string
	Map      uint8
	X, Y     uint8
	StandX   uint8
	StandY   uint8
	Item     uint8
	ItemName string
	MinOwned int
}

var choiceRewards = []ChoiceReward{
	{Place: "vermilion old rod house", Map: 0xA3, X: 2, Y: 4, StandX: 2, StandY: 5, Item: 0x4C, ItemName: "old rod"},
	{Place: "fuchsia good rod house", Map: 0xA4, X: 5, Y: 3, StandX: 6, StandY: 3, Item: 0x4D, ItemName: "good rod"},
	{Place: "route 12 super rod house", Map: 0xBD, X: 2, Y: 4, StandX: 3, StandY: 4, Item: 0x4E, ItemName: "super rod"},
	{Place: "route 2 oak aide", Map: 0x31, X: 1, Y: 4, StandX: 2, StandY: 4, Item: 0xC8, ItemName: "hm05", MinOwned: 10},
	{Place: "route 11 oak aide", Map: 0x56, X: 2, Y: 6, StandX: 2, StandY: 5, Item: 0x47, ItemName: "itemfinder", MinOwned: 30},
	{Place: "route 15 oak aide", Map: 0xB9, X: 4, Y: 2, StandX: 4, StandY: 3, Item: 0x4B, ItemName: "exp all", MinOwned: 50},
}

func init() {
	// These destinations are interaction-owned. Place resolves them so a
	// semantic reward objective can route there, but PlaceNames does not expose
	// them as generic exploration targets that stop before consuming the reward.
	for _, reward := range choiceRewards {
		interactionPlaces[reward.Place] = Destination{
			Map: reward.Map, X: reward.StandX, Y: reward.StandY,
		}
	}
}

// ChoiceRewards returns a copy so the Red agent adapter can offer the matching
// semantic item objectives without maintaining a second coordinate/item table.
func ChoiceRewards() []ChoiceReward {
	out := make([]ChoiceReward, len(choiceRewards))
	copy(out, choiceRewards)
	return out
}

// IsChoiceRewardActor reports whether a person-shaped map object is one of the
// scripted reward actors above. Generic KindTalk should suppress these actors.
func IsChoiceRewardActor(mapID, x, y uint8) bool {
	for _, reward := range choiceRewards {
		if reward.Map == mapID && reward.X == x && reward.Y == y {
			return true
		}
	}
	return false
}

func choiceRewardAt(mapID, x, y, item uint8) (ChoiceReward, bool) {
	for _, reward := range choiceRewards {
		if reward.Map == mapID && reward.X == x && reward.Y == y && reward.Item == item {
			return reward, true
		}
	}
	return ChoiceReward{}, false
}

// TalkAtChoice performs one explicitly-owned two-option choice during a talk.
// It is deliberately separate from Talk/TalkAt: those generic primitives must
// continue refusing to answer arbitrary gameplay choices.
func TalkAtChoice(m *emu.Emu, romData []byte, x, y uint8, choice int, policy MovePolicy) (int, error) {
	presses, err := TalkAt(m, romData, x, y, policy)
	if err == nil {
		// Some one-time scripts become ordinary dialogue after their event is
		// complete. Let the caller's semantic postcondition decide whether that
		// is success; do not fabricate another choice.
		return presses, nil
	}
	var menuErr *ErrTalkMenu
	if !errors.As(err, &menuErr) {
		return presses, err
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	menu := state.DecodeTwoOptionMenu(&mem)
	if menu == nil {
		return presses, fmt.Errorf("skill: TalkAtChoice: interaction opened a non-choice menu: %w", err)
	}
	if choice < 0 || choice > 1 {
		return presses, fmt.Errorf("skill: TalkAtChoice: choice index %d out of range", choice)
	}
	if err := selectTwoOption(m, choice); err != nil {
		return presses, fmt.Errorf("skill: TalkAtChoice: select option %d: %w", choice, err)
	}

	rec := RecoverDialogue(m, dialogueRecoveryBudget)
	switch rec.Stop {
	case DialogueRecovered:
		return presses, nil
	case DialogueChoiceRequired:
		return presses, fmt.Errorf("skill: TalkAtChoice: owned choice led to another unanswered choice: %q", rec.Text)
	case DialogueMenuOpen:
		return presses, fmt.Errorf("skill: TalkAtChoice: owned choice led to a menu: %q", rec.Text)
	case DialogueBudgetExhausted:
		return presses, fmt.Errorf("skill: TalkAtChoice: post-choice dialogue did not settle: %q", rec.Text)
	case DialogueUnexpectedMode:
		return presses, fmt.Errorf("skill: TalkAtChoice: interaction unexpectedly entered battle")
	default:
		return presses, fmt.Errorf("skill: TalkAtChoice: recovery stopped with %d", rec.Stop)
	}
}

func receiveChoiceReward(m *emu.Emu, romData []byte, reward ChoiceReward, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	before := bagCount(state.DecodeInventory(&mem).Items, reward.Item)

	if err := EnsureBagSpaceFor(m, reward.Item); err != nil {
		return fmt.Errorf("skill: choice reward %s: make bag space: %w", reward.ItemName, err)
	}
	if _, err := TalkAtChoice(m, romData, reward.X, reward.Y, 0, policy); err != nil {
		return fmt.Errorf("skill: choice reward %s: %w", reward.ItemName, err)
	}

	state.Snapshot(m, &mem)
	after := bagCount(state.DecodeInventory(&mem).Items, reward.Item)
	if after != before+1 {
		return fmt.Errorf("%w: choice reward %s item %d was %d before and %d after", ErrBagNotRisen, reward.ItemName, reward.Item, before, after)
	}
	return nil
}
