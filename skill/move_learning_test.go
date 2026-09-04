package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
)

const (
	learnTypeNormal uint8 = 0x00
	learnTypePoison uint8 = 0x03
	learnTypeGrass  uint8 = 0x16
)

func TestNaturalMoveLearningIvysaurKeepsTackleCoverage(t *testing.T) {
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	poisonPowder := rom.Move{ID: 77, Effect: 0x42, Type: learnTypePoison, Accuracy: 191, PP: 35}
	tackleMove := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	romData := fakeROM(t, tackleMove, growl, leechSeed, vineWhip, poisonPowder)

	current := [4]uint8{tackleMove.ID, growl.ID, leechSeed.ID, vineWhip.ID}
	d := DecideNaturalMove(romData, learnTypeGrass, learnTypePoison, current, poisonPowder.ID)
	if !d.Learn {
		t.Fatalf("PoisonPowder was declined: %+v", d)
	}
	if d.ReplaceSlot != 1 {
		t.Fatalf("replacement slot = %d, want 1 (GROWL); decision=%+v", d.ReplaceSlot, d)
	}
	if current[d.ReplaceSlot] == tackleMove.ID {
		t.Fatalf("strategic learning discarded TACKLE coverage: %+v", d)
	}
}

func TestNaturalMoveLearningDeclinesBadMove(t *testing.T) {
	tackleMove := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	poisonPowder := rom.Move{ID: 77, Effect: 0x42, Type: learnTypePoison, Accuracy: 191, PP: 35}
	splash := rom.Move{ID: 150, Effect: 0x55, Accuracy: 255, PP: 40}
	romData := fakeROM(t, tackleMove, vineWhip, leechSeed, poisonPowder, splash)

	d := DecideNaturalMove(romData, learnTypeGrass, learnTypePoison,
		[4]uint8{tackleMove.ID, vineWhip.ID, leechSeed.ID, poisonPowder.ID}, splash.ID)
	if d.Learn || d.ReplaceSlot != -1 {
		t.Fatalf("SPLASH should be declined, got %+v", d)
	}
}

func TestNaturalMoveLearningNeverReplacesHM(t *testing.T) {
	cut := rom.Move{ID: 15, Power: 50, Type: learnTypeNormal, Accuracy: 242, PP: 30}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	razorLeaf := rom.Move{ID: 75, Power: 55, Type: learnTypeGrass, Accuracy: 242, PP: 25}
	romData := fakeROM(t, cut, growl, leechSeed, vineWhip, razorLeaf)

	d := DecideNaturalMove(romData, learnTypeGrass, learnTypePoison,
		[4]uint8{cut.ID, growl.ID, leechSeed.ID, vineWhip.ID}, razorLeaf.ID)
	if !d.Learn {
		t.Fatalf("RAZOR LEAF should improve this set: %+v", d)
	}
	if d.ReplaceSlot == 0 {
		t.Fatalf("HM CUT was selected for replacement: %+v", d)
	}
}

func TestNaturalMoveLearningTreatsFixedDamageAsAttack(t *testing.T) {
	seismicToss := rom.Move{ID: 69, Effect: rom.SpecialDamageEffect, Accuracy: 255, PP: 20}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	poisonPowder := rom.Move{ID: 77, Effect: 0x42, Type: learnTypePoison, Accuracy: 191, PP: 35}
	splash := rom.Move{ID: 150, Effect: 0x55, Accuracy: 255, PP: 40}
	romData := fakeROM(t, seismicToss, growl, leechSeed, poisonPowder, splash)

	d := DecideNaturalMove(romData, learnTypePoison, learnTypePoison,
		[4]uint8{seismicToss.ID, growl.ID, leechSeed.ID, poisonPowder.ID}, splash.ID)
	if d.Learn && d.ReplaceSlot == 0 {
		t.Fatalf("fixed-damage SEISMIC TOSS was treated like expendable status: %+v", d)
	}
}

func TestNaturalMoveLearningIsDeterministic(t *testing.T) {
	a := rom.Move{ID: 33, Power: 35, Type: learnTypeNormal, Accuracy: 255, PP: 35}
	b := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	c := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	dmove := rom.Move{ID: 22, Power: 35, Type: learnTypeGrass, Accuracy: 255, PP: 10}
	offered := rom.Move{ID: 77, Effect: 0x42, Type: learnTypePoison, Accuracy: 191, PP: 35}
	romData := fakeROM(t, a, b, c, dmove, offered)
	current := [4]uint8{a.ID, b.ID, c.ID, dmove.ID}

	first := DecideNaturalMove(romData, learnTypeGrass, learnTypePoison, current, offered.ID)
	for i := 0; i < 20; i++ {
		if got := DecideNaturalMove(romData, learnTypeGrass, learnTypePoison, current, offered.ID); got != first {
			t.Fatalf("decision changed: first=%+v got=%+v", first, got)
		}
	}
}
