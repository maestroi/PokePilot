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
		if dest.Map != reward.Map || dest.Kind != DestinationInteraction || dest.X != reward.X || dest.Y != reward.Y {
			t.Fatalf("reward place %q = %+v, want interaction actor on map %#02x at (%d,%d)", reward.Place, dest, reward.Map, reward.X, reward.Y)
		}
		foundStand := false
		for _, target := range destinationRouteTargets(dest) {
			if target.X == int(reward.StandX) && target.Y == int(reward.StandY) {
				foundStand = true
				break
			}
		}
		if !foundStand {
			t.Fatalf("reward place %q interaction targets omit known stand (%d,%d)", reward.Place, reward.StandX, reward.StandY)
		}
	}
}

func TestRodRewardsKeepKnownWalkableApproachesAsInteractionCandidates(t *testing.T) {
	for _, name := range []string{"vermilion old rod house", "fuchsia good rod house", "route 12 super rod house"} {
		var reward ChoiceReward
		found := false
		for _, candidate := range ChoiceRewards() {
			if candidate.Place == name {
				reward, found = candidate, true
				break
			}
		}
		if !found {
			t.Fatalf("missing choice reward %q", name)
		}
		dest, ok := Place(name)
		if !ok {
			t.Fatalf("%s did not resolve", name)
		}
		seen := false
		for _, target := range destinationRouteTargets(dest) {
			if target.X == int(reward.StandX) && target.Y == int(reward.StandY) {
				seen = true
				break
			}
		}
		if !seen {
			t.Fatalf("%s interaction does not include known walkable approach (%d,%d)", name, reward.StandX, reward.StandY)
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
