package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/skill"
)

func TestRedNPCRewardItemIDResolvesTrustedRewardTable(t *testing.T) {
	for _, reward := range skill.ChoiceRewards() {
		o := Objective{
			Kind:   KindPickup,
			X:      reward.X,
			Y:      reward.Y,
			Item:   ItemID(reward.ItemName),
			Intent: redNPCRewardIntent,
		}
		got, ok := redNPCRewardItemID(reward.Map, o)
		if !ok || got != reward.Item {
			t.Fatalf("reward %s resolved to %#02x,%v, want %#02x,true", reward.ItemName, got, ok, reward.Item)
		}
	}
}

func TestRedNPCRewardItemIDRejectsUntrustedPickupShape(t *testing.T) {
	reward := skill.ChoiceRewards()[0]
	base := Objective{
		Kind:   KindPickup,
		X:      reward.X,
		Y:      reward.Y,
		Item:   ItemID(reward.ItemName),
		Intent: redNPCRewardIntent,
	}

	cases := []struct {
		name  string
		mapID uint8
		o     Objective
	}{
		{name: "wrong map", mapID: reward.Map - 1, o: base},
		{name: "missing intent", mapID: reward.Map, o: func() Objective { o := base; o.Intent = ""; return o }()},
		{name: "wrong actor", mapID: reward.Map, o: func() Objective { o := base; o.X++; return o }()},
		{name: "wrong item", mapID: reward.Map, o: func() Objective { o := base; o.Item = "potion"; return o }()},
		{name: "wrong kind", mapID: reward.Map, o: func() Objective { o := base; o.Kind = KindTalk; return o }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if raw, ok := redNPCRewardItemID(tc.mapID, tc.o); ok {
				t.Fatalf("untrusted reward shape resolved to %#02x", raw)
			}
		})
	}
}

func TestRedObjectiveAdapterValidatesOldRodRewardWithoutGenericWhitelist(t *testing.T) {
	if raw, ok := redItemID(ItemID("old rod")); ok {
		t.Fatalf("old rod unexpectedly widened generic executable vocabulary: %#02x", raw)
	}

	o := Objective{
		Kind:   KindPickup,
		X:      2,
		Y:      4,
		Item:   ItemID("old rod"),
		Intent: redNPCRewardIntent,
	}
	adapter := &redObjectiveAdapter{}
	if err := adapter.Validate(o, Observation{Map: 0xA3}); err != nil {
		t.Fatalf("Validate(old rod reward): %v", err)
	}
	if raw, ok := adapter.resolvePickupItemID(0xA3, o); !ok || raw != 0x4C {
		t.Fatalf("resolvePickupItemID(old rod reward) = %#02x,%v, want 0x4c,true", raw, ok)
	}
}
