package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// yellowSemanticObservationAdapter is intentionally minimal for phase 0.
// Unlike Blue, Yellow must not be registered in gen1Games: its RAM layout and
// game-specific progression differ from Red/Blue.
type yellowSemanticObservationAdapter struct{}

func (yellowSemanticObservationAdapter) GameID() game.GameID { return yellowprofile.GameID }

func init() {
	registerSemanticObservationAdapter(yellowSemanticObservationAdapter{})
}

func (yellowSemanticObservationAdapter) Observe(m *emu.Emu, romData []byte, profile game.GameProfile) (Observation, error) {
	base, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return Observation{}, fmt.Errorf("base observation: %w", err)
	}
	if base.NativeMapID > 0xff {
		return Observation{}, fmt.Errorf("native map id 0x%x does not fit Pokémon Yellow map identity", base.NativeMapID)
	}

	obs := Observation{
		GameID:            profile.ID(),
		Map:               uint8(base.NativeMapID),
		Location:          base.Location,
		MapName:           base.MapName,
		X:                 base.X,
		Y:                 base.Y,
		Facing:            base.Facing,
		Controllable:      base.Controllable,
		InBattle:          base.InBattle,
		PartyCount:        len(base.Party),
		Party:             make([]PartyMon, len(base.Party)),
		Badges:            append([]string(nil), base.Badges...),
		Money:             base.Money,
		RespawnPlace:      base.RespawnPlace,
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
		PokedexOwned:      []SpeciesID{},
		PokedexSeen:       []SpeciesID{},
		WildGrass:         []WildSpecies{},
		Requirements:      []Requirement{},
		RouteBlockages:    []RouteBlockage{},
		Unroutable:        []string{},
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
