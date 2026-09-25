package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

type yellowEvent uint16

const (
	eventFollowedOakIntoLab      yellowEvent = 0
	eventOakAskedToChooseMon     yellowEvent = 33
	eventGotStarter              yellowEvent = 34
	eventBattledRivalInOaksLab   yellowEvent = 35
	eventGotPokedex              yellowEvent = 37
	eventOakAppearedInPallet     yellowEvent = 39
	eventViridianGymOpen         yellowEvent = 40
	eventGotBulbasaurInCerulean  yellowEvent = 168
	eventBeatTowerJessieJames    yellowEvent = 275
	eventGotSquirtleFromJenny    yellowEvent = 329
	eventMansionSwitchOn         yellowEvent = 632
	eventRescuedMrFuji           yellowEvent = 1231
	eventBeatRoute22Rival2       yellowEvent = 1318
	eventPassedCascadeBadgeCheck yellowEvent = 1328
	eventPassedThunderBadgeCheck yellowEvent = 1329
	eventPassedRainbowBadgeCheck yellowEvent = 1330
	eventPassedSoulBadgeCheck    yellowEvent = 1331
	eventPassedMarshBadgeCheck   yellowEvent = 1332
	eventPassedVolcanoBadgeCheck yellowEvent = 1333
	eventPassedEarthBadgeCheck   yellowEvent = 1334
	eventGotCharmanderRoute24    yellowEvent = 0x54f
	eventGotDomeFossil           yellowEvent = 1400
	eventBeatMtMoonSuperNerd     yellowEvent = 1401
	eventBeatMtMoonJessieJames   yellowEvent = 1402
	eventGotHelixFossil          yellowEvent = 1407
	eventBeatRocketJessieJames   yellowEvent = 1698
	eventRocketHideoutDoorOpen   yellowEvent = 1701
	eventBeatRocketGiovanni      yellowEvent = 1703
	eventBeatSilphRival          yellowEvent = 1856
	eventBeatSilphJessieJames    yellowEvent = 1924
	eventGotMasterBall           yellowEvent = 1933
	eventBeatSilphGiovanni       yellowEvent = 1935
	eventBeatLorelei             yellowEvent = 2273
	eventAutowalkedIntoLorelei   yellowEvent = 2278
	eventBeatBruno               yellowEvent = 2281
	eventBeatAgatha              yellowEvent = 2289
	eventBeatLanceTrainer        yellowEvent = 2297
	eventBeatLance               yellowEvent = 2302
	eventBeatChampionRival       yellowEvent = 2305
)

const (
	ProgressYellowStarterReceived           game.ProgressID = "yellow_starter_received"
	ProgressYellowLabRivalResolved          game.ProgressID = "yellow_lab_rival_resolved"
	ProgressYellowRivalJolteonPath          game.ProgressID = "yellow_rival_jolteon_path"
	ProgressYellowRivalFlareonPath          game.ProgressID = "yellow_rival_flareon_path"
	ProgressYellowRivalVaporeonPath         game.ProgressID = "yellow_rival_vaporeon_path"
	ProgressYellowPikachuHappiness          game.ProgressID = "yellow_pikachu_happiness"
	ProgressYellowPikachuStarterPresent     game.ProgressID = "yellow_pikachu_starter_present"
	ProgressYellowPikachuFollowing          game.ProgressID = "yellow_pikachu_following"
	ProgressYellowPikachuSurfing            game.ProgressID = "yellow_pikachu_surfing"
	ProgressYellowMtMoonJessieJamesDefeated game.ProgressID = "yellow_mt_moon_jessie_james_defeated"
	ProgressYellowMtMoonExitResolved        game.ProgressID = "yellow_mt_moon_exit_resolved"
	ProgressYellowRocketJessieJamesDefeated game.ProgressID = "yellow_rocket_hideout_jessie_james_defeated"
	ProgressYellowTowerJessieJamesDefeated  game.ProgressID = "yellow_pokemon_tower_jessie_james_defeated"
	ProgressYellowSilphJessieJamesDefeated  game.ProgressID = "yellow_silph_jessie_james_defeated"
	ProgressYellowBulbasaurGiftAvailable    game.ProgressID = "yellow_bulbasaur_gift_available"
	ProgressYellowBulbasaurGiftReceived     game.ProgressID = "yellow_bulbasaur_gift_received"
	ProgressYellowCharmanderGiftAvailable   game.ProgressID = "yellow_charmander_gift_available"
	ProgressYellowCharmanderGiftReceived    game.ProgressID = "yellow_charmander_gift_received"
	ProgressYellowSquirtleGiftAvailable     game.ProgressID = "yellow_squirtle_gift_available"
	ProgressYellowSquirtleGiftReceived      game.ProgressID = "yellow_squirtle_gift_received"
)

const (
	rivalStarterJolteon  = 1
	rivalStarterFlareon  = 2
	rivalStarterVaporeon = 3

	pikachuFollowingBit = 5
	pikachuSurfingBit   = 6
	pikachuStarterBit   = 7

	saffronGuardsDrinkMask = 1 << 6
	elite4CompletedMask    = 1 << 0

	itemBicycle    = 0x06
	itemSecretKey  = 0x2b
	itemCardKey    = 0x30
	itemSSTicket   = 0x3f
	itemSilphScope = 0x48
	itemPokeFlute  = 0x49
	itemHM01       = 0xc4
	itemHM03       = 0xc6
	itemHM04       = 0xc7

	badgeThunder = 2
	badgeRainbow = 3
	badgeSoul    = 4
	badgeVolcano = 6
	badgeEarth   = 7

	route23Map        = 0x22
	route23NorthCaveY = 31
	indigoPlateauMap  = 0x09
	indigoLobbyMap    = 0xae
)

var yellowObservedEvents = []struct {
	event yellowEvent
	name  string
}{
	{eventGotStarter, "GotStarter"},
	{eventBattledRivalInOaksLab, "BattledRivalInOaksLab"},
	{eventGotPokedex, "GotPokedex"},
	{eventBeatMtMoonJessieJames, "BeatMtMoonJessieJames"},
	{eventBeatRocketJessieJames, "BeatRocketHideoutJessieJames"},
	{eventBeatTowerJessieJames, "BeatPokemonTowerJessieJames"},
	{eventBeatSilphJessieJames, "BeatSilphJessieJames"},
	{eventBeatChampionRival, "BeatChampionRival"},
}

var route23BadgeCheckEvents = [...]yellowEvent{
	eventPassedCascadeBadgeCheck,
	eventPassedThunderBadgeCheck,
	eventPassedRainbowBadgeCheck,
	eventPassedSoulBadgeCheck,
	eventPassedMarshBadgeCheck,
	eventPassedVolcanoBadgeCheck,
	eventPassedEarthBadgeCheck,
}

func yellowHasEvent(reader game.MemoryReader, event yellowEvent) bool {
	if reader == nil {
		return false
	}
	index := uint16(event)
	return reader.Peek8(sym.EventFlags+index/8)&(1<<uint(index%8)) != 0
}

func yellowEventNames(reader game.MemoryReader) []string {
	out := make([]string, 0, len(yellowObservedEvents))
	for _, observed := range yellowObservedEvents {
		if yellowHasEvent(reader, observed.event) {
			out = append(out, observed.name)
		}
	}
	return out
}

func yellowHasItem(reader game.MemoryReader, id uint8) bool {
	for _, item := range gen1.DecodeBag(reader, yellowRAMLayout) {
		if item.ID == id && item.Quantity > 0 {
			return true
		}
	}
	return false
}

func yellowHasBadge(reader game.MemoryReader, bit uint8) bool {
	return reader != nil && reader.Peek8(sym.ObtainedBadges)&(1<<bit) != 0
}

func yellowPartyHasRoom(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	return reader.Peek8(sym.PartyCount) < 6
}

func yellowPartyRecovered(reader game.MemoryReader) bool {
	if reader == nil {
		return false
	}
	count := int(reader.Peek8(sym.PartyCount))
	if count <= 0 || count > 6 {
		return false
	}
	for slot := 0; slot < count; slot++ {
		base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
		hp := uint16(reader.Peek8(base+0x01))<<8 | uint16(reader.Peek8(base+0x02))
		maxHP := uint16(reader.Peek8(base+0x22))<<8 | uint16(reader.Peek8(base+0x23))
		if maxHP == 0 || hp != maxHP || reader.Peek8(base+0x04) != 0 {
			return false
		}
		for move := uint16(0); move < 4; move++ {
			if reader.Peek8(base+0x08+move) != 0 && reader.Peek8(base+0x1d+move)&0x3f == 0 {
				return false
			}
		}
	}
	return true
}

func yellowPostSurgeCeladonArea(mapID uint8) bool {
	name := yellowrom.MapName(mapID)
	return strings.HasPrefix(name, "CELADON_") ||
		name == "GAME_CORNER" ||
		strings.HasPrefix(name, "GAME_CORNER_") ||
		strings.HasPrefix(name, "ROCKET_HIDEOUT_")
}

func yellowPostSurgeLavenderReached(mapID uint8) bool {
	name := yellowrom.MapName(mapID)
	switch mapID {
	case 0x04, 0x13, 0x4f, 0x50, 0x79, 0x4d, 0x4e, 0x4c, 0x12, 0x0a:
		return true
	}
	return strings.HasPrefix(name, "LAVENDER_") ||
		strings.HasPrefix(name, "POKEMON_TOWER_") ||
		name == "MR_FUJIS_HOUSE" ||
		strings.HasPrefix(name, "SAFFRON_") ||
		strings.HasPrefix(name, "SILPH_CO_") ||
		yellowPostSurgeCeladonArea(mapID)
}

func yellowVictoryRoadCleared(reader game.MemoryReader, mapID uint8, badgeChecksComplete, leagueStarted, champion, mainComplete bool) bool {
	if leagueStarted || champion || mainComplete {
		return true
	}
	if !badgeChecksComplete {
		return false
	}
	switch mapID {
	case indigoPlateauMap, indigoLobbyMap:
		return true
	case route23Map:
		return int(reader.Peek8(sym.YCoord)) <= route23NorthCaveY
	default:
		return false
	}
}

func projectYellowStory(reader game.MemoryReader, mapID uint8) game.ProgressState {
	if reader == nil {
		return game.ProgressState{}
	}

	mainComplete := reader.Peek8(sym.Elite4Flags)&elite4CompletedMask != 0
	labRivalResolved := yellowHasEvent(reader, eventBattledRivalInOaksLab)
	rival := reader.Peek8(sym.RivalStarter)
	pikaFlags := reader.Peek8(sym.PikachuSpawnStateFlags)
	happiness := int(reader.Peek8(sym.PikachuHappiness))

	mtMoonJJ := yellowHasEvent(reader, eventBeatMtMoonJessieJames)
	rocketJJ := yellowHasEvent(reader, eventBeatRocketJessieJames)
	towerJJ := yellowHasEvent(reader, eventBeatTowerJessieJames)
	silphJJ := yellowHasEvent(reader, eventBeatSilphJessieJames)

	fossil := yellowHasEvent(reader, eventBeatMtMoonSuperNerd) &&
		(yellowHasEvent(reader, eventGotDomeFossil) || yellowHasEvent(reader, eventGotHelixFossil))
	mtMoonExitResolved := fossil && mtMoonJJ
	pokedex := yellowHasEvent(reader, eventGotPokedex)
	hm03 := yellowHasItem(reader, itemHM03)
	hm04 := yellowHasItem(reader, itemHM04)
	silphCleared := yellowHasEvent(reader, eventBeatSilphGiovanni)
	masterBall := yellowHasEvent(reader, eventGotMasterBall)

	badgeChecks := 0
	for _, event := range route23BadgeCheckEvents {
		if yellowHasEvent(reader, event) {
			badgeChecks++
		}
	}
	badgeChecksComplete := badgeChecks == len(route23BadgeCheckEvents)

	leagueStarted := yellowHasEvent(reader, eventAutowalkedIntoLorelei)
	leagueLorelei := yellowHasEvent(reader, eventBeatLorelei) || mainComplete
	leagueBruno := yellowHasEvent(reader, eventBeatBruno) || mainComplete
	leagueAgatha := yellowHasEvent(reader, eventBeatAgatha) || mainComplete
	leagueLance := yellowHasEvent(reader, eventBeatLance) || mainComplete
	champion := yellowHasEvent(reader, eventBeatChampionRival) || mainComplete
	if leagueLorelei || leagueBruno || leagueAgatha || leagueLance || champion || mainComplete {
		leagueStarted = true
	}

	victoryRoadCleared := yellowVictoryRoadCleared(reader, mapID, badgeChecksComplete, leagueStarted, champion, mainComplete)
	leaguePastLobby := leagueStarted || champion || mainComplete
	indigoReady := leaguePastLobby || mapID == indigoPlateauMap || (mapID == indigoLobbyMap && yellowPartyRecovered(reader))

	gotBulbasaur := yellowHasEvent(reader, eventGotBulbasaurInCerulean)
	gotCharmander := yellowHasEvent(reader, eventGotCharmanderRoute24)
	gotSquirtle := yellowHasEvent(reader, eventGotSquirtleFromJenny)
	partyRoom := yellowPartyHasRoom(reader)

	return game.ProgressState{
		{ID: gen1.ProgressMtMoonFossilAcquired, Complete: fossil},
		{ID: gen1.ProgressPokedexAcquired, Complete: pokedex},
		{ID: gen1.ProgressSSTicketAcquired, Complete: yellowHasItem(reader, itemSSTicket)},
		{ID: gen1.ProgressHM01Acquired, Complete: yellowHasItem(reader, itemHM01)},
		{ID: gen1.ProgressBicycleAcquired, Complete: yellowHasItem(reader, itemBicycle)},
		{ID: gen1.ProgressThunderBadge, Complete: yellowHasBadge(reader, badgeThunder)},
		{ID: gen1.ProgressPostSurgeLavenderReached, Complete: yellowPostSurgeLavenderReached(mapID)},
		{ID: gen1.ProgressPostSurgeCeladonReady, Complete: yellowPostSurgeCeladonArea(mapID) && yellowPartyRecovered(reader)},
		{ID: gen1.ProgressRainbowBadge, Complete: yellowHasBadge(reader, badgeRainbow)},
		{ID: gen1.ProgressSilphScopeAcquired, Complete: yellowHasItem(reader, itemSilphScope)},
		{ID: gen1.ProgressPokeFluteAcquired, Complete: yellowHasItem(reader, itemPokeFlute)},
		{ID: gen1.ProgressFuchsiaProgressionComplete, Complete: yellowHasBadge(reader, badgeSoul) && hm03 && hm04},
		{ID: gen1.ProgressSaffronGateOpen, Complete: reader.Peek8(sym.StatusFlags1)&saffronGuardsDrinkMask != 0},
		{ID: gen1.ProgressCardKeyOwned, Complete: yellowHasItem(reader, itemCardKey)},
		{ID: gen1.ProgressSilphCoCleared, Complete: silphCleared && silphJJ},
		{ID: gen1.ProgressSilphRescueComplete, Complete: silphCleared && silphJJ && masterBall},
		{ID: gen1.ProgressMansionSwitchOn, Complete: yellowHasEvent(reader, eventMansionSwitchOn)},
		{ID: gen1.ProgressSecretKeyOwned, Complete: yellowHasItem(reader, itemSecretKey)},
		{ID: gen1.ProgressViridianGymOpen, Complete: yellowHasEvent(reader, eventViridianGymOpen)},
		{ID: gen1.ProgressRoute22RivalResolved, Complete: yellowHasEvent(reader, eventBeatRoute22Rival2)},
		{ID: gen1.ProgressRoute23BadgeChecks, Complete: badgeChecksComplete, Value: badgeChecks},
		{ID: gen1.ProgressVictoryRoadCleared, Complete: victoryRoadCleared},
		{ID: gen1.ProgressLeagueChallengeStarted, Complete: leagueStarted},
		{ID: gen1.ProgressLeagueChampionDefeated, Complete: champion},
		{ID: gen1.ProgressMainStoryComplete, Complete: mainComplete},
		{ID: gen1.ProgressVolcanoBadge, Complete: yellowHasBadge(reader, badgeVolcano)},
		{ID: gen1.ProgressEarthBadge, Complete: yellowHasBadge(reader, badgeEarth)},
		{ID: gen1.ProgressIndigoPlateauReady, Complete: indigoReady},

		{ID: ProgressYellowStarterReceived, Complete: yellowHasEvent(reader, eventGotStarter)},
		{ID: ProgressYellowLabRivalResolved, Complete: labRivalResolved},
		{ID: ProgressYellowRivalJolteonPath, Complete: labRivalResolved && rival == rivalStarterJolteon},
		{ID: ProgressYellowRivalFlareonPath, Complete: labRivalResolved && rival == rivalStarterFlareon},
		{ID: ProgressYellowRivalVaporeonPath, Complete: labRivalResolved && rival == rivalStarterVaporeon},
		{ID: ProgressYellowPikachuHappiness, Value: happiness},
		{ID: ProgressYellowPikachuStarterPresent, Complete: pikaFlags&(1<<pikachuStarterBit) != 0},
		{ID: ProgressYellowPikachuFollowing, Complete: pikaFlags&(1<<pikachuFollowingBit) != 0},
		{ID: ProgressYellowPikachuSurfing, Complete: pikaFlags&(1<<pikachuSurfingBit) != 0},
		{ID: ProgressYellowMtMoonJessieJamesDefeated, Complete: mtMoonJJ},
		{ID: ProgressYellowMtMoonExitResolved, Complete: mtMoonExitResolved},
		{ID: ProgressYellowRocketJessieJamesDefeated, Complete: rocketJJ},
		{ID: ProgressYellowTowerJessieJamesDefeated, Complete: towerJJ},
		{ID: ProgressYellowSilphJessieJamesDefeated, Complete: silphJJ},
		{ID: ProgressYellowBulbasaurGiftAvailable, Complete: !gotBulbasaur && happiness >= 147 && partyRoom},
		{ID: ProgressYellowBulbasaurGiftReceived, Complete: gotBulbasaur},
		{ID: ProgressYellowCharmanderGiftAvailable, Complete: !gotCharmander && partyRoom},
		{ID: ProgressYellowCharmanderGiftReceived, Complete: gotCharmander},
		{ID: ProgressYellowSquirtleGiftAvailable, Complete: !gotSquirtle && yellowHasBadge(reader, badgeThunder) && partyRoom},
		{ID: ProgressYellowSquirtleGiftReceived, Complete: gotSquirtle},
	}
}
