package tetris

import "testing"

func TestCellsMatchKnownSpawnGeometry(t *testing.T) {
	tests := []struct {
		piece    Piece
		rotation uint8
		want     [4]CellOffset
	}{
		{PieceI, 0, [4]CellOffset{{-2, 0}, {-1, 0}, {0, 0}, {1, 0}}},
		{PieceI, 1, [4]CellOffset{{-1, -2}, {-1, -1}, {-1, 0}, {-1, 1}}},
		{PieceO, 0, [4]CellOffset{{-1, 0}, {0, 0}, {-1, 1}, {0, 1}}},
		{PieceT, 0, [4]CellOffset{{-2, 0}, {-1, 0}, {0, 0}, {-1, 1}}},
		{PieceT, 2, [4]CellOffset{{-1, -1}, {-2, 0}, {-1, 0}, {0, 0}}},
	}
	for _, tc := range tests {
		got, ok := Cells(tc.piece, tc.rotation)
		if !ok {
			t.Fatalf("Cells(%s,%d) unavailable", tc.piece, tc.rotation)
		}
		if got != tc.want {
			t.Fatalf("Cells(%s,%d) = %#v, want %#v", tc.piece, tc.rotation, got, tc.want)
		}
	}
}

func TestCellsEveryTetrominoHasFourUniqueCells(t *testing.T) {
	for _, piece := range []Piece{PieceL, PieceJ, PieceI, PieceO, PieceS, PieceZ, PieceT} {
		for rotation := uint8(0); rotation < 4; rotation++ {
			cells, ok := Cells(piece, rotation)
			if !ok {
				t.Fatalf("Cells(%s,%d) unavailable", piece, rotation)
			}
			seen := make(map[CellOffset]bool, 4)
			for _, cell := range cells {
				if seen[cell] {
					t.Fatalf("Cells(%s,%d) contains duplicate %#v", piece, rotation, cell)
				}
				seen[cell] = true
			}
		}
	}
}

func TestCellsRejectUnknownPieceAndRotation(t *testing.T) {
	if _, ok := Cells(Piece("X"), 0); ok {
		t.Fatal("unknown piece unexpectedly accepted")
	}
	if _, ok := Cells(PieceT, 4); ok {
		t.Fatal("rotation 4 unexpectedly accepted")
	}
}
