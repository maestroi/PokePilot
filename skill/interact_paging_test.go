package skill

import "testing"

// TestDialoguePagingStuckResetsWhenTextAdvances is the productive half of
// Talk's give-up rule: a box that is still drawing or turning the page is
// not stuck, however many A presses it has already consumed.
func TestDialoguePagingStuckResetsWhenTextAdvances(t *testing.T) {
	got, stuck := dialoguePagingStuck(5, "Hi", "Hiya")
	if stuck {
		t.Fatal("advancing text must not count as a stuck box")
	}
	if got != 0 {
		t.Fatalf("unchanged = %d, want 0 after the text advanced", got)
	}
}

// TestDialoguePagingStuckGivesUpOnlyAfterFrozenText is the farm failure
// from run-2axaf02lt07e25vpblunugp6u round 54: Bill's S.S. Ticket speech
// was still typing "They invit" when Talk hit its 30-press cap. The cap
// must fire only once the same screen has been paged with no change,
// which is how a jammed box looks; the key-item jingle freezes the
// "received an S.S.TICKET!" line for a few settles and then continues.
func TestDialoguePagingStuckGivesUpOnlyAfterFrozenText(t *testing.T) {
	const frozen = "A×7 received an S.S.TICKET!"
	unchanged := 0
	stuck := false
	for i := 0; i < talkPressCap-1; i++ {
		unchanged, stuck = dialoguePagingStuck(unchanged, frozen, frozen)
		if stuck {
			t.Fatalf("stuck after %d frozen pages, want cap %d", i+1, talkPressCap)
		}
	}
	unchanged, stuck = dialoguePagingStuck(unchanged, frozen, frozen)
	if !stuck {
		t.Fatal("want stuck after talkPressCap frozen pages")
	}
	if unchanged != talkPressCap {
		t.Fatalf("unchanged = %d, want %d", unchanged, talkPressCap)
	}

	unchanged, stuck = dialoguePagingStuck(unchanged, frozen, "That cruise ship")
	if stuck || unchanged != 0 {
		t.Fatalf("after the next page: unchanged=%d stuck=%v, want 0, false", unchanged, stuck)
	}
}
