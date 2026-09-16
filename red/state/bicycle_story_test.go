package state

import "testing"

func TestDecodeStoryFactsBicycleAcquiredFromBag(t *testing.T) {
	var mem Mem
	owned := DecodeStoryFacts(&mem, InventoryState{Items: []BagItem{{ID: bicycleItemID, Quantity: 1}}})
	if !owned.BicycleAcquired {
		t.Fatalf("BicycleAcquired = false with Bicycle in bag: %+v", owned)
	}

	missing := DecodeStoryFacts(&mem, InventoryState{})
	if missing.BicycleAcquired {
		t.Fatalf("BicycleAcquired = true without Bicycle in bag: %+v", missing)
	}
}
