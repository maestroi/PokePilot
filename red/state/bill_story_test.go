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

func TestDecodeStoryFactsProjectsSSTicketFromDurableMarker(t *testing.T) {
	var mem Mem
	// A completed Bill rescue: EVENT_GOT_SS_TICKET is set (the ROM sets it
	// only after the ticket is given) but the ticket is not in the bag —
	// stored in the PC after the S.S. Anne sailed. The fact must agree with
	// the Bill skill's positive postcondition, not the bag alone.
	setTestEvent(&mem, EventGotSSTicket)
	facts := DecodeStoryFacts(&mem, InventoryState{})
	if !facts.SSTicketAcquired {
		t.Fatalf("EVENT_GOT_SS_TICKET set with an empty inventory did not project S.S. Ticket progression: %+v", facts)
	}
}

func TestDecodeStoryFactsProjectsHM01(t *testing.T) {
	var mem Mem
	without := DecodeStoryFacts(&mem, InventoryState{})
	if without.HM01Acquired {
		t.Fatal("empty inventory unexpectedly projects HM01 progression")
	}

	with := DecodeStoryFacts(&mem, InventoryState{Items: []BagItem{{ID: hm01ItemID, Quantity: 1}}})
	if !with.HM01Acquired {
		t.Fatalf("HM01 in inventory did not project story fact: %+v", with)
	}
}
