package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TestYellowRivalBattle is the first real battle on Yellow, fought through the
// skill layer's own Battle() with the stat-aware policy. It loads the settled
// post-starter fixture (the player has Pikachu; the lab sequence is one step
// from the rival challenge), drives the few scripted inputs that gate the
// fight, and then hands control to Battle().
//
// It exists because a battle is the one subsystem that a decode test cannot
// cover: Battle() must read the Yellow battle block (enemy species, HP, the
// move list) to choose and observe moves, and a Red-addressed read of that
// block on Yellow RAM picks up plausible-looking garbage. The positive
// postcondition is a win, decoded from Yellow's battle result address.
//
// The pre-battle gating is scripted dialogue, not pathfinding: Oak's tutorial
// chain pages with A, and the rival challenge fires when the player steps to
// y=6. Battle() owns everything from the first real battle input.
func TestYellowRivalBattle(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("pokemon_yellow.gb not available")
	}
	st, err := os.ReadFile("failure/yellow_starter.state")
	if err != nil {
		t.Skip("yellow_starter.state not available")
	}
	e, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.LoadState(st); err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	a := AddressesForROM(romData)

	// Page Oak's post-starter tutorial chain until input returns.
	for i := 0; i < 200; i++ {
		e.Tap(emu.A, 3, 7)
		e.StepFrames(12)
		state.Snapshot(e, &mem)
		if a.Controllable(&mem) && e.Peek8(0xCD6B) == 0 {
			break
		}
	}

	// Walk south to y=6; the rival challenge takes over from there.
	for i := 0; i < 30; i++ {
		state.Snapshot(e, &mem)
		if !a.Controllable(&mem) {
			break
		}
		e.Tap(emu.Down, 4, 16)
		e.StepFrames(24)
	}

	// Page the challenge text; the battle starts on its own.
	for i := 0; i < 120; i++ {
		e.Tap(emu.A, 3, 7)
		e.StepFrames(12)
		state.Snapshot(e, &mem)
		if a.Decode(&mem).Battle != nil {
			break
		}
	}
	state.Snapshot(e, &mem)
	if a.Decode(&mem).Battle == nil {
		t.Fatal("rival battle did not start")
	}

	// Fight it through the skill layer.
	res, err := Battle(e, StatAwareMove(romData))
	if err != nil {
		t.Fatalf("Battle on Yellow: %v", err)
	}
	if res != state.ResultWon {
		t.Fatalf("rival battle result = %d, want %d (ResultWon)", res, state.ResultWon)
	}
	t.Logf("rival battle won on Yellow")
}
