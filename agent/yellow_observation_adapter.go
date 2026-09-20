package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/gen1rom"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

// yellowSemanticObservationAdapter keeps Yellow separate from the Red/Blue
// gen1Games layout group. The Yellow profile now owns map/player/party and
// story projection; later phases can enrich inventory/battle/Dex execution
// without leaking native event ids into the generic observation contract.
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
		PokedexOwned:      append([]SpeciesID(nil), base.PokedexOwned...),
		PokedexSeen:       append([]SpeciesID(nil), base.PokedexSeen...),
		WildGrass:         []WildSpecies{},
		Requirements:      []Requirement{},
		RouteBlockages:    []RouteBlockage{},
		Unroutable:        []string{},
	}
	for _, item := range base.Bag {
		obs.Bag = append(obs.Bag, Item{Name: item.Name, Quantity: item.Quantity})
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

	cat, err := buildYellowDexCatalog(romData, obs.PokedexOwned, obs.PokedexSeen)
	if err != nil {
		return Observation{}, fmt.Errorf("Yellow Dex catalog: %w", err)
	}
	obs.Dex = cat

	wild, err := yellowrom.WildEncounters(romData)
	if err != nil {
		return Observation{}, fmt.Errorf("Yellow wild encounters: %w", err)
	}
	type levels struct {
		min, max uint8
		slots    int
	}
	local := map[SpeciesID]levels{}
	for _, enc := range wild {
		if enc.MapID != obs.Map || enc.Habitat != gen1rom.HabitatGrass {
			continue
		}
		id, ok := gen1.Species(enc.Species)
		if !ok {
			continue
		}
		key := SpeciesID(id)
		cur, exists := local[key]
		if !exists || enc.Level < cur.min {
			cur.min = enc.Level
		}
		if !exists || enc.Level > cur.max {
			cur.max = enc.Level
		}
		cur.slots++
		local[key] = cur
	}
	for id, level := range local {
		obs.WildGrass = append(obs.WildGrass, WildSpecies{
			Name: string(id), MinLevel: level.min, MaxLevel: level.max, Slots: level.slots,
		})
	}
	obs.HasGrass = len(obs.WildGrass) > 0

	catalog, err := yellowObjectiveCatalog(romData, obs)
	if err != nil {
		return Observation{}, fmt.Errorf("Yellow objective catalog: %w", err)
	}
	obs.Catalog = catalog
	return obs, nil
}
