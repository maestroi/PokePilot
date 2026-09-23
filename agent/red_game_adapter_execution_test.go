package agent

import (
	"errors"
	"testing"
)

func TestNormalizeRedOwnedExecutionResultMenuFallback(t *testing.T) {
	controllerErr := errors.New("item-use menu did not appear")

	tests := []struct {
		name   string
		obj    Objective
		result ObjectiveResult
		want   Outcome
	}{
		{
			name: "unclassified use item becomes blocked",
			obj:  Objective{Kind: KindUseItem, Item: ItemID("potion"), Slot: 0},
			want: OutcomeBlocked,
		},
		{
			name:   "specific use item outcome is preserved",
			obj:    Objective{Kind: KindUseItem, Item: ItemID("potion"), Slot: 0},
			result: ObjectiveResult{Outcome: OutcomePostconditionFailed},
			want:   OutcomePostconditionFailed,
		},
		{
			name: "unclassified buy becomes blocked",
			obj:  Objective{Kind: KindBuy, Item: ItemID("water stone"), Qty: 1},
			want: OutcomeBlocked,
		},
		{
			name:   "specific buy outcome is preserved",
			obj:    Objective{Kind: KindBuy, Item: ItemID("water stone"), Qty: 1},
			result: ObjectiveResult{Outcome: OutcomeStabilizationFailed},
			want:   OutcomeStabilizationFailed,
		},
		{
			name: "other objective kinds are not reclassified",
			obj:  Objective{Kind: KindGoTo, Place: PlaceID("route 1")},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRedOwnedExecutionResult(tt.obj, tt.result, controllerErr)
			if !errors.Is(err, controllerErr) {
				t.Fatalf("error = %v, want original controller error", err)
			}
			if got.Outcome != tt.want {
				t.Fatalf("Outcome = %q, want %q", got.Outcome, tt.want)
			}
		})
	}
}

func TestNormalizeRedOwnedExecutionResultSuccessUnchanged(t *testing.T) {
	original := ObjectiveResult{Outcome: OutcomeCompleted}
	got, err := normalizeRedOwnedExecutionResult(
		Objective{Kind: KindUseItem, Item: ItemID("potion"), Slot: 0},
		original,
		nil,
	)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if got.Outcome != OutcomeCompleted {
		t.Fatalf("Outcome = %q, want completed", got.Outcome)
	}
}
