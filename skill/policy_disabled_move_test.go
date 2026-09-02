package skill

import "testing"

func TestStatAwareMoveSkipsDisabledStrongestMove(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, ember))
	b := battleWith(7, 7, 20, 20, tackle.ID, ember.ID)

	// Ember is stronger and normally wins this choice, but game slot 2 is
	// disabled. Policies consume BattleState.Usable(), so they must fall back
	// to Tackle instead of selecting a move the ROM will bounce back to the
	// fight menu forever.
	b.DisabledMove = 2
	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d, want 0 (TACKLE): slot 1 (EMBER) is disabled", got)
	}
}

func TestStatAwareMoveUsesMoveAgainWhenDisableClears(t *testing.T) {
	p := StatAwareMove(fakeROM(t, tackle, ember))
	b := battleWith(7, 7, 20, 20, tackle.ID, ember.ID)
	b.DisabledMove = 2
	if got := p(b); got != 0 {
		t.Fatalf("policy chose slot %d while EMBER is disabled, want 0", got)
	}

	b.DisabledMove = 0
	if got := p(b); got != 1 {
		t.Fatalf("policy chose slot %d after disable cleared, want 1 (EMBER)", got)
	}
}
