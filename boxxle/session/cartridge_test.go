package session

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/profile"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/profiles"
)

// These regressions need the supported cartridge and are skipped without it,
// like the Tetris cartridge tests. They exist because the synthetic fixtures
// in the boxxle package encode the same layout the decoder reads: a decoder
// that disagrees with the cartridge passes every one of them. This one failed
// the original decoder, which read a WRAM address that holds sprite data.
func openBoxxleCartridge(t *testing.T) *emu.Emu {
	t.Helper()
	path := os.Getenv("BOXXLE_ROM")
	if path == "" {
		path = "../../roms/boxxle.gb"
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("BOXXLE_ROM: %v", err)
	}
	if _, _, err := profiles.DetectCartridge(rom); err != nil {
		t.Skipf("BOXXLE_ROM is not the supported revision: %v", err)
	}
	m, err := emu.OpenCGBBytes(rom)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { m.Close() })
	// Every launch path boots before it plays; start from the same boundary.
	if _, err := Boot(profile.New(), m); err != nil {
		t.Fatalf("launch boot: %v", err)
	}
	return m
}

// TestBootReachesFirstPuzzle drives the real title and menu into level 1-1 and
// checks the decoded board is that level: 3 crates, 3 goals, a sealed interior.
func TestBootReachesFirstPuzzle(t *testing.T) {
	m := openBoxxleCartridge(t)
	state, err := bootToPuzzle(profile.New(), m)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != boxxle.ScreenPuzzle || state.Player == nil {
		t.Fatalf("boot ended on %s player=%v, want a puzzle", state.Screen, state.Player)
	}
	if len(state.Crates) != 3 || len(state.Goals) != 3 {
		t.Fatalf("level 1-1 decoded %d crates / %d goals, want 3 / 3", len(state.Crates), len(state.Goals))
	}
}

// TestRunSolvesAndAdvances is the first-milestone qualification: boot, decode,
// solve autonomously, and move on to the next puzzle, twice over.
func TestRunSolvesAndAdvances(t *testing.T) {
	m := openBoxxleCartridge(t)
	res := Run(profile.New(), m, RunOptions{Levels: 2, MaxPushes: 300, MaxFrames: 100000})
	if res.Err != nil || res.Reason != "done" {
		t.Fatalf("Run = %s err=%v after %d push(es), want done", res.Reason, res.Err, res.Pushes)
	}
	if res.Levels != 2 {
		t.Fatalf("Levels = %d, want 2", res.Levels)
	}
	// "done" is only the positive proof of progress if the run ended on a fresh,
	// unsolved board rather than the solved one it started the advance from.
	if res.State.Screen != boxxle.ScreenPuzzle || res.State.Solved {
		t.Fatalf("ended on %s solved=%v, want an unsolved next puzzle", res.State.Screen, res.State.Solved)
	}
	if res.UsedFallback {
		t.Fatal("first-milestone solve used solver fallback; the deterministic solver should own these boards")
	}
}

// TestEarlyLevelBatch is the post-milestone qualification: solve the first
// five puzzles and advance after each one.
func TestEarlyLevelBatch(t *testing.T) {
	m := openBoxxleCartridge(t)
	res := Run(profile.New(), m, RunOptions{Goal: Goal{Kind: GoalEarly, Levels: EarlyLevels}, MaxPushes: 800, MaxFrames: 250000})
	if res.Err != nil || res.Reason != "done" {
		t.Fatalf("Run = %s err=%v after %d push(es), want done", res.Reason, res.Err, res.Pushes)
	}
	if res.Levels != EarlyLevels {
		t.Fatalf("Levels = %d, want %d", res.Levels, EarlyLevels)
	}
	if res.State.Screen != boxxle.ScreenPuzzle || res.State.Solved {
		t.Fatalf("ended on %s solved=%v, want an unsolved next puzzle", res.State.Screen, res.State.Solved)
	}
	t.Logf("early batch: %d pushes, fallback=%v", res.Pushes, res.UsedFallback)
}
