package skill

import "testing"

func TestChoiceRewardsHaveInteractionOwnedDestinations(t *testing.T) {
	rewards := ChoiceRewards()
	if len(rewards) != 6 {
		t.Fatalf("ChoiceRewards count = %d, want 6", len(rewards))
	}
	seen := map[string]bool{}
	for _, reward := range rewards {
		if reward.Place == "" || reward.ItemName == "" {
			t.Fatalf("incomplete choice reward: %+v", reward)
		}
		if seen[reward.Place] {
			t.Fatalf("duplicate choice reward place %q", reward.Place)
		}
		seen[reward.Place] = true
		if !IsChoiceRewardActor(reward.Map, reward.X, reward.Y) {
			t.Fatalf("reward actor was not classified: %+v", reward)
		}
		dest, ok := Place(reward.Place)
		if !ok {
			t.Fatalf("reward place %q did not resolve", reward.Place)
		}
		if dest.Map != reward.Map || dest.X != reward.StandX || dest.Y != reward.StandY {
			t.Fatalf("reward place %q = %+v, want map %#02x stand (%d,%d)", reward.Place, dest, reward.Map, reward.StandX, reward.StandY)
		}
	}
}

func TestOldRodRewardUsesWalkableGuruApproach(t *testing.T) {
	dest, ok := Place("vermilion old rod house")
	if !ok {
		t.Fatal("vermilion old rod house did not resolve")
	}
	// The old (3,4) destination is a wall in the HOUSE tileset. The guru is
	// fixed at (2,4); (2,5) is the directly-adjacent walkable approach tile.
	want := (Destination{Map: 0xA3, X: 2, Y: 5})
	if dest != want {
		t.Fatalf("old rod destination = %+v, want %+v", dest, want)
	}
}

func TestSuperRodRewardUsesWalkableGuruApproach(t *testing.T) {
	dest, ok := Place("route 12 super rod house")
	if !ok {
		t.Fatal("route 12 super rod house did not resolve")
	}
	// Map 0xBD's generated collision grid has walls at (3,4) and (4,4).
	// The guru is fixed at (2,4), so (2,5) is the adjacent open floor tile.
	want := (Destination{Map: 0xBD, X: 2, Y: 5})
	if dest != want {
		t.Fatalf("super rod destination = %+v, want %+v", dest, want)
	}
}

func TestChoiceRewardThresholdsMatchRedAides(t *testing.T) {
	want := map[string]int{
		"hm05":       10,
		"itemfinder": 30,
		"exp all":    50,
	}
	for _, reward := range ChoiceRewards() {
		if threshold, ok := want[reward.ItemName]; ok && reward.MinOwned != threshold {
			t.Fatalf("%s MinOwned = %d, want %d", reward.ItemName, reward.MinOwned, threshold)
		}
	}
}
