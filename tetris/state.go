// Package tetris defines semantic Tetris state independent of raw RAM layout.
package tetris

import (
	"fmt"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/tetris/sym"
)

const (
	BoardWidth  = 10
	BoardHeight = 18
)

type Mode string

const (
	ModeUnknown Mode = "unknown"
	ModeA       Mode = "type-a"
	ModeB       Mode = "type-b"
	ModeVersus  Mode = "versus"
)

type Screen string

const (
	ScreenUnknown     Screen = "unknown"
	ScreenIntro       Screen = "intro"
	ScreenTitle       Screen = "title"
	ScreenGameType    Screen = "game-type-select"
	ScreenMusic       Screen = "music-select"
	ScreenLevel       Screen = "level-select"
	ScreenHeight      Screen = "height-select"
	ScreenHighScore   Screen = "high-score"
	ScreenStarting    Screen = "starting"
	ScreenPlaying     Screen = "playing"
	ScreenVersusSetup Screen = "versus-setup"
	ScreenGameOver    Screen = "game-over"
	ScreenEnding      Screen = "ending"
)

type Piece string

const (
	PieceL Piece = "L"
	PieceJ Piece = "J"
	PieceI Piece = "I"
	PieceO Piece = "O"
	PieceS Piece = "S"
	PieceZ Piece = "Z"
	PieceT Piece = "T"
)

type Board [BoardHeight][BoardWidth]bool

type PieceState struct {
	Piece    Piece `json:"piece"`
	Rotation uint8 `json:"rotation"`
	X        int   `json:"x"`
	Y        int   `json:"y"`
}

type PiecePreview struct {
	Piece    Piece `json:"piece"`
	Rotation uint8 `json:"rotation"`
}

// State is the semantic snapshot consumed by future Tetris policy/controller
// code. Board contains locked cells only; the falling piece is separate.
type State struct {
	Mode   Mode   `json:"mode"`
	Screen Screen `json:"screen"`

	Board Board `json:"board"`

	Active *PieceState   `json:"active,omitempty"`
	Next   *PiecePreview `json:"next,omitempty"`

	Level          int  `json:"level"`
	Score          int  `json:"score"`
	ScoreValid     bool `json:"score_valid"`
	LinesCleared   int  `json:"lines_cleared"`
	LinesRemaining int  `json:"lines_remaining"`
	LineGoal       int  `json:"line_goal"`

	Paused             bool `json:"paused"`
	GameOver           bool `json:"game_over"`
	Complete           bool `json:"complete"`
	ReadyForPieceInput bool `json:"ready_for_piece_input"`
}

// StateProfile is the optional gameplay-state capability implemented by the
// Tetris cartridge profile. It deliberately extends CartridgeProfile rather
// than the Pokemon-specific game.GameProfile.
type StateProfile interface {
	game.CartridgeProfile
	DecodeTetrisState(game.MemoryReader) (State, error)
}

// Observe decodes Tetris state through a cartridge capability without teaching
// generic cartridge selection about Tetris-specific fields.
func Observe(profile game.CartridgeProfile, reader game.MemoryReader) (State, error) {
	decoder, ok := profile.(StateProfile)
	if !ok {
		if profile == nil {
			return State{}, fmt.Errorf("tetris: nil cartridge profile")
		}
		return State{}, fmt.Errorf("tetris: cartridge profile %s@%s does not expose Tetris state", profile.ID(), profile.Revision())
	}
	return decoder.DecodeTetrisState(reader)
}

// DecodeState translates the supported Tetris RAM layout into semantic state.
func DecodeState(reader game.MemoryReader) (State, error) {
	if reader == nil {
		return State{}, fmt.Errorf("tetris: nil memory reader")
	}

	gameState := reader.Peek8(sym.GameState)
	mode := decodeMode(reader.Peek8(sym.GameType), reader.Peek8(sym.IsMultiplayer) != 0)
	screen := decodeScreen(gameState)

	state := State{
		Mode:     mode,
		Screen:   screen,
		Board:    decodeBoard(reader),
		Level:    int(reader.Peek8(sym.Level)),
		Paused:   reader.Peek8(sym.Paused) != 0,
		GameOver: isGameOverState(gameState),
		Complete: isCompleteState(gameState),
	}

	if mode == ModeA {
		score, err := decodePackedBCDLE(readBytes(reader, sym.Score, 3))
		if err != nil {
			return State{}, fmt.Errorf("tetris: score: %w", err)
		}
		state.Score = score
		state.ScoreValid = true
	}

	lineBytes := 2
	if mode == ModeB || mode == ModeVersus {
		// Type B and versus initialize/update only the low BCD byte. The high
		// byte is not part of their line-goal state.
		lineBytes = 1
	}
	lines, err := decodePackedBCDLE(readBytes(reader, sym.Lines, lineBytes))
	if err != nil {
		return State{}, fmt.Errorf("tetris: lines: %w", err)
	}
	switch mode {
	case ModeB:
		state.LineGoal = 25
		state.LinesRemaining = lines
		state.LinesCleared = clampNonNegative(state.LineGoal - lines)
	case ModeVersus:
		state.LineGoal = 30
		state.LinesRemaining = lines
		state.LinesCleared = clampNonNegative(state.LineGoal - lines)
	default:
		state.LinesCleared = lines
	}

	if preview, ok := decodePreview(reader.Peek8(sym.PreviewPiece)); ok {
		state.Next = &preview
	}

	if screen == ScreenPlaying && reader.Peek8(sym.ActiveVisible) != 0x80 {
		if active, ok := decodeActive(
			reader.Peek8(sym.ActivePiece),
			reader.Peek8(sym.ActiveX),
			reader.Peek8(sym.ActiveY),
		); ok {
			state.Active = &active
		}
	}

	state.ReadyForPieceInput =
		screen == ScreenPlaying &&
			!state.Paused &&
			reader.Peek8(sym.LockStage) == 0 &&
			reader.Peek8(sym.WipeCounter) == 0 &&
			state.Active != nil

	return state, nil
}

func decodeBoard(reader game.MemoryReader) Board {
	var board Board
	for y := 0; y < BoardHeight; y++ {
		row := sym.BoardTopLeft + uint16(y)*sym.BoardStride
		for x := 0; x < BoardWidth; x++ {
			board[y][x] = reader.Peek8(row+uint16(x)) != sym.EmptyBoardTile
		}
	}
	return board
}

func decodeMode(raw byte, multiplayer bool) Mode {
	if multiplayer {
		return ModeVersus
	}
	switch raw {
	case sym.GameTypeA:
		return ModeA
	case sym.GameTypeB:
		return ModeB
	default:
		return ModeUnknown
	}
}

func decodeScreen(raw byte) Screen {
	switch raw {
	case 0x00, 0x1A:
		return ScreenPlaying
	case 0x01, 0x04, 0x0D, 0x34:
		return ScreenGameOver
	case 0x06, 0x07:
		return ScreenTitle
	case 0x08, 0x09, 0x0E:
		return ScreenGameType
	case 0x0F, 0x2A, 0x2B:
		return ScreenMusic
	case 0x10, 0x11, 0x12, 0x13:
		return ScreenLevel
	case 0x14:
		return ScreenHeight
	case 0x15:
		return ScreenHighScore
	case 0x0A, 0x18, 0x19, 0x1C, 0x1F:
		return ScreenStarting
	case 0x16, 0x17:
		return ScreenVersusSetup
	case 0x24, 0x25, 0x35:
		return ScreenIntro
	case 0x02, 0x03, 0x05, 0x1B, 0x1D, 0x1E,
		0x20, 0x21, 0x22, 0x23, 0x26, 0x27, 0x28, 0x29,
		0x2C, 0x2D, 0x2E, 0x2F, 0x30, 0x31, 0x32, 0x33:
		return ScreenEnding
	default:
		return ScreenUnknown
	}
}

func isGameOverState(raw byte) bool {
	switch raw {
	case 0x01, 0x04, 0x0D, 0x34:
		return true
	default:
		return false
	}
}

func isCompleteState(raw byte) bool {
	switch raw {
	case 0x05, 0x20, 0x22, 0x23, 0x26, 0x27, 0x28, 0x29,
		0x2C, 0x2D, 0x2E, 0x2F, 0x30, 0x31, 0x32, 0x33:
		return true
	default:
		return false
	}
}

func decodeActive(rawPiece, rawX, rawY byte) (PieceState, bool) {
	piece, rotation, ok := DecodePiece(rawPiece)
	if !ok {
		return PieceState{}, false
	}
	return PieceState{
		Piece:    piece,
		Rotation: rotation,
		// The game spawns an anchor at raw X=$3f, Y=$18. Horizontal and
		// vertical movement change these values in exact 8-pixel increments.
		X: 4 + (int(rawX)-0x3f)/8,
		Y: (int(rawY) - 0x18) / 8,
	}, true
}

func decodePreview(raw byte) (PiecePreview, bool) {
	piece, rotation, ok := DecodePiece(raw)
	if !ok {
		return PiecePreview{}, false
	}
	return PiecePreview{Piece: piece, Rotation: rotation}, true
}

// DecodePiece maps the game's 0..27 piece/orientation id to a tetromino and
// quarter-turn orientation. The lower two bits encode rotation.
func DecodePiece(raw byte) (Piece, uint8, bool) {
	if raw > 0x1B {
		return "", 0, false
	}
	rotation := raw & 0x03
	switch raw & 0x1C {
	case 0x00:
		return PieceL, rotation, true
	case 0x04:
		return PieceJ, rotation, true
	case 0x08:
		return PieceI, rotation, true
	case 0x0C:
		return PieceO, rotation, true
	case 0x10:
		return PieceS, rotation, true
	case 0x14:
		return PieceZ, rotation, true
	case 0x18:
		return PieceT, rotation, true
	default:
		return "", 0, false
	}
}

func decodePackedBCDLE(raw []byte) (int, error) {
	multiplier := 1
	value := 0
	for _, b := range raw {
		hi, lo := b>>4, b&0x0F
		if hi > 9 || lo > 9 {
			return 0, fmt.Errorf("invalid packed BCD byte 0x%02x", b)
		}
		value += (int(hi)*10 + int(lo)) * multiplier
		multiplier *= 100
	}
	return value, nil
}

func readBytes(reader game.MemoryReader, addr uint16, n int) []byte {
	out := make([]byte, n)
	reader.PeekInto(addr, out)
	return out
}

func clampNonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
