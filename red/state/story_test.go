package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeStoryFacts(t *testing.T) {
	var mem Mem
	inv := InventoryState{Items: []BagItem{
		{ID: cardKeyItemID, Quantity: 1},
		{ID: secretKeyItemID, Quantity: 1},
	}}
	mem[sym.StatusFlags1] |= saffronGuardsDrinkMask
	for _, event := range []Event{
		eventViridianGymOpen,
		eventMansionSwitchOn,
		eventBeatRoute22Rival2ndBattle,
		eventBeatSilphCoGiovanni,
		eventAutowalkedIntoLoreleisRoom,
		EventBeatChampionRival,
	} {
		setTestEvent(&mem, event)
	}
	for _, event := range route23BadgeCheckEvents {
		setTestEvent(&mem, event)
	}

	got := DecodeStoryFacts(&mem, inv)
	if !got.SaffronGateOpen || !got.CardKeyOwned || !got.SilphCoCleared {
		t.Fatalf("Saffron/Silph facts = %+v", got)
	}
	if !got.MansionSwitchOn || !got.SecretKeyOwned {
		t.Fatalf("Mansion facts = %+v", got)
	}
	if !got.ViridianGymOpen || !got.Route22RivalResolved {
		t.Fatalf("late-route facts = %+v", got)
	}
	if got.Route23BadgeChecksPassed != len(route23BadgeCheckEvents) || !got.Route23BadgeChecksComplete {
		t.Fatalf("Route 23 facts = %+v", got)
	}
	if !got.LeagueChallengeStarted || !got.LeagueChampionDefeated {
		t.Fatalf("League facts = %+v", got)
	}
}

func TestDecodeStoryFactsRoute23PartialAndChampionImpliesLeagueStarted(t *testing.T) {
	var mem Mem
	setTestEvent(&mem, eventPassedCascadeBadgeCheck)
	setTestEvent(&mem, eventPassedThunderBadgeCheck)
	setTestEvent(&mem, EventBeatChampionRival)

	got := DecodeStoryFacts(&mem, InventoryState{})
	if got.Route23BadgeChecksPassed != 2 || got.Route23BadgeChecksComplete {
		t.Fatalf("partial Route 23 facts = %+v", got)
	}
	if !got.LeagueChallengeStarted || !got.LeagueChampionDefeated {
		t.Fatalf("Champion should imply League started: %+v", got)
	}
}

func TestStoryEventIndicesMatchDecomp(t *testing.T) {
	events := parseEventConstants(t)
	pairs := []struct {
		label string
		event Event
	}{
		{"EVENT_VIRIDIAN_GYM_OPEN", eventViridianGymOpen},
		{"EVENT_MANSION_SWITCH_ON", eventMansionSwitchOn},
		{"EVENT_BEAT_ROUTE22_RIVAL_2ND_BATTLE", eventBeatRoute22Rival2ndBattle},
		{"EVENT_PASSED_CASCADEBADGE_CHECK", eventPassedCascadeBadgeCheck},
		{"EVENT_PASSED_THUNDERBADGE_CHECK", eventPassedThunderBadgeCheck},
		{"EVENT_PASSED_RAINBOWBADGE_CHECK", eventPassedRainbowBadgeCheck},
		{"EVENT_PASSED_SOULBADGE_CHECK", eventPassedSoulBadgeCheck},
		{"EVENT_PASSED_MARSHBADGE_CHECK", eventPassedMarshBadgeCheck},
		{"EVENT_PASSED_VOLCANOBADGE_CHECK", eventPassedVolcanoBadgeCheck},
		{"EVENT_PASSED_EARTHBADGE_CHECK", eventPassedEarthBadgeCheck},
		{"EVENT_BEAT_SILPH_CO_GIOVANNI", eventBeatSilphCoGiovanni},
		{"EVENT_AUTOWALKED_INTO_LORELEIS_ROOM", eventAutowalkedIntoLoreleisRoom},
	}
	for _, pair := range pairs {
		want, ok := events[pair.label]
		if !ok {
			t.Errorf("label %s not found in %s", pair.label, eventConstantsFile)
			continue
		}
		if uint16(pair.event) != want {
			t.Errorf("%s = %#x, want %#x", pair.label, uint16(pair.event), want)
		}
	}
}

func setTestEvent(mem *Mem, event Event) {
	addr := sym.EventFlags + uint16(event)/8
	mem[addr] |= 1 << (uint16(event) % 8)
}
