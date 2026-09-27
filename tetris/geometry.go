package tetris

// CellOffset is one occupied tetromino cell relative to PieceState's anchor.
// X grows right and Y grows down.
type CellOffset struct {
	X int
	Y int
}

var pieceCells = map[Piece][4][4]CellOffset{
	PieceL: {
		{{-2, 0}, {-1, 0}, {0, 0}, {-2, 1}},
		{{-1, -1}, {-1, 0}, {-1, 1}, {0, 1}},
		{{0, -1}, {-2, 0}, {-1, 0}, {0, 0}},
		{{-2, -1}, {-1, -1}, {-1, 0}, {-1, 1}},
	},
	PieceJ: {
		{{-2, 0}, {-1, 0}, {0, 0}, {0, 1}},
		{{-1, -1}, {0, -1}, {-1, 0}, {-1, 1}},
		{{-2, -1}, {-2, 0}, {-1, 0}, {0, 0}},
		{{-1, -1}, {-1, 0}, {-2, 1}, {-1, 1}},
	},
	PieceI: {
		{{-2, 0}, {-1, 0}, {0, 0}, {1, 0}},
		{{-1, -2}, {-1, -1}, {-1, 0}, {-1, 1}},
		{{-2, 0}, {-1, 0}, {0, 0}, {1, 0}},
		{{-1, -2}, {-1, -1}, {-1, 0}, {-1, 1}},
	},
	PieceO: {
		{{-1, 0}, {0, 0}, {-1, 1}, {0, 1}},
		{{-1, 0}, {0, 0}, {-1, 1}, {0, 1}},
		{{-1, 0}, {0, 0}, {-1, 1}, {0, 1}},
		{{-1, 0}, {0, 0}, {-1, 1}, {0, 1}},
	},
	PieceS: {
		{{-2, 0}, {-1, 0}, {-1, 1}, {0, 1}},
		{{-1, -1}, {-2, 0}, {-1, 0}, {-2, 1}},
		{{-2, 0}, {-1, 0}, {-1, 1}, {0, 1}},
		{{-1, -1}, {-2, 0}, {-1, 0}, {-2, 1}},
	},
	PieceZ: {
		{{-1, 0}, {0, 0}, {-2, 1}, {-1, 1}},
		{{-2, -1}, {-2, 0}, {-1, 0}, {-1, 1}},
		{{-1, 0}, {0, 0}, {-2, 1}, {-1, 1}},
		{{-2, -1}, {-2, 0}, {-1, 0}, {-1, 1}},
	},
	PieceT: {
		{{-2, 0}, {-1, 0}, {0, 0}, {-1, 1}},
		{{-1, -1}, {-1, 0}, {0, 0}, {-1, 1}},
		{{-1, -1}, {-2, 0}, {-1, 0}, {0, 0}},
		{{-1, -1}, {-2, 0}, {-1, 0}, {-1, 1}},
	},
}

// Cells returns the exact occupied cell offsets for the supported Tetris
// revision's raw piece/orientation index. The geometry is transcribed from the
// game's sprite matrix: PieceState's anchor corresponds to matrix row/column 2.
func Cells(piece Piece, rotation uint8) ([4]CellOffset, bool) {
	rotations, ok := pieceCells[piece]
	if !ok || rotation > 3 {
		return [4]CellOffset{}, false
	}
	return rotations[rotation], true
}
