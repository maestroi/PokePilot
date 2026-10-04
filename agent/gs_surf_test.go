package agent

import "testing"

func TestGSDanceTheaterCoordinatesMatchPinnedDecomp(t *testing.T) {
	want := []gsKimonoTrainer{
		{name: "naoko", x: 0, y: 2, standX: 0, standY: 3},
		{name: "sayo", x: 2, y: 1, standX: 2, standY: 2},
		{name: "zuki", x: 6, y: 2, standX: 6, standY: 3},
		{name: "kuni", x: 9, y: 1, standX: 9, standY: 2},
		{name: "miki", x: 11, y: 2, standX: 11, standY: 3},
	}
	if len(gsDanceTheaterKimonoGirls) != len(want) {
		t.Fatalf("Kimono Girl count = %d, want %d", len(gsDanceTheaterKimonoGirls), len(want))
	}
	for i := range want {
		if gsDanceTheaterKimonoGirls[i] != want[i] {
			t.Fatalf("Kimono Girl %d = %+v, want %+v", i, gsDanceTheaterKimonoGirls[i], want[i])
		}
	}
	if gsDanceTheaterSurfGuyX != 7 || gsDanceTheaterSurfGuyY != 10 ||
		gsDanceTheaterSurfGuyStandX != 7 || gsDanceTheaterSurfGuyStandY != 11 {
		t.Fatalf("Surf gentleman interaction = guy(%d,%d) stand(%d,%d), want guy(7,10) stand(7,11)",
			gsDanceTheaterSurfGuyX, gsDanceTheaterSurfGuyY,
			gsDanceTheaterSurfGuyStandX, gsDanceTheaterSurfGuyStandY)
	}
}

func TestGSDanceTheaterIsInsideOwnedGen2Corridor(t *testing.T) {
	theater, err := gsOpeningMapID("DANCE_THEATER")
	if err != nil {
		t.Fatal(err)
	}
	if !gsSecondBadgeOwnedMap(theater) {
		t.Fatalf("DANCE_THEATER (%#04x) is outside owned Gen2 corridor", theater)
	}
}
