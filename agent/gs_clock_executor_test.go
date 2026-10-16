package agent

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSClockExecutorStructuredErrors(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	if _, err := game.PerformClockLeg(adapter, game.ClockLeg{Until: game.TimeMorning}); !errors.Is(err, game.ErrClockUnavailable) {
		t.Fatalf("nil emulator: %v", err)
	}
	if _, err := game.PerformClockLeg(adapter, game.ClockLeg{Until: "dusk"}); !errors.Is(err, game.ErrUnknownClockPeriod) {
		t.Fatalf("unknown period: %v", err)
	}
}

// A ROM-backed boundary regression (opt-in like the existing GS opening tests).
// The checkpoint is the replay contract: a restored clock and the observed
// wTimeOfDay must agree without waiting real hours or re-performing the leg.
func TestGoldClockLegCheckpointReplay(t *testing.T) {
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	m, err := emu.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if _, err := skill.BootToOverworld(m); err != nil {
		t.Fatalf("BootToOverworld: %v", err)
	}
	a := newGSObjectiveAdapter(m, m.ROM(), gsprofile.GoldGameID)
	before := m.FrameCount()
	observed, err := game.PerformClockLeg(a, game.ClockLeg{Until: game.TimeNight})
	if err != nil || observed != game.TimeNight {
		t.Fatalf("clock leg = %s, %v", observed, err)
	}
	if frames := m.FrameCount() - before; frames > gsClockSettleFrames+1 {
		t.Fatalf("clock leg ran %d frames, want at most %d", frames, gsClockSettleFrames+1)
	}
	clock, err := m.RTC()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := m.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	frames := m.FrameCount()
	m.StepFrames(120)
	if err := m.LoadState(checkpoint); err != nil {
		t.Fatal(err)
	}
	restored, err := m.RTC()
	if err != nil || restored != clock {
		t.Fatalf("restored RTC = %+v, %v, want %+v", restored, err, clock)
	}
	if m.FrameCount() != frames {
		t.Fatalf("restored frame = %d, want %d", m.FrameCount(), frames)
	}
	if got := gsprofile.NewGold().ObservedClockPeriod(m); got != game.TimeNight {
		t.Fatalf("replayed observed period = %s, want night", got)
	}
}
