package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestValidateRedTalkObjectiveRejectsMagikarpSalesman(t *testing.T) {
	o := Objective{Kind: KindTalk, X: mtMoonMagikarpSalesmanHomeX, Y: mtMoonMagikarpSalesmanHomeY}
	err := validateRedTalkObjective(nil, mtMoonPokecenterMapID, o)
	if err == nil {
		t.Fatal("Magikarp salesman generic Talk was accepted")
	}
	if !errors.Is(err, skill.ErrNoDialogue) {
		t.Fatalf("Magikarp salesman guard error = %v, want recoverable blocked classification", err)
	}
}

func TestValidateRedTalkObjectiveRejectsChoiceRewardActor(t *testing.T) {
	o := Objective{Kind: KindTalk, X: 2, Y: 4}
	err := validateRedTalkObjective(nil, 0xA3, o) // Vermilion Old Rod guru
	if err == nil {
		t.Fatal("Old Rod guru generic Talk was accepted")
	}
	if !errors.Is(err, skill.ErrNoDialogue) {
		t.Fatalf("Old Rod guru guard error = %v, want recoverable blocked classification", err)
	}
}

func TestValidateRedTalkObjectiveAllowsOrdinaryNPC(t *testing.T) {
	o := Objective{Kind: KindTalk, X: 7, Y: 3} // ordinary gentleman in Mt. Moon Center
	if err := validateRedTalkObjective(nil, mtMoonPokecenterMapID, o); err != nil {
		t.Fatalf("ordinary NPC generic Talk rejected: %v", err)
	}
}

func TestValidateRedTalkObjectiveIgnoresNonTalkObjective(t *testing.T) {
	o := Objective{Kind: KindGoTo, X: mtMoonMagikarpSalesmanHomeX, Y: mtMoonMagikarpSalesmanHomeY}
	if err := validateRedTalkObjective(nil, mtMoonPokecenterMapID, o); err != nil {
		t.Fatalf("non-Talk objective rejected by Talk guard: %v", err)
	}
}
