package rom

import (
	"testing"

	redrom "github.com/maestroi/pokepilot/red/rom"
)

// Yellow's Viridian Forest Bug Catcher set 1 is L7 (pokeyellow parties.asm).
func TestRealYellowTrainerPrizeMoney(t *testing.T) {
	data := loadRealYellowROM(t)
	if got, err := redrom.TrainerPrizeMoney(data, 2, 1); err != nil || got != 10*7 {
		t.Fatalf("Yellow Bug Catcher set 1 prize = %d, %v; want 70", got, err)
	}
}
