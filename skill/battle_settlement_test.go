package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// settleRuntimeDecoder is a scripted BattleRuntimeDecoder. Settling asks the
// runtime the same question every pass, so the sequence is consumed one
// decode per pass and the last entry repeats.
type settleRuntimeDecoder struct {
	states  []game.BattleRuntimeState
	decodes int
}

func (d *settleRuntimeDecoder) DecodeBattleRuntime(game.MemoryReader) game.BattleRuntimeState {
	if len(d.states) == 0 {
		return game.BattleRuntimeState{}
	}
	i := d.decodes
	d.decodes++
	if i >= len(d.states) {
		i = len(d.states) - 1
	}
	return d.states[i]
}

// syntheticSettleROM is a minimal cartridge. Settlement only needs frames to
// advance; the semantics under test come from the decoder.
func syntheticSettleROM() []byte {
	rom := make([]byte, 0x8000)
	rom[0x100], rom[0x101], rom[0x102], rom[0x103] = 0x00, 0xC3, 0x00, 0x01
	return rom
}

func settleTestEmu(t *testing.T) *emu.Emu {
	t.Helper()
	m, err := emu.OpenCGBBytes(syntheticSettleROM())
	if err != nil {
		t.Fatalf("OpenCGBBytes: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// A scripted map can queue a second trainer battle before the first one's
// aftermath settles (Mt. Moon B2F queues Jessie, then James). Settling only
// steps frames, so it cannot play that battle: it must hand control back to
// the battle loop instead of spending the settle budget and then reporting
// the objective boundary dirty (run-acduyt1qbev9c, issue #2189).
func TestSettleAfterBattleReportsARestartedBattle(t *testing.T) {
	m := settleTestEmu(t)
	// TextActive keeps each pass to a short tap instead of a bare frame.
	decoder := &settleRuntimeDecoder{states: []game.BattleRuntimeState{
		{InBattle: false, Controllable: false, TextActive: true},
		{InBattle: true, Controllable: false, TextActive: true},
	}}
	start := m.FrameCount()

	err := settleAfterBattle(m, decoder)
	if !errors.Is(err, errBattleRestarted) {
		t.Fatalf("settleAfterBattle = %v, want errBattleRestarted", err)
	}
	if elapsed := int(m.FrameCount() - start); elapsed >= settleBudget {
		t.Fatalf("settlement consumed %d frames (budget %d); it waited out the second battle instead of reporting it",
			elapsed, settleBudget)
	}
}

// The ordinary case is unchanged: an aftermath whose control returns settles
// clean, and "stabilized" still means settleStableFrames consecutive
// controllable passes.
func TestSettleAfterBattleReturnsOnStableControl(t *testing.T) {
	m := settleTestEmu(t)
	decoder := &settleRuntimeDecoder{states: []game.BattleRuntimeState{
		{InBattle: false, Controllable: true, TextActive: false},
	}}

	if err := settleAfterBattle(m, decoder); err != nil {
		t.Fatalf("settleAfterBattle = %v, want nil", err)
	}
	if decoder.decodes < settleStableFrames {
		t.Fatalf("settled after %d decodes, want at least %d stable passes", decoder.decodes, settleStableFrames)
	}
}

// Long post-battle map scripts keep TextActive past the idle settle budget
// (Rocket B2F executive + Lance follow). Each TextActive pass taps A (~10
// frames), so a few dozen scripted passes already outlast settleBudget;
// settlement must keep driving instead of timing out (run-a018a6cmhnkr /
// triage:b815bee7497dfea5).
func TestSettleAfterBattleExtendsBudgetWhileScriptOwnsMachine(t *testing.T) {
	m := settleTestEmu(t)
	// Tap(A,3,7) advances 10 frames; exceed settleBudget with script ownership.
	scriptPasses := settleBudget/10 + 3
	states := make([]game.BattleRuntimeState, 0, scriptPasses+settleStableFrames)
	for i := 0; i < scriptPasses; i++ {
		states = append(states, game.BattleRuntimeState{InBattle: false, Controllable: false, TextActive: true})
	}
	for i := 0; i < settleStableFrames; i++ {
		states = append(states, game.BattleRuntimeState{InBattle: false, Controllable: true, TextActive: false})
	}
	decoder := &settleRuntimeDecoder{states: states}
	start := m.FrameCount()

	if err := settleAfterBattle(m, decoder); err != nil {
		t.Fatalf("settleAfterBattle = %v, want nil after a long scripted aftermath", err)
	}
	if elapsed := int(m.FrameCount() - start); elapsed <= settleBudget {
		t.Fatalf("settled in %d frames; expected the scripted aftermath to outlast settleBudget=%d", elapsed, settleBudget)
	}
}

// Control can return on the last frames of the idle budget without enough
// remaining iterations for settleStableFrames. Settlement must finish proving
// stability instead of claiming the overworld is not controllable.
func TestSettleAfterBattleGraceWhenControlReturnsAtDeadline(t *testing.T) {
	m := settleTestEmu(t)
	states := make([]game.BattleRuntimeState, 0, settleBudget+settleStableFrames)
	for i := 0; i < settleBudget; i++ {
		states = append(states, game.BattleRuntimeState{InBattle: false, Controllable: false, TextActive: false})
	}
	for i := 0; i < settleStableFrames; i++ {
		states = append(states, game.BattleRuntimeState{InBattle: false, Controllable: true, TextActive: false})
	}
	decoder := &settleRuntimeDecoder{states: states}

	if err := settleAfterBattle(m, decoder); err != nil {
		t.Fatalf("settleAfterBattle = %v, want nil when control returns at the idle deadline", err)
	}
}
