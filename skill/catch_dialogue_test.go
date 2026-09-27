package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestCatchDialogueResolution(t *testing.T) {
	tests := []struct {
		name       string
		result     DialogueRecoveryResult
		wantBattle bool
		wantChoice bool
		wantErr    string
	}{
		{name: "ordinary text recovered", result: DialogueRecoveryResult{Stop: DialogueRecovered}},
		{name: "trainer text entered battle", result: DialogueRecoveryResult{Stop: DialogueUnexpectedMode}, wantBattle: true},
		{name: "choice remains owned by caller", result: DialogueRecoveryResult{Stop: DialogueChoiceRequired, Text: "YES NO"}, wantChoice: true},
		{name: "menu remains owned by caller", result: DialogueRecoveryResult{Stop: DialogueMenuOpen}, wantChoice: true},
		{name: "stuck text is explicit", result: DialogueRecoveryResult{Stop: DialogueBudgetExhausted, Text: "HELLO"}, wantErr: "did not clear"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			battle, err := catchDialogueResolution(tt.result)
			if battle != tt.wantBattle {
				t.Fatalf("battle = %v, want %v", battle, tt.wantBattle)
			}
			var choice *ErrDialogueChoice
			if errors.As(err, &choice) != tt.wantChoice {
				t.Fatalf("choice error = %v, want %v (err=%v)", errors.As(err, &choice), tt.wantChoice, err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
			if tt.wantErr == "" && !tt.wantChoice && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
