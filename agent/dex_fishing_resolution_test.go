package agent

import "testing"

func TestRedFishingRodIDResolvesDexRodsWithoutGenericWhitelist(t *testing.T) {
	for _, tc := range []struct {
		name string
		want uint8
	}{
		{name: "old rod", want: 0x4C},
		{name: "good rod", want: 0x4D},
		{name: "super rod", want: 0x4E},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := ItemID(tc.name)
			if raw, ok := redItemID(id); ok {
				t.Fatalf("redItemID(%q) unexpectedly widened generic executable vocabulary: %#02x", id, raw)
			}
			raw, ok := redFishingRodID(id)
			if !ok || raw != tc.want {
				t.Fatalf("redFishingRodID(%q) = %#02x,%v, want %#02x,true", id, raw, ok, tc.want)
			}
		})
	}
}

func TestRedFishingRodIDRejectsNonRodItems(t *testing.T) {
	for _, id := range []ItemID{"bicycle", "pokeball", "hm03", ""} {
		if raw, ok := redFishingRodID(id); ok {
			t.Fatalf("redFishingRodID(%q) = %#02x,true, want rejected", id, raw)
		}
	}
}
