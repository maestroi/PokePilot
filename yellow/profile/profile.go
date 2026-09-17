// Package profile implements the Pokémon Yellow adapter for the generic game
// profile contract. Yellow-specific RAM addresses, event encodings, map ids
// and ROM identity stay on this side of the boundary.
//
// Yellow shares Gen I's engine *shape* with Red — party, battle, inventory,
// badge and event-flag structures are byte-identical — but not its layout:
// most WRAM addressables sit one byte lower, the ROM map tables moved, and
// the starter is a scripted Pikachu rather than a choice. So Yellow owns its
// own engine surface (yellow/sym, yellow/state, yellow/rom) and does not
// delegate to red/profile the way Blue does.
package profile

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	reddata "github.com/maestroi/pokepilot/red/data"
	redstate "github.com/maestroi/pokepilot/red/state"
	yellowstate "github.com/maestroi/pokepilot/yellow/state"
	yellowsym "github.com/maestroi/pokepilot/yellow/sym"
)

const (
	GameID   game.GameID     = "pokemon-yellow"
	Revision game.RevisionID = "en-us-rev0"
)

// Profile is the Yellow GameProfile.
type Profile struct{}

func New() *Profile { return &Profile{} }

func (*Profile) ID() game.GameID               { return GameID }
func (*Profile) Revision() game.RevisionID     { return Revision }
func (*Profile) Detect(info game.ROMInfo) bool { return info.SHA1 == yellowsym.ROMSHA1 }
func (*Profile) ROMParser() game.ROMParser     { return parser{} }

func (*Profile) Symbols() game.SymbolTable {
	return game.SymbolTable{
		"player.map":       {Name: "player.map", Address: yellowsym.CurMap, Width: 1},
		"player.x":         {Name: "player.x", Address: yellowsym.XCoord, Width: 1},
		"player.y":         {Name: "player.y", Address: yellowsym.YCoord, Width: 1},
		"player.direction": {Name: "player.direction", Address: yellowsym.SpritePlayerFacing, Width: 1},
		"party.count":      {Name: "party.count", Address: yellowsym.PartyCount, Width: 1},
		"party.members":    {Name: "party.members", Address: yellowsym.PartyMon1, Width: int(yellowsym.PartyMonSize) * 6},
		"battle.mode":      {Name: "battle.mode", Address: yellowsym.IsInBattle, Width: 1},
		"badges":           {Name: "badges", Address: yellowsym.ObtainedBadges, Width: 1},
		"bag":              {Name: "bag", Address: yellowsym.BagItems},
		"money":            {Name: "money", Address: yellowsym.PlayerMoney, Width: 3},
		"respawn.map":      {Name: "respawn.map", Address: yellowsym.LastBlackoutMap, Width: 1},
		"story.events":     {Name: "story.events", Address: yellowsym.EventFlags},
	}
}

// Features declares only what Yellow actually has an implementation for. The
// baseline (position, party, badges, money) is complete; richer capabilities
// are enabled as the Yellow skill layer lands. Generic code must test a
// feature rather than branch on a game name.
func (*Profile) Features() game.ProfileFeatures {
	return game.ProfileFeatures{
		game.FeatureMapParsing:      true,
		game.FeatureInventory:       true,
		game.FeatureStoryProgress:   true,
		game.FeatureBattles:         true,
		game.FeatureSemanticSpecies: true,
	}
}

type parser struct{}

func (parser) MapName(rawMapID uint16) (string, bool) {
	if rawMapID > 0xff {
		return "", false
	}
	name := yellowstate.MapName(uint8(rawMapID))
	return name, name != ""
}

// Species maps a raw Gen I species index to a semantic id. The Gen I species
// table is shared between Red and Yellow, so this delegates to red/data.
func (parser) Species(rawSpecies uint16) (game.SpeciesID, bool) {
	if rawSpecies > 0xff {
		return "", false
	}
	return reddata.Species(uint8(rawSpecies))
}

// DecodeObservation turns a Yellow RAM snapshot into semantic state.
func (*Profile) DecodeObservation(reader game.MemoryReader, _ []byte) (game.ProfileObservation, error) {
	if reader == nil {
		return game.ProfileObservation{}, fmt.Errorf("yellow profile: nil memory reader")
	}
	var mem yellowstate.Mem
	reader.PeekInto(0, mem[:])
	gs := yellowstate.Decode(&mem)
	mapName := yellowstate.MapName(gs.Player.MapID)
	obs := game.ProfileObservation{
		NativeMapID:  uint16(gs.Player.MapID),
		Location:     semanticLocation(mapName),
		MapName:      mapName,
		X:            gs.Player.X,
		Y:            gs.Player.Y,
		Facing:       gs.Player.Facing.String(),
		Controllable: yellowstate.Controllable(&mem),
		InBattle:     gs.Battle != nil,
		Party:        make([]game.ProfilePartyMon, len(gs.Party.Mons)),
		Badges:       []string{},
		Money:        gs.Inventory.Money,
		RespawnPlace: semanticLocation(yellowstate.MapName(mem.U8(yellowsym.LastBlackoutMap))),
		Events:       []string{},
		BlackedOut:   mem.U8(yellowsym.StatusFlags4)&(1<<5) != 0,
	}
	for i, mon := range gs.Party.Mons {
		species, ok := reddata.Species(mon.Species)
		if !ok {
			species = game.SpeciesID("unknown")
		}
		obs.Party[i] = game.ProfilePartyMon{
			Species: species,
			Level:   mon.Level,
			HP:      mon.HP,
			MaxHP:   mon.MaxHP,
			Status:  mon.StatusName(),
		}
	}
	for badge := redstate.BadgeBoulder; badge <= redstate.BadgeEarth; badge++ {
		if gs.Progress.Has(badge) {
			obs.Badges = append(obs.Badges, badge.String())
		}
	}
	return obs, nil
}

func semanticLocation(mapName string) game.PlaceID {
	return game.CanonicalID(strings.ReplaceAll(mapName, "_", " "))
}
