package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
)

type gsSemanticObservationAdapter struct {
	id game.GameID
}

func (a gsSemanticObservationAdapter) GameID() game.GameID { return a.id }

func init() {
	registerSemanticObservationAdapter(gsSemanticObservationAdapter{id: gsprofile.GoldGameID})
	registerSemanticObservationAdapter(gsSemanticObservationAdapter{id: gsprofile.SilverGameID})
}

// Observe projects the Phase-2 Gold/Silver profile state into the planner
// observation without importing Gen-II RAM ids into generic runtime code.
// Rich routing/battle/objective catalogs remain later adapter phases.
func (gsSemanticObservationAdapter) Observe(m *emu.Emu, romData []byte, profile game.GameProfile) (Observation, error) {
	base, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return Observation{}, fmt.Errorf("base observation: %w", err)
	}
	obs := Observation{
		Location:     base.Location,
		MapName:      base.MapName,
		X:            base.X,
		Y:            base.Y,
		Facing:       base.Facing,
		Controllable: base.Controllable,
		InBattle:     base.InBattle,
		TimeOfDay:    string(base.TimeOfDay),
		Region:       string(base.Region),
		PartyCount:   len(base.Party),
		Party:        make([]PartyMon, len(base.Party)),
		Badges:       append([]string(nil), base.Badges...),
		Money:        base.Money,
		RespawnPlace: base.RespawnPlace,
		Events:       append([]string(nil), base.Events...),
		Story:        append(ProgressState(nil), base.Story...),
		BlackedOut:   base.BlackedOut,
		Bag:          make([]Item, 0, len(base.Bag)),
		PokedexOwned: append([]SpeciesID(nil), base.PokedexOwned...),
		PokedexSeen:  append([]SpeciesID(nil), base.PokedexSeen...),
		Dex:          base.Dex,
	}
	// Observation.Map is the legacy one-byte Gen-I migration handle. A GS map
	// is a (group, number) pair and therefore must not be truncated into it.
	for i, mon := range base.Party {
		obs.Party[i] = PartyMon{
			Species:    SpeciesID(mon.Species),
			HeldItem:   ItemID(mon.HeldItem),
			IsEgg:      mon.IsEgg,
			Level:      mon.Level,
			Experience: mon.Experience,
			HP:         mon.HP,
			MaxHP:      mon.MaxHP,
			Status:     mon.Status,
			Gender:     string(mon.Gender),
			Shiny:      mon.Shiny,
			Pokerus:    string(mon.Pokerus),
			Friendship: mon.Friendship,
		}
	}
	for _, item := range base.Bag {
		if item.Quantity <= 0 {
			continue
		}
		name := item.Name
		if name == "" {
			name = string(item.ID)
		}
		obs.Bag = append(obs.Bag, Item{Name: name, Quantity: item.Quantity})
	}
	if field, ok := profile.(game.FieldMoveDecoder); ok {
		for _, id := range game.ProgressionFieldMoves() {
			capability, supported, err := field.DecodeFieldMoveCapability(m, romData, id)
			if err != nil {
				return Observation{}, fmt.Errorf("field capability %s: %w", id, err)
			}
			if !supported {
				continue
			}
			obs.FieldCapabilities = append(obs.FieldCapabilities, FieldCapability{
				Name:       CapabilityID(id),
				Badge:      capability.BadgeRequired,
				BadgeOwned: capability.BadgeOwned,
				HMOwned:    capability.MachineOwned,
				Learned:    capability.Learned,
				PartySlot:  capability.PartySlot,
				Usable:     capability.Usable,
				Preparable: capability.Preparable,
			})
		}
	}
	// A missing wild table leaves the clock empty. Red-shaped or not-yet-located
	// images must not fail the rest of the observation, and they must not invent
	// a wait. Gold and Silver fill the leg from the cartridge's own windows.
	if enc, err := gsrom.EncountersOnMap(romData, base.NativeMapID); err == nil {
		obs.ClockEncounters = projectClockEncounters(base.TimeOfDay, gsrom.ClockSlots(enc))
	}
	return obs, nil
}
