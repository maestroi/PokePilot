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
