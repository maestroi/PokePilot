package skill

import "testing"

func TestLeagueItemIDsMatchGen1Constants(t *testing.T) {
	if itemRevive != 0x35 || itemMaxRevive != 0x36 {
		t.Fatalf("revive ids = %#02x/%#02x, want 0x35/0x36", itemRevive, itemMaxRevive)
	}
	if itemEther != 0x50 || itemMaxEther != 0x51 || itemElixer != 0x52 || itemMaxElixer != 0x53 {
		t.Fatalf("PP restore ids = %#02x/%#02x/%#02x/%#02x, want 0x50..0x53", itemEther, itemMaxEther, itemElixer, itemMaxElixer)
	}
}
