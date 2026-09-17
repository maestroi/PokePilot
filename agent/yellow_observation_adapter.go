package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// yellowSemanticObservationAdapter owns the Yellow-facing observation. It is
// deliberately a subset of redSemanticObservationAdapter: the portable
// semantic state (location, party, badges, money, bag, story progress) comes
// from the profile's own decode of Yellow's RAM, and that is all the generic
// runtime needs to reason about the game.
//
// The Red adapter additionally enriches the observation with ROM-derived
// facts that require a game-specific reader of that game's ROM tables: move
// lookups, mart inventories, wild-grass encounter tables, field-move
// capability/preparability, route blockages and the training estimator. Those
// readers read Red's table addresses, which are not Yellow's, so they are not
// reused here. They arrive with the Yellow skill layer (docs/POKEYELLOW.md),
// and Observation carries them as empty slices meanwhile, which every generic
// consumer already tolerates.
type yellowSemanticObservationAdapter struct{}

func (yellowSemanticObservationAdapter) GameID() game.GameID { return yellowprofile.GameID }

func (a yellowSemanticObservationAdapter) Observe(m *emu.Emu, romData []byte, profile game.GameProfile) (Observation, error) {
	base, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return Observation{}, fmt.Errorf("base observation: %w", err)
	}
	if base.NativeMapID > 0xff {
		return Observation{}, fmt.Errorf("native map id 0x%x does not fit Pokémon Yellow map identity", base.NativeMapID)
	}

	obs := Observation{
		GameID:            yellowprofile.GameID,
		Map:               uint8(base.NativeMapID),
		Location:          base.Location,
		MapName:           base.MapName,
		X:                 base.X,
		Y:                 base.Y,
		Facing:            base.Facing,
		Controllable:      base.Controllable,
		InBattle:          base.InBattle,
		PartyCount:        len(base.Party),
		Money:             base.Money,
		RespawnPlace:      base.RespawnPlace,
		Party:             make([]PartyMon, len(base.Party)),
		Badges:            append([]string(nil), base.Badges...),
		Events:            append([]string(nil), base.Events...),
		Story:             append(ProgressState(nil), base.Story...),
		BlackedOut:        base.BlackedOut,
		LeadMoves:         []Move{},
		LeadPP:            []uint8{},
		Bag:               []Item{},
		FieldCapabilities: []FieldCapability{},
		RecentDialogue:    []string{},
		History:           []RoundRecord{},
		Failures:          []Failure{},
		Requirements:      []Requirement{},
		PokedexOwned:      []SpeciesID{},
		PokedexSeen:       []SpeciesID{},
		WildGrass:         []WildSpecies{},
	}
	for i, mon := range base.Party {
		obs.Party[i] = PartyMon{
			Species:    SpeciesID(mon.Species),
			Level:      mon.Level,
			Experience: mon.Experience,
			HP:         mon.HP,
			MaxHP:      mon.MaxHP,
			Status:     mon.Status,
		}
	}
	return obs, nil
}

func init() {
	registerSemanticObservationAdapter(yellowSemanticObservationAdapter{})
}
