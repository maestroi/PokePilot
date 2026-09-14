package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestGenericTalkDeclinableYesNoAcceptsBothLayouts(t *testing.T) {
	cases := []state.InteractionState{
		{Kind: state.InteractionTwoOption, Options: [2]string{"YES", "NO"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"NO", "YES"}},
	}
	for _, interaction := range cases {
		if !genericTalkDeclinableYesNo(interaction) {
			t.Fatalf("expected YES/NO interaction to be safely declinable: %+v", interaction)
		}
	}
}

func TestGenericTalkDeclinableYesNoRejectsOtherInteractions(t *testing.T) {
	cases := []state.InteractionState{
		{Kind: state.InteractionTwoOption, Options: [2]string{"TRADE", "CANCEL"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"HEAL", "CANCEL"}},
		{Kind: state.InteractionTwoOption, Options: [2]string{"NORTH", "WEST"}},
		{Kind: state.InteractionMenu, Options: [2]string{"YES", "NO"}},
		{Kind: state.InteractionDialogue},
	}
	for _, interaction := range cases {
		if genericTalkDeclinableYesNo(interaction) {
			t.Fatalf("unexpected interaction was classified as safely declinable: %+v", interaction)
		}
	}
}
