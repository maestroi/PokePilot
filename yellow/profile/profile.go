// Package profile implements the Pokémon Yellow profile baseline.
//
// Phase 0 intentionally exposes only exact ROM identity plus the small set of
// RAM semantics needed to prove profile selection and basic player observation.
// Yellow is not registered as sharing Red/Blue's gameplay adapter because its
// RAM layout and story/mechanics differ. Later phases enable capabilities only
// after their Yellow implementations exist.
package profile

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	GameID   game.GameID     = "pokemon-yellow"
	Revision game.RevisionID = "en-us-rev0"
)

type Profile struct{}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID               { return GameID }
func (*Profile) Revision() game.RevisionID     { return Revision }
func (*Profile) Detect(info game.ROMInfo) bool { return info.SHA1 == sym.ROMSHA1 }

func (*Profile) Symbols() game.SymbolTable {
	return game.SymbolTable{
		"player.map":               {Name: "player.map", Address: sym.CurMap, Width: 1},
		"player.x":                 {Name: "player.x", Address: sym.XCoord, Width: 1},
		"player.y":                 {Name: "player.y", Address: sym.YCoord, Width: 1},
		"player.direction":         {Name: "player.direction", Address: sym.SpritePlayerFacing, Width: 1},
		"party.count":              {Name: "party.count", Address: sym.PartyCount, Width: 1},
		"party.members":            {Name: "party.members", Address: sym.PartyMon1, Width: int(sym.PartyMonSize) * 6},
		"battle.mode":              {Name: "battle.mode", Address: sym.IsInBattle, Width: 1},
		"badges":                   {Name: "badges", Address: sym.ObtainedBadges, Width: 1},
		"bag":                      {Name: "bag", Address: sym.BagItems},
		"money":                    {Name: "money", Address: sym.PlayerMoney, Width: 3},
		"respawn.map":              {Name: "respawn.map", Address: sym.LastBlackoutMap, Width: 1},
		"story.events":             {Name: "story.events", Address: sym.EventFlags},
		"yellow.rival.starter":     {Name: "yellow.rival.starter", Address: sym.RivalStarter, Width: 1},
		"yellow.pikachu.happiness": {Name: "yellow.pikachu.happiness", Address: sym.PikachuHappiness, Width: 1},
	}
}

func (*Profile) Features() game.ProfileFeatures {
	return game.ProfileFeatures{
		game.FeatureMapParsing:      true,
		game.FeatureStoryProgress:   true,
		game.FeatureSemanticSpecies: true,
	}
}

func (*Profile) ROMParser() game.ROMParser { return parser{} }

type parser struct{}

func (parser) MapName(rawMapID uint16) (string, bool) {
	if rawMapID > 0xff {
		return "", false
	}
	name := yellowrom.MapName(uint8(rawMapID))
	return name, name != ""
}

func (parser) Species(rawSpecies uint16) (game.SpeciesID, bool) {
	if rawSpecies > 0xff {
		return "", false
	}
	return gen1.Species(uint8(rawSpecies))
}

func (*Profile) DecodeObservation(reader game.MemoryReader, romData []byte) (game.ProfileObservation, error) {
	if reader == nil {
		return game.ProfileObservation{}, fmt.Errorf("yellow profile: nil memory reader")
	}
	mapID := reader.Peek8(sym.CurMap)
	mapName, _ := (parser{}).MapName(uint16(mapID))
	pokedexOwned, pokedexSeen := yellowPokedex(reader, romData)
	location := game.PlaceID("")
	if mapName != "" {
		location = game.PlaceID(game.CanonicalID(strings.ReplaceAll(mapName, "_", " ")))
	}
	return game.ProfileObservation{
		NativeMapID: uint16(mapID),
		Location:    location,
		MapName:     mapName,
		X:           reader.Peek8(sym.XCoord),
		Y:           reader.Peek8(sym.YCoord),
		Facing:      decodeFacing(reader.Peek8(sym.SpritePlayerFacing)),
		// Phase 1 exposes the shared control-state semantic while leaving party,
		// inventory and story decoding for later Yellow phases.
		Controllable: yellowControllable(reader),
		InBattle:     reader.Peek8(sym.IsInBattle) != 0,
		Party:        gen1.DecodeParty(reader, yellowRAMLayout),
		Badges:       gen1.DecodeBadges(reader, yellowRAMLayout),
		Money:        gen1.DecodeMoney(reader, yellowRAMLayout),
		RespawnPlace: yellowLocation(reader.Peek8(sym.LastBlackoutMap)),
		PokedexOwned: pokedexOwned,
		PokedexSeen:  pokedexSeen,
		Events:       yellowEventNames(reader),
		Story:        projectYellowStory(reader, mapID),
	}, nil
}

func yellowLocation(mapID uint8) game.PlaceID {
	name := yellowrom.MapName(mapID)
	if name == "" {
		return ""
	}
	return game.PlaceID(game.CanonicalID(strings.ReplaceAll(name, "_", " ")))
}

func decodeFacing(v byte) string {
	switch v & 0x0c {
	case 0x00:
		return "down"
	case 0x04:
		return "up"
	case 0x08:
		return "left"
	case 0x0c:
		return "right"
	default:
		return ""
	}
}
