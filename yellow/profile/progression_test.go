package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/yellow/sym"
)

func setYellowEvent(mem *fakeMemory, event yellowEvent) {
	index := uint16(event)
	mem[sym.EventFlags+index/8] |= 1 << uint(index%8)
}

func TestYellowStarterAndRivalBranchProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  byte
		want game.ProgressID
	}{
		{"jolteon", rivalStarterJolteon, ProgressYellowRivalJolteonPath},
		{"flareon", rivalStarterFlareon, ProgressYellowRivalFlareonPath},
		{"vaporeon", rivalStarterVaporeon, ProgressYellowRivalVaporeonPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mem fakeMemory
			mem[sym.PartyCount] = 1
			mem[sym.PartyMon1] = 0x54
			mem[sym.RivalStarter] = tc.raw
			mem[sym.PikachuHappiness] = 90
			mem[sym.PikachuSpawnStateFlags] = 1<<pikachuStarterBit | 1<<pikachuFollowingBit
			setYellowEvent(&mem, eventGotStarter)
			setYellowEvent(&mem, eventBattledRivalInOaksLab)

			story := projectYellowStory(&mem, 0x00)
			if !story.Has(ProgressYellowStarterReceived) || !story.Has(ProgressYellowLabRivalResolved) {
				t.Fatalf("starter facts missing: %+v", story)
			}
			if !story.Has(tc.want) {
				t.Fatalf("rival branch %q not projected: %+v", tc.want, story)
			}
			if got, ok := story.Value(ProgressYellowPikachuHappiness); !ok || got != 90 {
				t.Fatalf("Pikachu happiness = %d,%v, want 90,true", got, ok)
			}
			if !story.Has(ProgressYellowPikachuStarterPresent) || !story.Has(ProgressYellowPikachuFollowing) {
				t.Fatalf("Pikachu follower facts missing: %+v", story)
			}
		})
	}
}

func TestYellowRivalBranchIsNotInferredBeforeLabBattle(t *testing.T) {
	var mem fakeMemory
	mem[sym.RivalStarter] = rivalStarterJolteon
	story := projectYellowStory(&mem, 0x00)
	if story.Has(ProgressYellowRivalJolteonPath) {
		t.Fatal("temporary pre-battle Jolteon value was treated as committed rival path")
	}
}

func TestYellowJessieJamesGatesMtMoonAndSilphProgress(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	setYellowEvent(&mem, eventGotDomeFossil)
	setYellowEvent(&mem, eventBeatMtMoonSuperNerd)
	setYellowEvent(&mem, eventBeatSilphGiovanni)
	setYellowEvent(&mem, eventGotMasterBall)

	story := projectYellowStory(&mem, 0x00)
	if !story.Has(gen1.ProgressMtMoonFossilAcquired) {
		t.Fatal("fossil acquisition was hidden before Jessie/James")
	}
	if story.Has(ProgressYellowMtMoonExitResolved) {
		t.Fatal("Mt. Moon exit resolved before Jessie/James")
	}
	if story.Has(gen1.ProgressSilphCoCleared) {
		t.Fatal("Silph progression completed before Jessie/James")
	}

	setYellowEvent(&mem, eventBeatMtMoonJessieJames)
	setYellowEvent(&mem, eventBeatSilphJessieJames)
	story = projectYellowStory(&mem, 0x00)
	if !story.Has(ProgressYellowMtMoonExitResolved) {
		t.Fatal("Mt. Moon exit did not resolve after fossil + Jessie/James")
	}
	if !story.Has(gen1.ProgressSilphCoCleared) || !story.Has(gen1.ProgressSilphRescueComplete) {
		t.Fatal("Silph progression did not complete after Yellow-specific gate")
	}
}

func TestYellowGiftAvailabilityUsesRealPrerequisites(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	mem[sym.PikachuHappiness] = 146

	story := projectYellowStory(&mem, 0x00)
	if story.Has(ProgressYellowBulbasaurGiftAvailable) {
		t.Fatal("Bulbasaur available below happiness 147")
	}
	if !story.Has(ProgressYellowCharmanderGiftAvailable) {
		t.Fatal("Charmander gift should be available with party room before receipt")
	}
	if story.Has(ProgressYellowSquirtleGiftAvailable) {
		t.Fatal("Squirtle available before Thunder Badge")
	}

	mem[sym.PikachuHappiness] = 147
	mem[sym.ObtainedBadges] |= 1 << badgeThunder
	story = projectYellowStory(&mem, 0x00)
	if !story.Has(ProgressYellowBulbasaurGiftAvailable) {
		t.Fatal("Bulbasaur not available at happiness 147")
	}
	if !story.Has(ProgressYellowSquirtleGiftAvailable) {
		t.Fatal("Squirtle not available after Thunder Badge")
	}

	setYellowEvent(&mem, eventGotBulbasaurInCerulean)
	setYellowEvent(&mem, eventGotCharmanderRoute24)
	setYellowEvent(&mem, eventGotSquirtleFromJenny)
	story = projectYellowStory(&mem, 0x00)
	if story.Has(ProgressYellowBulbasaurGiftAvailable) ||
		story.Has(ProgressYellowCharmanderGiftAvailable) ||
		story.Has(ProgressYellowSquirtleGiftAvailable) {
		t.Fatal("received Yellow gifts remained available")
	}
	if !story.Has(ProgressYellowBulbasaurGiftReceived) ||
		!story.Has(ProgressYellowCharmanderGiftReceived) ||
		!story.Has(ProgressYellowSquirtleGiftReceived) {
		t.Fatal("received Yellow gifts not projected")
	}
}

func TestYellowSharedKantoProgressComesFromYellowState(t *testing.T) {
	var mem fakeMemory
	mem[sym.PartyCount] = 1
	base := uint16(sym.PartyMon1)
	mem[base+0x01], mem[base+0x02] = 0, 20
	mem[base+0x22], mem[base+0x23] = 0, 20
	mem[base+0x08] = 33
	mem[base+0x1d] = 10

	mem[sym.NumBagItems] = 5
	items := []struct{ id, qty byte }{
		{itemSSTicket, 1},
		{itemHM01, 1},
		{itemCardKey, 1},
		{itemHM03, 1},
		{itemHM04, 1},
	}
	for i, item := range items {
		at := int(sym.BagItems) + i*2
		mem[at], mem[at+1] = item.id, item.qty
	}
	mem[sym.StatusFlags1] |= saffronGuardsDrinkMask
	mem[sym.ObtainedBadges] |= 1 << badgeSoul
	setYellowEvent(&mem, eventGotPokedex)
	setYellowEvent(&mem, eventViridianGymOpen)
	for _, event := range route23BadgeCheckEvents {
		setYellowEvent(&mem, event)
	}

	story := projectYellowStory(&mem, indigoPlateauMap)
	for _, id := range []game.ProgressID{
		gen1.ProgressPokedexAcquired,
		gen1.ProgressSSTicketAcquired,
		gen1.ProgressHM01Acquired,
		gen1.ProgressSaffronGateOpen,
		gen1.ProgressCardKeyOwned,
		gen1.ProgressFuchsiaProgressionComplete,
		gen1.ProgressViridianGymOpen,
		gen1.ProgressRoute23BadgeChecks,
		gen1.ProgressVictoryRoadCleared,
		gen1.ProgressIndigoPlateauReady,
	} {
		if !story.Has(id) {
			t.Errorf("shared progress %q not complete", id)
		}
	}
	if got, ok := story.Value(gen1.ProgressRoute23BadgeChecks); !ok || got != 7 {
		t.Fatalf("badge checks = %d,%v, want 7,true", got, ok)
	}
}

func TestYellowProjectsSharedLeagueRoomFacts(t *testing.T) {
	var mem fakeMemory
	for _, event := range []yellowEvent{
		eventAutowalkedIntoLorelei,
		eventBeatLorelei,
		eventBeatBruno,
		eventBeatAgatha,
		eventBeatLance,
	} {
		setYellowEvent(&mem, event)
	}
	story := projectYellowStory(&mem, indigoPlateauMap)
	for _, id := range []game.ProgressID{
		gen1.ProgressLeagueChallengeStarted,
		gen1.ProgressLeagueLoreleiDefeated,
		gen1.ProgressLeagueBrunoDefeated,
		gen1.ProgressLeagueAgathaDefeated,
		gen1.ProgressLeagueLanceDefeated,
	} {
		if !story.Has(id) {
			t.Errorf("shared League progress %q not complete", id)
		}
	}
	if story.Has(gen1.ProgressLeagueChampionDefeated) {
		t.Fatal("Champion progress completed before Champion event")
	}
}

func TestYellowMainStoryUsesDurableElite4Flag(t *testing.T) {
	var mem fakeMemory
	mem[sym.Elite4Flags] = elite4CompletedMask
	story := projectYellowStory(&mem, 0x00)
	if !story.Has(gen1.ProgressMainStoryComplete) ||
		!story.Has(gen1.ProgressLeagueChampionDefeated) ||
		!story.Has(gen1.ProgressLeagueChallengeStarted) {
		t.Fatalf("durable Hall-of-Fame state not projected: %+v", story)
	}
}

func TestYellowSelectedEventIndices(t *testing.T) {
	got := map[string]yellowEvent{
		"EVENT_GOT_STARTER":                        eventGotStarter,
		"EVENT_BATTLED_RIVAL_IN_OAKS_LAB":          eventBattledRivalInOaksLab,
		"EVENT_GOT_BULBASAUR_IN_CERULEAN":          eventGotBulbasaurInCerulean,
		"EVENT_BEAT_POKEMONTOWER_7_JESSIE_JAMES":   eventBeatTowerJessieJames,
		"EVENT_GOT_SQUIRTLE_FROM_OFFICER_JENNY":    eventGotSquirtleFromJenny,
		"EVENT_BEAT_MT_MOON_3_JESSIE_JAMES":        eventBeatMtMoonJessieJames,
		"EVENT_BEAT_ROCKET_HIDEOUT_4_JESSIE_JAMES": eventBeatRocketJessieJames,
		"EVENT_BEAT_SILPH_CO_11F_JESSIE_JAMES":     eventBeatSilphJessieJames,
		"EVENT_BEAT_CHAMPION_RIVAL":                eventBeatChampionRival,
	}
	want := map[string]yellowEvent{
		"EVENT_GOT_STARTER":                        34,
		"EVENT_BATTLED_RIVAL_IN_OAKS_LAB":          35,
		"EVENT_GOT_BULBASAUR_IN_CERULEAN":          168,
		"EVENT_BEAT_POKEMONTOWER_7_JESSIE_JAMES":   273,
		"EVENT_GOT_SQUIRTLE_FROM_OFFICER_JENNY":    327,
		"EVENT_BEAT_MT_MOON_3_JESSIE_JAMES":        1402,
		"EVENT_BEAT_ROCKET_HIDEOUT_4_JESSIE_JAMES": 1698,
		"EVENT_BEAT_SILPH_CO_11F_JESSIE_JAMES":     1924,
		"EVENT_BEAT_CHAMPION_RIVAL":                2305,
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Errorf("%s = %d, want %d", name, got[name], expected)
		}
	}
}
