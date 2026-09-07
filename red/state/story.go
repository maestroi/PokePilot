package state

import "github.com/maestroi/pokepilot/red/sym"

const (
	// wStatusFlags1 bit 6 is BIT_GAVE_SAFFRON_GUARDS_DRINK.
	saffronGuardsDrinkMask uint8 = 1 << 6

	// Item ids from pokered/constants/item_constants.asm.
	secretKeyItemID uint8 = 0x2b
	cardKeyItemID   uint8 = 0x30
)

// These event ids stay private to the Red story decoder. Agent/planner code
// consumes StoryFacts instead of learning raw event-bit numbers. The values
// are checked against the vendored event_constants.asm in story_test.go.
const (
	eventViridianGymOpen          Event = 0x028
	eventMansionSwitchOn          Event = 0x278
	eventBeatRoute22Rival2ndBattle Event = 0x526
	eventPassedCascadeBadgeCheck  Event = 0x530
	eventPassedThunderBadgeCheck  Event = 0x531
	eventPassedRainbowBadgeCheck  Event = 0x532
	eventPassedSoulBadgeCheck     Event = 0x533
	eventPassedMarshBadgeCheck    Event = 0x534
	eventPassedVolcanoBadgeCheck  Event = 0x535
	eventPassedEarthBadgeCheck    Event = 0x536
	eventBeatSilphCoGiovanni      Event = 0x78f
	eventAutowalkedIntoLoreleisRoom Event = 0x8e6
)

// StoryFacts is the compact semantic progression state exposed to the agent.
// It deliberately names game concepts, not event ids or WRAM bits, so the
// planner/operator can reason about gates without inheriting Red's encoding.
// Every field is deterministically re-derived from the current RAM/bag state.
type StoryFacts struct {
	SaffronGateOpen          bool
	CardKeyOwned             bool
	SilphCoCleared           bool
	MansionSwitchOn          bool
	SecretKeyOwned           bool
	ViridianGymOpen          bool
	Route22RivalResolved     bool
	Route23BadgeChecksPassed int
	Route23BadgeChecksComplete bool
	LeagueChallengeStarted   bool
	LeagueChampionDefeated   bool
}

var route23BadgeCheckEvents = [...]Event{
	eventPassedCascadeBadgeCheck,
	eventPassedThunderBadgeCheck,
	eventPassedRainbowBadgeCheck,
	eventPassedSoulBadgeCheck,
	eventPassedMarshBadgeCheck,
	eventPassedVolcanoBadgeCheck,
	eventPassedEarthBadgeCheck,
}

// DecodeStoryFacts derives planner-facing progression semantics from Red's
// authoritative RAM and decoded inventory. No run memory is involved, so the
// same checkpoint always reconstructs the same facts after resume.
func DecodeStoryFacts(m *Mem, inv InventoryState) StoryFacts {
	facts := StoryFacts{
		SaffronGateOpen:        m.U8(sym.StatusFlags1)&saffronGuardsDrinkMask != 0,
		CardKeyOwned:           inventoryHasItem(inv, cardKeyItemID),
		SilphCoCleared:         HasEvent(m, eventBeatSilphCoGiovanni),
		MansionSwitchOn:        HasEvent(m, eventMansionSwitchOn),
		SecretKeyOwned:         inventoryHasItem(inv, secretKeyItemID),
		ViridianGymOpen:        HasEvent(m, eventViridianGymOpen),
		Route22RivalResolved:   HasEvent(m, eventBeatRoute22Rival2ndBattle),
		LeagueChallengeStarted: HasEvent(m, eventAutowalkedIntoLoreleisRoom),
		LeagueChampionDefeated: HasEvent(m, EventBeatChampionRival),
	}
	for _, event := range route23BadgeCheckEvents {
		if HasEvent(m, event) {
			facts.Route23BadgeChecksPassed++
		}
	}
	facts.Route23BadgeChecksComplete = facts.Route23BadgeChecksPassed == len(route23BadgeCheckEvents)
	if facts.LeagueChampionDefeated {
		facts.LeagueChallengeStarted = true
	}
	return facts
}

func inventoryHasItem(inv InventoryState, id uint8) bool {
	for _, item := range inv.Items {
		if item.ID == id && item.Quantity > 0 {
			return true
		}
	}
	return false
}
