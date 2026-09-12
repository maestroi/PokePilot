// Package profile implements the Pokémon Red adapter for the generic game
// profile contract. Red-specific RAM addresses, event encodings, map ids and
// ROM identity stay on this side of the boundary.
package profile

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	GameID   game.GameID     = "pokemon-red"
	Revision game.RevisionID = "en-us-rev0"
)

// Progress IDs are Red profile vocabulary. The agent currently aliases the
// same strings while its progression executor completes the adapter migration.
const (
	ProgressMtMoonFossilAcquired       game.ProgressID = "mt_moon_fossil_acquired"
	ProgressPokedexAcquired            game.ProgressID = "pokedex_acquired"
	ProgressSSTicketAcquired           game.ProgressID = "ss_ticket_acquired"
	ProgressHM01Acquired               game.ProgressID = "hm01_acquired"
	ProgressThunderBadge               game.ProgressID = "thunder_badge"
	ProgressRainbowBadge               game.ProgressID = "rainbow_badge"
	ProgressSilphScopeAcquired         game.ProgressID = "silph_scope_acquired"
	ProgressPokeFluteAcquired          game.ProgressID = "poke_flute_acquired"
	ProgressFuchsiaProgressionComplete game.ProgressID = "fuchsia_progression_complete"
	ProgressSaffronGateOpen            game.ProgressID = "saffron_gate_open"
	ProgressCardKeyOwned               game.ProgressID = "card_key_owned"
	ProgressSilphCoCleared             game.ProgressID = "silph_co_cleared"
	ProgressSilphRescueComplete        game.ProgressID = "silph_rescue_complete"
	ProgressMansionSwitchOn            game.ProgressID = "mansion_switch_on"
	ProgressSecretKeyOwned             game.ProgressID = "secret_key_owned"
	ProgressViridianGymOpen            game.ProgressID = "viridian_gym_open"
	ProgressRoute22RivalResolved       game.ProgressID = "route_22_rival_resolved"
	ProgressRoute23BadgeChecks         game.ProgressID = "route_23_badge_checks"
	ProgressLeagueChallengeStarted     game.ProgressID = "league_challenge_started"
	ProgressLeagueChampionDefeated     game.ProgressID = "league_champion_defeated"
	ProgressMainStoryComplete          game.ProgressID = "main_story_complete"
	ProgressVolcanoBadge               game.ProgressID = "volcano_badge"
	ProgressEarthBadge                 game.ProgressID = "earth_badge"
	ProgressIndigoPlateauReady         game.ProgressID = "indigo_plateau_ready"
)

const (
	indigoPlateauMap      uint8 = 0x09
	indigoPlateauLobbyMap uint8 = 0xAE
)

type Profile struct{}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID             { return GameID }
func (*Profile) Revision() game.RevisionID    { return Revision }
func (*Profile) Detect(info game.ROMInfo) bool { return info.SHA1 == sym.ROMSHA1 }

func (*Profile) Symbols() game.SymbolTable {
	return game.SymbolTable{
		"player.map":       {Name: "player.map", Address: sym.CurMap, Width: 1},
		"player.x":         {Name: "player.x", Address: sym.XCoord, Width: 1},
		"player.y":         {Name: "player.y", Address: sym.YCoord, Width: 1},
		"player.direction": {Name: "player.direction", Address: sym.SpritePlayerFacing, Width: 1},
		"party.count":      {Name: "party.count", Address: sym.PartyCount, Width: 1},
		"party.members":    {Name: "party.members", Address: sym.PartyMon1, Width: int(sym.PartyMonSize) * 6},
		"battle.mode":      {Name: "battle.mode", Address: sym.IsInBattle, Width: 1},
		"badges":           {Name: "badges", Address: sym.ObtainedBadges, Width: 1},
		"bag":              {Name: "bag", Address: sym.BagItems},
		"money":            {Name: "money", Address: sym.PlayerMoney, Width: 3},
		"respawn.map":      {Name: "respawn.map", Address: sym.LastBlackoutMap, Width: 1},
		"story.events":     {Name: "story.events", Address: sym.EventFlags},
	}
}

func (*Profile) Features() game.ProfileFeatures {
	return game.ProfileFeatures{
		game.FeatureMapParsing:      true,
		game.FeatureInventory:       true,
		game.FeatureStoryProgress:   true,
		game.FeatureBattles:         true,
		game.FeatureFieldMoves:      true,
		game.FeatureTrainerFlags:    true,
		game.FeatureSemanticSpecies: true,
	}
}

func (*Profile) ROMParser() game.ROMParser { return parser{} }

type parser struct{}

func (parser) MapName(rawMapID uint16) (string, bool) {
	if rawMapID > 0xff {
		return "", false
	}
	name := state.MapName(uint8(rawMapID))
	return name, name != ""
}

func (parser) Species(rawSpecies uint16) (game.SpeciesID, bool) {
	if rawSpecies > 0xff {
		return "", false
	}
	return reddata.Species(uint8(rawSpecies))
}

var observedEvents = []state.Event{
	state.EventFollowedOakIntoLab,
	state.EventOakAskedToChooseMon,
	state.EventGotStarter,
	state.EventBattledRivalInOaksLab,
	state.EventGotPokeballsFromOak,
	state.EventGotPokedex,
	state.EventOakAppearedInPallet,
	state.EventBeatChampionRival,
}

func (*Profile) DecodeObservation(reader game.MemoryReader, _ []byte) (game.ProfileObservation, error) {
	if reader == nil {
		return game.ProfileObservation{}, fmt.Errorf("red profile: nil memory reader")
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	gs := state.Decode(&mem)
	mapName := state.MapName(gs.Player.MapID)
	obs := game.ProfileObservation{
		NativeMapID:   uint16(gs.Player.MapID),
		Location:      semanticLocation(mapName),
		MapName:       mapName,
		X:             gs.Player.X,
		Y:             gs.Player.Y,
		Facing:        gs.Player.Facing.String(),
		Controllable:  state.Controllable(&mem),
		InBattle:      gs.Battle != nil,
		Party:         make([]game.ProfilePartyMon, len(gs.Party.Mons)),
		Badges:        []string{},
		Money:         gs.Inventory.Money,
		RespawnPlace:  semanticLocation(state.MapName(mem.U8(sym.LastBlackoutMap))),
		Events:        []string{},
		BlackedOut:    mem.U8(sym.StatusFlags4)&(1<<5) != 0,
	}
	for i, mon := range gs.Party.Mons {
		species, ok := reddata.Species(mon.Species)
		if !ok {
			species = game.SpeciesID("unknown")
		}
		experience, _ := state.PartyExperience(&mem, i)
		obs.Party[i] = game.ProfilePartyMon{
			Species:    species,
			Level:      mon.Level,
			Experience: experience,
			HP:         mon.HP,
			MaxHP:      mon.MaxHP,
			Status:     mon.StatusName(),
		}
	}
	for badge := state.BadgeBoulder; badge <= state.BadgeEarth; badge++ {
		if gs.Progress.Has(badge) {
			obs.Badges = append(obs.Badges, badge.String())
		}
	}
	for _, event := range observedEvents {
		if state.HasEvent(&mem, event) {
			obs.Events = append(obs.Events, event.String())
		}
	}
	obs.Story = storyState(&mem, gs.Inventory, state.DecodeStoryFacts(&mem, gs.Inventory))
	return obs, nil
}

func semanticLocation(mapName string) game.PlaceID {
	return game.CanonicalID(strings.ReplaceAll(mapName, "_", " "))
}

func storyState(mem *state.Mem, _ state.InventoryState, facts state.StoryFacts) game.ProgressState {
	badges := state.DecodeProgress(mem)
	mapID := mem.U8(sym.CurMap)
	indigoReady := mapID == indigoPlateauMap || mapID == indigoPlateauLobbyMap || facts.LeagueChallengeStarted || facts.LeagueChampionDefeated || facts.MainStoryComplete
	return game.ProgressState{
		{ID: ProgressMtMoonFossilAcquired, Complete: facts.MtMoonFossilAcquired},
		{ID: ProgressPokedexAcquired, Complete: facts.PokedexAcquired},
		{ID: ProgressSSTicketAcquired, Complete: facts.SSTicketAcquired},
		{ID: ProgressHM01Acquired, Complete: facts.HM01Acquired},
		{ID: ProgressSilphScopeAcquired, Complete: facts.SilphScopeAcquired},
		{ID: ProgressPokeFluteAcquired, Complete: facts.PokeFluteAcquired},
		{ID: ProgressFuchsiaProgressionComplete, Complete: facts.FuchsiaProgressionComplete},
		{ID: ProgressSaffronGateOpen, Complete: facts.SaffronGateOpen},
		{ID: ProgressCardKeyOwned, Complete: facts.CardKeyOwned},
		{ID: ProgressSilphCoCleared, Complete: facts.SilphCoCleared},
		{ID: ProgressSilphRescueComplete, Complete: facts.SilphRescueComplete},
		{ID: ProgressMansionSwitchOn, Complete: facts.MansionSwitchOn},
		{ID: ProgressSecretKeyOwned, Complete: facts.SecretKeyOwned},
		{ID: ProgressViridianGymOpen, Complete: facts.ViridianGymOpen},
		{ID: ProgressRoute22RivalResolved, Complete: facts.Route22RivalResolved},
		{ID: ProgressRoute23BadgeChecks, Complete: facts.Route23BadgeChecksComplete, Value: facts.Route23BadgeChecksPassed},
		{ID: ProgressLeagueChallengeStarted, Complete: facts.LeagueChallengeStarted},
		{ID: ProgressLeagueChampionDefeated, Complete: facts.LeagueChampionDefeated},
		{ID: ProgressMainStoryComplete, Complete: facts.MainStoryComplete},
		{ID: ProgressThunderBadge, Complete: badges.Has(state.BadgeThunder)},
		{ID: ProgressRainbowBadge, Complete: badges.Has(state.BadgeRainbow)},
		{ID: ProgressVolcanoBadge, Complete: badges.Has(state.BadgeVolcano)},
		{ID: ProgressEarthBadge, Complete: badges.Has(state.BadgeEarth)},
		{ID: ProgressIndigoPlateauReady, Complete: indigoReady},
	}
}
