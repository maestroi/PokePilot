package agent

import "github.com/maestroi/pokepilot/skill"

// redNPCRewardItemID resolves the raw Red item byte for a semantic NPC reward
// objective without widening the generic planner-executable item vocabulary.
// Economy/observation code intentionally knows about many key items that the
// planner must not be able to manufacture as arbitrary pickup/use arguments.
//
// A reward item is therefore executable only when the objective carries the
// reward intent and exactly matches the trusted map/actor/item tuple from the
// skill-owned reward table.
func redNPCRewardItemID(mapID uint8, o Objective) (uint8, bool) {
	if o.Kind != KindPickup || o.Intent != redNPCRewardIntent {
		return 0, false
	}
	for _, reward := range skill.ChoiceRewards() {
		if reward.Map == mapID && reward.X == o.X && reward.Y == o.Y && ItemID(reward.ItemName) == o.Item {
			return reward.Item, true
		}
	}
	return 0, false
}

func (a *redObjectiveAdapter) resolvePickupItemID(mapID uint8, o Objective) (uint8, bool) {
	if raw, ok := redNPCRewardItemID(mapID, o); ok {
		return raw, true
	}
	return a.resolveItemID(o.Item)
}
