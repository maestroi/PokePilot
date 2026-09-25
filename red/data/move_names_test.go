package data

import "testing"

func TestMoveNameSemanticTable(t *testing.T) {
	tests := []struct {
		id   uint8
		want string
	}{
		{33, "tackle"},
		{44, "bite"},
		{57, "surf"},
		{165, "struggle"},
	}
	for _, tt := range tests {
		got, ok := MoveName(tt.id)
		if !ok || got != tt.want {
			t.Fatalf("MoveName(%d) = %q, %v; want %q, true", tt.id, got, ok, tt.want)
		}
	}
	for _, id := range []uint8{0, 166, 255} {
		if got, ok := MoveName(id); ok || got != "" {
			t.Fatalf("MoveName(%d) = %q, %v; want empty,false", id, got, ok)
		}
	}
}
