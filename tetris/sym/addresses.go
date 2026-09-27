// Package sym contains RAM symbols for Tetris (World) (Rev 1).
//
// Addresses are verified against the kaspermeerts/tetris disassembly for the
// exact ROM fingerprint supported by tetris/profile. Keep revision-specific
// addresses here rather than in generic runtime code.
package sym

const (
	// Score is a six-digit packed-BCD value stored least-significant pair first.
	Score uint16 = 0xC0A0

	// Active piece sprite/state structure.
	ActiveVisible uint16 = 0xC200
	ActiveY       uint16 = 0xC201
	ActiveX       uint16 = 0xC202
	ActivePiece   uint16 = 0xC203

	// PreviewPiece stores the visible next piece id/orientation.
	PreviewPiece uint16 = 0xC213

	// BoardTopLeft is the top-left cell of the 10x18 playfield buffer.
	// Rows are separated by BoardStride bytes.
	BoardTopLeft uint16 = 0xC802
	BoardStride  uint16 = 0x20

	// LockStage is non-zero while a landed piece is locking/clearing.
	LockStage uint16 = 0xFF98

	// Lines starts at the Type A two-byte packed-BCD total. Type B/versus use
	// only the low byte as their remaining-goal counter.
	Lines uint16 = 0xFF9E

	Level  uint16 = 0xFFA9
	Paused uint16 = 0xFFAB

	GameType      uint16 = 0xFFC0
	IsMultiplayer uint16 = 0xFFC5

	GameState   uint16 = 0xFFE1
	WipeCounter uint16 = 0xFFE3
)

const (
	EmptyBoardTile byte = 0x2F

	GameTypeA byte = 0x37
	GameTypeB byte = 0x77
)
