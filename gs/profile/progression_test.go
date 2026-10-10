package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func setGSEvent(mem fakeGSReader, event uint16) {
	mem[sym.EventFlags+event/8] |= byte(1 << uint(event%8))
}

func TestEarlyJohtoProgressionProjectsDurableFlags(t *testing.T) {
	mem := fakeGSReader{}
	setGSEvent(mem, eventGotPokemonFromElm)
	setGSEvent(mem, eventGotMysteryEggMrPokemon)
	mem[sym.StatusFlags] = statusFlagsPokedexMask
	mem[sym.MrPokemonsHouseSceneID] = sceneMrPokemonsHouseNoop
	mem[sym.CherrygroveCitySceneID] = sceneCherrygroveNoop
	mem[sym.ElmsLabSceneID] = sceneElmsLabNoop
	setGSEvent(mem, eventGaveMysteryEggToElm)

	story := projectEarlyStory(mem)
	for _, id := range []game.ProgressID{
		ProgressStarterReceived,
		ProgressMysteryEggReceived,
		ProgressPokedexAcquired,
		ProgressCherrygroveRivalResolved,
		ProgressRivalNamed,
		ProgressMysteryEggReturned,
	} {
		if !story.Has(id) {
			t.Fatalf("story missing completed %q: %+v", id, story)
		}
	}
}

func TestEarlyJohtoProgressionDoesNotConfuseFreshNoopScenesWithCompletion(t *testing.T) {
	mem := fakeGSReader{
		sym.CherrygroveCitySceneID: sceneCherrygroveNoop,
		sym.ElmsLabSceneID:         sceneElmsLabNoop,
	}
	story := projectEarlyStory(mem)
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("fresh Cherrygrove noop scene falsely resolved rival")
	}
	if story.Has(ProgressRivalNamed) {
		t.Fatal("fresh Elm noop scene falsely resolved rival naming")
	}
}

func TestEarlyJohtoProgressionRequiresMrPokemonScriptToFinishBeforeRivalResolution(t *testing.T) {
	mem := fakeGSReader{
		sym.StatusFlags:            statusFlagsPokedexMask,
		sym.CherrygroveCitySceneID: sceneCherrygroveNoop,
	}
	setGSEvent(mem, eventGotMysteryEggMrPokemon)

	story := projectEarlyStory(mem)
	if !story.Has(ProgressMysteryEggReceived) || !story.Has(ProgressPokedexAcquired) {
		t.Fatalf("egg/dex not projected: %+v", story)
	}
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival resolved before Mr. Pokemon scene reached its durable noop state")
	}

	mem[sym.MrPokemonsHouseSceneID] = sceneMrPokemonsHouseNoop
	mem[sym.CherrygroveCitySceneID] = 1
	story = projectEarlyStory(mem)
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival resolved while Cherrygrove rival scene is still armed")
	}

	mem[sym.CherrygroveCitySceneID] = sceneCherrygroveNoop
	story = projectEarlyStory(mem)
	if !story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival did not resolve after Mr. Pokemon completion and Cherrygrove scene reset")
	}
}

func TestGSObservationIncludesMysteryEggReturnVerifier(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:       24,
		sym.MapNumber:      5,
		sym.MapWidth:       5,
		sym.MapHeight:      6,
		sym.MapStatus:      gen2MapStatusHandle,
		sym.MapEventStatus: gen2MapEventsOn,
	}
	setGSEvent(mem, eventGaveMysteryEggToElm)

	obs, err := NewGold().DecodeObservation(mem, nil)
	if err != nil {
		t.Fatalf("DecodeObservation: %v", err)
	}
	fact, ok := obs.Story.Lookup(ProgressMysteryEggReturned)
	if !ok || !fact.Complete {
		t.Fatalf("mystery egg return fact = %+v, ok=%v", fact, ok)
	}
}

func TestFirstBadgeProgressionProjectsSproutTowerZephyrAndEggBoundaries(t *testing.T) {
	mem := fakeGSReader{}
	if story := projectEarlyStory(mem); story.Has(ProgressSproutTowerCleared) || story.Has(ProgressZephyrBadgeEarned) || story.Has(ProgressTogepiEggReceived) {
		t.Fatalf("fresh story already completed early-Johto milestones: %+v", story)
	}

	setGSEvent(mem, eventGotHM05Flash)
	story := projectEarlyStory(mem)
	if !story.Has(ProgressSproutTowerCleared) {
		t.Fatalf("FLASH handoff did not complete Sprout Tower: %+v", story)
	}
	if story.Has(ProgressZephyrBadgeEarned) {
		t.Fatal("Sprout Tower completion falsely granted Zephyr Badge")
	}

	mem[sym.JohtoBadges] |= johtoBadgeZephyrMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressZephyrBadgeEarned) {
		t.Fatalf("Zephyr badge bit did not complete milestone: %+v", story)
	}

	setGSEvent(mem, eventGotTogepiEggFromElmsAide)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressTogepiEggReceived) {
		t.Fatalf("Togepi Egg event did not complete post-Falkner handoff: %+v", story)
	}

	setGSEvent(mem, eventClearedSlowpokeWell)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressSlowpokeWellCleared) {
		t.Fatalf("Slowpoke Well event did not complete Rocket clear milestone: %+v", story)
	}
	if story.Has(ProgressHiveBadgeEarned) {
		t.Fatal("Slowpoke Well completion falsely granted Hive Badge")
	}

	mem[sym.JohtoBadges] |= johtoBadgeHiveMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressHiveBadgeEarned) {
		t.Fatalf("Hive badge bit did not complete Bugsy milestone: %+v", story)
	}
}

func TestEarlyJohtoEventIndicesMatchPinnedPokegoldConstants(t *testing.T) {
	// constants/event_flags.asm at gs/data.SourceRevision. These literals are
	// intentional: using the production constants as both setup and expected
	// value would not catch an off-by-one event binding.
	if eventGotHM03Surf != 18 {
		t.Fatalf("EVENT_GOT_HM03_SURF = %d, want 18", eventGotHM03Surf)
	}
	if eventGotHM05Flash != 20 {
		t.Fatalf("EVENT_GOT_HM05_FLASH = %d, want 20", eventGotHM05Flash)
	}
	if eventGotTogepiEggFromElmsAide != 45 {
		t.Fatalf("EVENT_GOT_TOGEPI_EGG_FROM_ELMS_AIDE = %d, want 45", eventGotTogepiEggFromElmsAide)
	}
	if eventClearedSlowpokeWell != 43 {
		t.Fatalf("EVENT_CLEARED_SLOWPOKE_WELL = %d, want 43", eventClearedSlowpokeWell)
	}
}

func TestEarlyJohtoBadgeMasksMatchPinnedPokegoldEngineFlags(t *testing.T) {
	if johtoBadgeZephyrMask != 1<<0 {
		t.Fatalf("ENGINE_ZEPHYRBADGE mask = %#02x, want %#02x", johtoBadgeZephyrMask, 1<<0)
	}
	if johtoBadgeHiveMask != 1<<1 {
		t.Fatalf("ENGINE_HIVEBADGE mask = %#02x, want %#02x", johtoBadgeHiveMask, 1<<1)
	}
	if johtoBadgePlainMask != 1<<2 {
		t.Fatalf("ENGINE_PLAINBADGE mask = %#02x, want %#02x", johtoBadgePlainMask, 1<<2)
	}
	if johtoBadgeFogMask != 1<<3 {
		t.Fatalf("ENGINE_FOGBADGE mask = %#02x, want %#02x", johtoBadgeFogMask, 1<<3)
	}
}

func TestIlexProgressionRequiresPostWinAzaleaScene(t *testing.T) {
	mem := fakeGSReader{}
	mem[sym.JohtoBadges] = johtoBadgeHiveMask
	setGSEvent(mem, eventRivalAzaleaTown)
	mem[sym.AzaleaTownSceneID] = 1

	story := projectEarlyStory(mem)
	if story.Has(ProgressAzaleaRivalResolved) {
		t.Fatal("rival event set before battle falsely completed Azalea rival")
	}

	mem[sym.AzaleaTownSceneID] = sceneAzaleaTownNoop
	story = projectEarlyStory(mem)
	if !story.Has(ProgressAzaleaRivalResolved) {
		t.Fatalf("post-win NOOP scene did not complete Azalea rival: %+v", story)
	}

	setGSEvent(mem, eventHerdedFarfetchd)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressFarfetchdHerded) {
		t.Fatalf("Farfetchd herd event not projected: %+v", story)
	}
	if story.Has(ProgressHM01CutAcquired) {
		t.Fatal("Farfetchd completion falsely granted HM01")
	}

	setGSEvent(mem, eventGotHM01Cut)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressHM01CutAcquired) {
		t.Fatalf("HM01 Cut event not projected: %+v", story)
	}
	if story.Has(ProgressTM02HeadbuttAcquired) {
		t.Fatal("HM01 acquisition falsely crossed the north-Ilex Cut boundary")
	}

	setGSEvent(mem, eventGotTM02Headbutt)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressTM02HeadbuttAcquired) {
		t.Fatalf("TM02 Headbutt event not projected: %+v", story)
	}
	if story.Has(ProgressPlainBadgeEarned) {
		t.Fatal("north-Ilex Headbutt boundary falsely granted Plain Badge")
	}
}

func TestExperimentalFrontierTracksCurrentExecutableBoundary(t *testing.T) {
	mem := fakeGSReader{}
	story := projectEarlyStory(mem)
	if story.Has(ProgressSupportedFrontier) {
		t.Fatal("fresh save already reports the experimental Gen-II frontier complete")
	}

	setGSEvent(mem, eventGotTM02Headbutt)
	mem[sym.JohtoBadges] |= johtoBadgePlainMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressPlainBadgeEarned) {
		t.Fatalf("Plain Badge boundary missing from story: %+v", story)
	}
	if story.Has(ProgressSupportedFrontier) {
		t.Fatal("Plain Badge reached frontier before Ecruteak slice")
	}

	setGSEvent(mem, eventGotSquirtBottle)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressSquirtBottleAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("SquirtBottle boundary = %+v, want acquired but not frontier", story)
	}

	setGSEvent(mem, eventFoughtSudowoodo)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressSudowoodoCleared) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Sudowoodo boundary = %+v, want cleared but not frontier", story)
	}

	if story.Has(ProgressTM08RockSmashAcquired) {
		t.Fatal("TM08 reported before the Route 36 guy hands it over")
	}
	setGSEvent(mem, eventGotTM08RockSmash)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressTM08RockSmashAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("TM08 boundary = %+v, want acquired but not frontier", story)
	}

	setGSEvent(mem, eventReleasedTheBeasts)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressBurnedTowerCleared) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Burned Tower boundary = %+v, want cleared but not frontier", story)
	}

	mem[sym.JohtoBadges] |= johtoBadgeFogMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressFogBadgeEarned) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Fog Badge boundary = %+v, want earned but not frontier", story)
	}
	if story.Has(ProgressHM03SurfAcquired) {
		t.Fatal("HM03 Surf reported before the Dance Theater handoff")
	}

	setGSEvent(mem, eventGotHM03Surf)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressHM03SurfAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("HM03 Surf is acquired but must not finish the Olivine/Cianwood frontier: %+v", story)
	}

	setGSEvent(mem, eventJasmineExplainedAmphysSickness)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressLighthouseSicknessExplained) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Lighthouse request must not finish medicine quest: %+v", story)
	}
	setGSEvent(mem, eventGotSecretPotionFromPharmacy)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressSecretPotionAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("SecretPotion pickup is not Jasmine's cure: %+v", story)
	}
	setGSEvent(mem, eventJasmineReturnedToGym)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressJasmineReturnedToGym) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Jasmine cure must not complete the six-badge frontier: %+v", story)
	}
	setGSEvent(mem, eventGotHM04Strength)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressHM04StrengthAcquired) || story.Has(ProgressStormBadgeEarned) {
		t.Fatalf("HM04 pickup must not fake Chuck's badge: %+v", story)
	}
	mem[sym.JohtoBadges] |= johtoBadgeStormMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressStormBadgeEarned) || story.Has(ProgressHM02FlyAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Storm Badge must not grant Fly or finish the frontier: %+v", story)
	}
	setGSEvent(mem, eventGotHM02Fly)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressHM02FlyAcquired) || story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Fly pickup must not award Mineral Badge: %+v", story)
	}
	mem[sym.JohtoBadges] |= johtoBadgeMineralMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressMineralBadgeEarned) || !story.Has(ProgressSupportedFrontier) {
		t.Fatalf("Mineral Badge must finish the supported frontier: %+v", story)
	}
}

func TestPinnedIlexEventAndSceneConstants(t *testing.T) {
	if eventGotHM01Cut != 16 {
		t.Fatalf("EVENT_GOT_HM01_CUT = %d, want 16", eventGotHM01Cut)
	}
	if eventHerdedFarfetchd != 41 {
		t.Fatalf("EVENT_HERDED_FARFETCHD = %d, want 41", eventHerdedFarfetchd)
	}
	if eventGotTM02Headbutt != 95 {
		t.Fatalf("EVENT_GOT_TM02_HEADBUTT = %d, want 95", eventGotTM02Headbutt)
	}
	if eventRivalAzaleaTown != 1727 {
		t.Fatalf("EVENT_RIVAL_AZALEA_TOWN = %d, want 1727", eventRivalAzaleaTown)
	}
	if sym.AzaleaTownSceneID != 0xd6d4 {
		t.Fatalf("wAzaleaTownSceneID = %#04x, want %#04x", sym.AzaleaTownSceneID, 0xd6d4)
	}
}

func TestPinnedEcruteakEventConstants(t *testing.T) {
	if eventFoughtSudowoodo != 42 {
		t.Fatalf("EVENT_FOUGHT_SUDOWOODO = %d, want 42", eventFoughtSudowoodo)
	}
	if eventGotSquirtBottle != 92 {
		t.Fatalf("EVENT_GOT_SQUIRTBOTTLE = %d, want 92", eventGotSquirtBottle)
	}
	if eventReleasedTheBeasts != 123 {
		t.Fatalf("EVENT_RELEASED_THE_BEASTS = %d, want 123", eventReleasedTheBeasts)
	}
}

func TestPinnedOlivineCianwoodEventConstants(t *testing.T) {
	// Independently counted from the vendored pokegold event_flags.asm,
	// including the unused flags before the early story indices.
	if eventJasmineExplainedAmphysSickness != 55 ||
		eventGotSecretPotionFromPharmacy != 35 ||
		eventJasmineReturnedToGym != 32 {
		t.Fatalf("Olivine/Cianwood event indices = (%d,%d,%d), want (55,35,32)",
			eventJasmineExplainedAmphysSickness,
			eventGotSecretPotionFromPharmacy, eventJasmineReturnedToGym)
	}
}

func TestMedicineEventDoesNotRequireItemStillInBag(t *testing.T) {
	mem := fakeGSReader{}
	setGSEvent(mem, eventJasmineExplainedAmphysSickness)
	setGSEvent(mem, eventGotSecretPotionFromPharmacy)
	// Jasmine consumes the SecretPotion. Its historical acquisition remains
	// true while the proof of completed treatment is her separate event.
	story := projectEarlyStory(mem)
	if !story.Has(ProgressSecretPotionAcquired) || story.Has(ProgressJasmineReturnedToGym) {
		t.Fatalf("SecretPotion provenance was coupled to current inventory: %+v", story)
	}
	setGSEvent(mem, eventJasmineReturnedToGym)
	story = projectEarlyStory(mem)
	if !story.Has(ProgressJasmineReturnedToGym) || story.Has(ProgressSupportedFrontier) {
		t.Fatal("Jasmine's medicine treatment must remain durable without claiming a Mineral Badge")
	}
}

func TestPinnedCoastalHMAndBadgeFacts(t *testing.T) {
	// pokegold/constants/event_flags.asm has HM02 Fly at 17 and HM04 Strength
	// at 19; constants/engine_flags.asm puts Mineral before Storm in wJohtoBadges.
	if eventGotHM02Fly != 17 || eventGotHM04Strength != 19 {
		t.Fatalf("HM02/HM04 event indices = (%d,%d), want (17,19)", eventGotHM02Fly, eventGotHM04Strength)
	}
	if johtoBadgeMineralMask != 1<<4 || johtoBadgeStormMask != 1<<5 {
		t.Fatalf("Mineral/Storm mask = (%#x,%#x), want (0x10,0x20)", johtoBadgeMineralMask, johtoBadgeStormMask)
	}
}

func TestCoastalMilestonesNeverInferBadgeFromWinEventAlone(t *testing.T) {
	mem := fakeGSReader{}
	// Both trainer scripts set EVENT_BEAT_* separately from the engine badge
	// bits; a script interrupted before setflag cannot manufacture a badge.
	setGSEvent(mem, 663) // EVENT_BEAT_JASMINE
	setGSEvent(mem, 664) // EVENT_BEAT_CHUCK
	story := projectEarlyStory(mem)
	if story.Has(ProgressMineralBadgeEarned) || story.Has(ProgressStormBadgeEarned) ||
		story.Has(ProgressSupportedFrontier) {
		t.Fatalf("battle events alone must not report badge victory: %+v", story)
	}
	mem[sym.JohtoBadges] |= johtoBadgeMineralMask
	story = projectEarlyStory(mem)
	if !story.Has(ProgressMineralBadgeEarned) || !story.Has(ProgressSupportedFrontier) {
		t.Fatalf("engine badge must report completed Mineral Badge: %+v", story)
	}
}
