package skill

import "testing"

func TestRepelDuration(t *testing.T) {
	cases := map[uint8]int{
		ItemRepel:      100,
		ItemSuperRepel: 200,
		ItemMaxRepel:   250,
	}
	for item, want := range cases {
		got, ok := RepelDuration(item)
		if !ok || got != want {
			t.Errorf("RepelDuration(%#02x) = %d,%v, want %d,true", item, got, ok, want)
		}
	}
	if _, ok := RepelDuration(0x14); ok {
		t.Fatal("POTION classified as Repel")
	}
}
