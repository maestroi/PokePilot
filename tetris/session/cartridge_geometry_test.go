package session

import (
	"os"
	"sort"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/tetris"
	tetriscontrol "github.com/maestroi/pokepilot/tetris/control"
	tetrispolicy "github.com/maestroi/pokepilot/tetris/policy"
	tetrisprofile "github.com/maestroi/pokepilot/tetris/profile"
)

// These regressions need the supported cartridge and are skipped without it,
// the same way profiles.TestTetrisCartridgeResolvesWhenROMAvailable is. They
// exist because the pure unit tests share Cells with the code under test: a
// table that disagrees with the cartridge passes every one of them, which is
// how the anchor column shipped one cell left (#2061) and the anchor row one
// cell high.
func openTetrisCartridge(t *testing.T, mode tetris.Mode) (*emu.Emu, *tetrisprofile.Profile) {
	t.Helper()
	path := os.Getenv("TETRIS_ROM")
	if path == "" {
		path = "../../roms/tetris.gb"
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("TETRIS_ROM: %v", err)
	}
	m, err := emu.OpenCGBBytes(rom)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	profile := tetrisprofile.New()
	if _, err := BootToPlaying(profile, m, mode); err != nil {
		t.Fatalf("boot: %v", err)
	}
	return m, profile
}

// TestAnchorFootprintMatchesCartridge locks pieces with no horizontal input and
// asserts that Cells describes the cells the cartridge itself writes: the
// decoded anchor may be sprite-matrix row/column 2, but anchor + Cells must sit
// exactly on the playfield buffer cells the game fills.
func TestAnchorFootprintMatchesCartridge(t *testing.T) {
	const want = 6
	m, profile := openTetrisCartridge(t, tetris.ModeA)
	defer m.Close()

	asserted := 0
	for attempts := 0; attempts < 60 && asserted < want; attempts++ {
		state, err := tetris.Observe(profile, m)
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		if state.GameOver || state.Complete {
			break
		}
		if !state.ReadyForPieceInput || state.Active == nil {
			m.StepFrame()
			continue
		}
		before := state.Board

		// Spread the stack so a line clear cannot consume the footprint under
		// test before we read the board.
		for i := 0; i < asserted%4; i++ {
			pulse(m, emu.Left)
		}

		anchor, settled, ok := softDropToLock(profile, m)
		if !ok || anchor.Active == nil {
			continue
		}
		locked := *anchor.Active
		written := writtenCells(before, settled.Board)
		if len(written) != 4 {
			continue
		}
		model, ok := tetris.Cells(locked.Piece, locked.Rotation)
		if !ok {
			t.Fatalf("Cells(%s,%d) unavailable", locked.Piece, locked.Rotation)
		}
		got := make([]tetris.CellOffset, 0, 4)
		for _, cell := range written {
			got = append(got, tetris.CellOffset{X: cell.x - locked.X, Y: cell.y - locked.Y})
		}
		if !sameOffsets(got, model[:]) {
			t.Fatalf("%s rotation %d at anchor (%d,%d): cartridge cells %v, Cells says %v",
				locked.Piece, locked.Rotation, locked.X, locked.Y, got, model)
		}
		asserted++
	}
	if asserted < want {
		t.Fatalf("asserted %d footprints, want %d", asserted, want)
	}
}

// TestPlannedBoardMatchesCartridge states the runtime invariant behind both
// halves of the anchor bug: the board policy plans must be the board the
// cartridge produces. A model that disagrees on either axis, or a column the
// controller cannot execute, fails here.
func TestPlannedBoardMatchesCartridge(t *testing.T) {
	const (
		minPlacements = 6
		maxPlacements = 8
	)
	m, profile := openTetrisCartridge(t, tetris.ModeA)
	defer m.Close()

	placed := 0
	for attempts := 0; attempts < 80 && placed < maxPlacements; attempts++ {
		state, err := tetris.Observe(profile, m)
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		if state.GameOver || state.Complete {
			break
		}
		if !state.ReadyForPieceInput {
			m.StepFrame()
			continue
		}

		decision, err := tetrispolicy.Choose(state, tetrispolicy.ObjectiveScore)
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		placement := decision.Candidate.Placement
		result, err := tetriscontrol.Place(profile, m, placement)
		if err != nil {
			t.Fatalf("place %s rotation %d column %d: %v", decision.Piece, placement.Rotation, placement.Column, err)
		}
		if decision.Candidate.ResultBoard != result.After.Board {
			t.Fatalf("planned board != cartridge board after %s rotation %d column %d",
				decision.Piece, placement.Rotation, placement.Column)
		}
		if decision.Candidate.LinesCleared != result.LinesCleared {
			t.Fatalf("planned %d lines, cartridge cleared %d for %s rotation %d column %d",
				decision.Candidate.LinesCleared, result.LinesCleared, decision.Piece, placement.Rotation, placement.Column)
		}
		placed++
	}
	if placed < minPlacements {
		t.Fatalf("verified %d placements against the cartridge, want at least %d", placed, minPlacements)
	}
}

type boardCell struct{ x, y int }

func writtenCells(before, after tetris.Board) []boardCell {
	var out []boardCell
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			if after[y][x] && !before[y][x] {
				out = append(out, boardCell{x, y})
			}
		}
	}
	return out
}

// softDropToLock holds Down until the piece starts locking and returns the last
// anchor observed while the piece was still visible, plus the settled state.
// The cartridge hides the active piece on the lock frame but leaves the anchor
// registers untouched, so that anchor is where the cells land.
func softDropToLock(profile *tetrisprofile.Profile, m *emu.Emu) (tetris.State, tetris.State, bool) {
	m.Press(emu.Down)
	anchor, settled := tetris.State{}, tetris.State{}
	haveAnchor, locked := false, false
	for i := 0; i < 400; i++ {
		m.StepFrame()
		state, err := tetris.Observe(profile, m)
		if err != nil {
			m.Release(emu.Down)
			return anchor, settled, false
		}
		if state.Locking || state.Clearing || state.GameOver || state.Complete {
			locked = true
			break
		}
		if state.Active != nil {
			anchor, haveAnchor = state, true
		}
	}
	m.Release(emu.Down)
	if !locked || !haveAnchor {
		return anchor, settled, false
	}
	m.StepFrame()
	for i := 0; i < 600; i++ {
		state, err := tetris.Observe(profile, m)
		if err != nil {
			return anchor, settled, false
		}
		if state.GameOver || state.Complete || state.ReadyForPieceInput {
			return anchor, state, true
		}
		m.StepFrame()
	}
	return anchor, settled, false
}

func sameOffsets(a, b []tetris.CellOffset) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := sortedOffsets(a), sortedOffsets(b)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sortedOffsets(in []tetris.CellOffset) []tetris.CellOffset {
	out := append([]tetris.CellOffset(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}
