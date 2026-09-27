package tetris

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris/sym"
)

type testMemory [1 << 16]byte

func (m *testMemory) Peek8(addr uint16) byte {
	return m[addr]
}

func (m *testMemory) PeekInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = m[uint16(int(addr)+i)]
	}
}

func gameplayMemory() *testMemory {
	m := &testMemory{}
	for y := 0; y < BoardHeight; y++ {
		row := sym.BoardTopLeft + uint16(y)*sym.BoardStride
		for x := 0; x < BoardWidth; x++ {
			m[row+uint16(x)] = sym.EmptyBoardTile
		}
	}
	m[sym.GameState] = 0x00
	m[sym.GameType] = sym.GameTypeA
	m[sym.ActiveVisible] = 0x00
	m[sym.ActiveX] = 0x3F
	m[sym.ActiveY] = 0x18
	m[sym.ActivePiece] = 0x18
	m[sym.PreviewPiece] = 0x00
	return m
}

func TestDecodeStateTypeA(t *testing.T) {
	m := gameplayMemory()
	m[sym.Level] = 9
	m[sym.ActiveX] = 0x47
	m[sym.ActiveY] = 0x28
	m[sym.ActivePiece] = 0x1A
	m[sym.PreviewPiece] = 0x09

	m[sym.Score] = 0x56
	m[sym.Score+1] = 0x34
	m[sym.Score+2] = 0x12
	m[sym.Lines] = 0x23
	m[sym.Lines+1] = 0x01

	m[sym.BoardTopLeft] = 0x80
	m[sym.BoardTopLeft+17*sym.BoardStride+9] = 0x8C

	got, err := DecodeState(m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeA || got.Screen != ScreenPlaying {
		t.Fatalf("mode/screen = %s/%s", got.Mode, got.Screen)
	}
	if !got.ScoreValid || got.Score != 123456 {
		t.Fatalf("score = %d valid=%v", got.Score, got.ScoreValid)
	}
	if got.LinesCleared != 123 || got.LinesRemaining != 0 || got.LineGoal != 0 {
		t.Fatalf("lines = cleared:%d remaining:%d goal:%d", got.LinesCleared, got.LinesRemaining, got.LineGoal)
	}
	if got.Level != 9 {
		t.Fatalf("level = %d", got.Level)
	}
	if got.Active == nil {
		t.Fatal("active piece is nil")
	}
	if got.Active.Piece != PieceT || got.Active.Rotation != 2 || got.Active.X != 5 || got.Active.Y != 2 {
		t.Fatalf("active = %#v", *got.Active)
	}
	if got.Next.Piece != PieceI || got.Next.Rotation != 1 {
		t.Fatalf("next = %#v", got.Next)
	}
	if !got.Board[0][0] || !got.Board[17][9] || got.Board[0][1] {
		t.Fatalf("unexpected board occupancy")
	}
	if !got.ReadyForPieceInput {
		t.Fatal("expected piece input to be ready")
	}
}

func TestDecodeStateTypeBLinesAndScoreSemantics(t *testing.T) {
	m := gameplayMemory()
	m[sym.GameType] = sym.GameTypeB
	m[sym.Level] = 9
	m[sym.Lines] = 0x17
	m[sym.Score] = 0xFF // Type B has no running score; this must be ignored.

	got, err := DecodeState(m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeB {
		t.Fatalf("mode = %s", got.Mode)
	}
	if got.ScoreValid || got.Score != 0 {
		t.Fatalf("unexpected Type B score = %d valid=%v", got.Score, got.ScoreValid)
	}
	if got.LineGoal != 25 || got.LinesRemaining != 17 || got.LinesCleared != 8 {
		t.Fatalf("lines = cleared:%d remaining:%d goal:%d", got.LinesCleared, got.LinesRemaining, got.LineGoal)
	}
}

func TestDecodeStateVersusUsesThirtyLineGoal(t *testing.T) {
	m := gameplayMemory()
	m[sym.IsMultiplayer] = 1
	m[sym.Lines] = 0x24

	got, err := DecodeState(m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeVersus || got.LineGoal != 30 || got.LinesRemaining != 24 || got.LinesCleared != 6 {
		t.Fatalf("versus state = %#v", got)
	}
}

func TestDecodeStateSuppressesPieceInputDuringTransitions(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*testMemory)
	}{
		{"paused", func(m *testMemory) { m[sym.Paused] = 1 }},
		{"locking", func(m *testMemory) { m[sym.LockStage] = 1 }},
		{"wiping", func(m *testMemory) { m[sym.WipeCounter] = 1 }},
		{"hidden", func(m *testMemory) { m[sym.ActiveVisible] = 0x80 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := gameplayMemory()
			tc.set(m)
			got, err := DecodeState(m)
			if err != nil {
				t.Fatal(err)
			}
			if got.ReadyForPieceInput {
				t.Fatalf("piece input unexpectedly ready: %#v", got)
			}
		})
	}
}

func TestDecodeStateGameOver(t *testing.T) {
	m := gameplayMemory()
	m[sym.GameState] = 0x04

	got, err := DecodeState(m)
	if err != nil {
		t.Fatal(err)
	}
	if got.Screen != ScreenGameOver || !got.GameOver || got.Complete || got.ReadyForPieceInput {
		t.Fatalf("game-over state = %#v", got)
	}
	if got.Active != nil {
		t.Fatalf("active piece should not be exposed on game-over screen: %#v", got.Active)
	}
}

func TestDecodeStateRejectsInvalidLineBCD(t *testing.T) {
	m := gameplayMemory()
	m[sym.Lines] = 0xFA
	if _, err := DecodeState(m); err == nil || !strings.Contains(err.Error(), "lines") {
		t.Fatalf("expected line BCD error, got %v", err)
	}
}

func TestDecodePiece(t *testing.T) {
	tests := []struct {
		raw      byte
		piece    Piece
		rotation uint8
	}{
		{0x00, PieceL, 0},
		{0x07, PieceJ, 3},
		{0x09, PieceI, 1},
		{0x0E, PieceO, 2},
		{0x10, PieceS, 0},
		{0x17, PieceZ, 3},
		{0x1B, PieceT, 3},
	}
	for _, tc := range tests {
		piece, rotation, ok := DecodePiece(tc.raw)
		if !ok || piece != tc.piece || rotation != tc.rotation {
			t.Fatalf("DecodePiece(%#02x) = %q,%d,%v", tc.raw, piece, rotation, ok)
		}
	}
	if _, _, ok := DecodePiece(0x1C); ok {
		t.Fatal("expected non-tetromino sprite id to be rejected")
	}
}

type identityOnlyProfile struct{}

func (identityOnlyProfile) ID() game.GameID               { return "identity-only" }
func (identityOnlyProfile) Revision() game.RevisionID     { return "rev0" }
func (identityOnlyProfile) Detect(game.ROMInfo) bool      { return false }

func TestObserveRequiresTetrisStateCapability(t *testing.T) {
	_, err := Observe(identityOnlyProfile{}, gameplayMemory())
	if err == nil || !strings.Contains(err.Error(), "does not expose Tetris state") {
		t.Fatalf("expected capability error, got %v", err)
	}
}
