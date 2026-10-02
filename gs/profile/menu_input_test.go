package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

// TestGoldSilverDeclareASingleStepMenuPressHold pins the measured input window
// of the retail Gold/Silver menus. A press shorter than the input pipeline is
// swallowed silently, so the declared hold has to clear that floor; and the
// menus repeat a held direction shortly after, so it also has to stay under the
// point where one press would double-step a cursor or skip a pocket.
//
// Measured on retail Gold from the run-1p7ixxdreiam630odlwlg1xf35 checkpoint:
// the START-menu cursor and the PACK pocket switch both drop a 3-frame press,
// step exactly once for 4..19 frames, and the PACK switches twice by 24.
func TestGoldSilverDeclareASingleStepMenuPressHold(t *testing.T) {
	const (
		droppedBelow = 4  // 3-frame presses are swallowed
		repeatsAt    = 20 // a 20-frame hold starts double-stepping
	)
	for _, tc := range []struct {
		name string
		p    *Profile
	}{{"gold", NewGold()}, {"silver", NewSilver()}} {
		t.Run(tc.name, func(t *testing.T) {
			timing, ok := game.GameProfile(tc.p).(game.MenuPressTiming)
			if !ok {
				t.Fatalf("%s profile does not declare a menu press hold", tc.name)
			}
			hold := timing.MenuPressHoldFrames()
			if hold < droppedBelow {
				t.Fatalf("declared hold %d is below the %d-frame floor; short presses are dropped", hold, droppedBelow)
			}
			if hold >= repeatsAt {
				t.Fatalf("declared hold %d reaches the %d-frame repeat window and can double-step", hold, repeatsAt)
			}
		})
	}
}
