package state

import "testing"

func TestDecodeStoryFactsProjectsSSTicket(t *testing.T) {
	var mem Mem
	without := DecodeStoryFacts(&mem, InventoryState{})
	if without.SSTicketAcquired {
		t.Fatal("empty inventory unexpectedly projects S.S. Ticket progression")
	}

	with := DecodeStoryFacts(&mem, InventoryState{Items: []BagItem{{ID: ssTicketItemID, Quantity: 1}}})
	if !with.SSTicketAcquired {
		t.Fatalf("S.S. Ticket in inventory did not project story fact: %+v", with)
	}
}
