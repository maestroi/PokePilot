package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

type redSemanticObservationAdapter struct {
	id game.GameID
}

func (a redSemanticObservationAdapter) GameID() game.GameID { return a.id }

func init() {
	for _, id := range gen1Games {
		registerSemanticObservationAdapter(redSemanticObservationAdapter{id: id})
	}
}

var redMoveTypeNames = map[uint8]string{
	0x00: "normal",
	0x01: "fighting",
	0x02: "flying",
	0x03: "poison",
	0x04: "ground",
	0x05: "rock",
	0x06: "flying",
	0x07: "bug",
	0x08: "ghost",
	0x14: "fire",
	0x15: "water",
	0x16: "grass",
	0x17: "electric",
	0x18: "psychic",
	0x19: "ice",
	0x1a: "dragon",
}

// Observe owns every Pokémon Red-specific enrichment required by the generic
// runtime. Generic ObserveChecked never decodes Red RAM, ROM tables, map object
// roles, encounter tables, marts, field moves, or route topology itself.
func (redSemanticObservationAdapter) Observe(m *emu.Emu, romData []byte, profile game.GameProfile) (Observation, error) {
	base, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return Observation{}, fmt.Errorf("base observation: %w", err)
	}
	if base.NativeMapID > 0xff {
		return Observation{}, fmt.Errorf("native map id 0x%x does not fit Pokémon Red map identity", base.NativeMapID)
	}

	var mem state.Mem
	gs := state.Read(m, &mem)
	obs := Observation{
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
		RepelSteps:        int(mem.U8(sym.RepelRemainingSteps)),
		Bag:               []Item{},
		FieldCapabilities: []FieldCapability{},
		RecentDialogue:    []string{},
		History:           []RoundRecord{},
		Failures:          []Failure{},
		Requirements:      []Requirement{},
		PokedexOwned:      []SpeciesID{},
		PokedexSeen:       []SpeciesID{},
	}
	obs.PokedexOwned, obs.PokedexSeen = ProjectPokedex(romData, gs.Pokedex)
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

	if len(gs.Party.Mons) > 0 {
		lead := gs.Party.Mons[0]
		hasDamagingMove := false
		for _, id := range lead.Moves {
			if id == 0 {
				continue
			}
			mv, err := rom.LookupMove(romData, id)
			if err == nil && observedMoveDealsDamage(mv) {
				hasDamagingMove = true
				break
			}
		}
		for slot, id := range lead.Moves {
			if id == 0 {
				continue
			}
			mv, err := rom.LookupMove(romData, id)
			if err != nil {
				continue
			}
			obs.LeadMoves = append(obs.LeadMoves, Move{Power: mv.Power, Type: redMoveTypeNames[mv.Type]})
			pp := lead.PP[slot]
			if hasDamagingMove && !observedMoveDealsDamage(mv) {
				pp = 0
			}
			obs.LeadPP = append(obs.LeadPP, pp)
		}
	}

	for _, it := range gs.Inventory.Items {
		if it.ID == 0 || it.Quantity == 0 {
			continue
		}
		name, ok := ItemName(it.ID)
		if !ok {
			if machine, err := rom.LookupTMHM(romData, it.ID); err == nil {
				name = string(machineItemID(machine))
			} else {
				name = "unknown"
			}
		}
		obs.Bag = append(obs.Bag, Item{Name: name, Quantity: int(it.Quantity)})
	}

	for _, cap := range skill.FieldCapabilities(&mem) {
		obs.FieldCapabilities = append(obs.FieldCapabilities, FieldCapability{
			Name:       CapabilityID(semanticPlace(cap.Name)),
			Badge:      cap.Badge.String(),
			BadgeOwned: cap.BadgeOwned,
			HMOwned:    cap.HMOwned,
			Learned:    cap.Learned,
			PartySlot:  cap.PartySlot,
			Usable:     cap.Usable,
			Preparable: cap.Usable || skill.CanPrepareFieldMove(romData, &mem, cap.Move),
		})
	}

	if grass, err := skill.HasReachableGrassLive(m, romData); err == nil {
		obs.HasGrass = grass
	}
	routes := routeAvailabilityFor(m, romData)
	obs.Unroutable = routes.Unroutable
	obs.RouteBlockages = routes.Blockages
	if cat, err := BuildDexCatalog(romData, obs.PokedexOwned, obs.PokedexSeen); err == nil {
		obs.Dex = annotateDexRouteRequirements(cat, obs.RouteBlockages)
	}
	obs.WildGrass = []WildSpecies{}
	if wild, err := skill.WildGrass(romData, obs.Map); err == nil {
		for _, w := range wild {
			name, ok := SpeciesName(w.ID)
			if !ok {
				continue
			}
			obs.WildGrass = append(obs.WildGrass, WildSpecies{
				Name: name, MinLevel: w.MinLevel, MaxLevel: w.MaxLevel, Slots: w.Slots,
			})
		}
	}
	if len(gs.Party.Mons) > 0 && obs.HasGrass {
		target := int(gs.Party.Mons[0].Level) + trainStep
		if target <= 100 {
			if estimate, err := currentTrainingEstimate(&mem, romData, obs.Map, target, trainSessionBattleBudget); err == nil {
				obs.Training = &estimate
			}
		}
	}

	obs.MartStock = []string{}
	if items, err := rom.MartItems(romData, obs.Map); err == nil {
		for _, id := range items {
			if name, ok := ItemName(id); ok {
				obs.MartStock = append(obs.MartStock, name)
			}
		}
	}

	objects := MapObjects(romData, obs.Map)
	hidden := state.HiddenObjectIDs(&mem)
	objectGrid := mapObjectReachabilityGridLive(romData, obs.Map, &mem)
	stationary := stationaryHomeTiles(romData, obs.Map)
	obs.MapObjects = make([]MapObject, 0, len(objects))
	for i, object := range objects {
		if hidden[uint8(i+1)] {
			continue
		}
		if object.Kind == "trainer" {
			if status, err := skill.TrainerStatusAtLive(romData, &mem, obs.Map, object.X, object.Y); err == nil {
				object.Challengeable = status.Challengeable
				object.Defeated = status.Defeated
			}
			if objectGrid != nil && !personReachableOnGrid(objectGrid, obs.X, obs.Y, object.X, object.Y, stationary) {
				continue
			}
		}
		if object.Kind == "item" && objectGrid != nil && !reachableOnGrid(objectGrid, obs.X, obs.Y, object.X, object.Y, stationary) {
			continue
		}
		if object.Kind == "person" && objectGrid != nil && !personReachableOnGrid(objectGrid, obs.X, obs.Y, object.X, object.Y, stationary) {
			continue
		}
		obs.MapObjects = append(obs.MapObjects, object)
	}
	obs.Catalog = redObjectiveCatalog(obs)
	return obs, nil
}

func unroutablePlaces(m *emu.Emu, romData []byte) []string {
	return routeAvailabilityFor(m, romData).Unroutable
}

func mapObjectReachabilityGridLive(romData []byte, mapID uint8, mem *state.Mem) *world.Grid {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return nil
	}
	if mem != nil && mem.U8(sym.CurMap) == mapID {
		if g, err := skill.LiveMapGridFromMem(mem, romData, h); err == nil {
			return g
		}
	}
	// Observation stays fail-open to the stable ROM geometry when a snapshot
	// is incomplete. A valid live snapshot, however, must own current object
	// reachability so script-replaced doors cannot advertise impossible work.
	g, err := world.Build(romData, h)
	if err != nil {
		return nil
	}
	return g
}

func mapObjectReachabilityGrid(romData []byte, mapID uint8) *world.Grid {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return nil
	}
	g, err := world.Build(romData, h)
	if err != nil {
		return nil
	}
	return g
}

func adjacent(px, py, x, y uint8) bool {
	return (px == x && (py == y+1 || py+1 == y)) || (py == y && (px == x+1 || px+1 == x))
}

func stationaryHomeTiles(romData []byte, mapID uint8) map[[2]int]bool {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return nil
	}
	blocked := map[[2]int]bool{}
	for _, o := range h.Objects {
		if o.Movement == rom.MovementStay {
			blocked[[2]int{int(o.X), int(o.Y)}] = true
		}
	}
	return blocked
}

func reachableOnGrid(g *world.Grid, px, py, x, y uint8, blocked map[[2]int]bool) bool {
	if adjacent(px, py, x, y) {
		return true
	}
	_, _, err := world.FindPathAdjacent(g, int(px), int(py), int(x), int(y), blocked)
	return err == nil
}

func personReachableOnGrid(g *world.Grid, px, py, x, y uint8, blocked map[[2]int]bool) bool {
	if reachableOnGrid(g, px, py, x, y, blocked) {
		return true
	}
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		midX, midY := int(x)+s.DX, int(y)+s.DY
		farX, farY := int(x)+2*s.DX, int(y)+2*s.DY
		if g.Walkable(midX, midY) || !g.Walkable(farX, farY) || blocked[[2]int{farX, farY}] {
			continue
		}
		if farX == int(px) && farY == int(py) {
			return true
		}
		if _, err := world.FindPath(g, int(px), int(py), farX, farY, blocked); err == nil {
			return true
		}
	}
	return false
}

func reachableOnFoot(romData []byte, mapID, px, py, x, y uint8) bool {
	if adjacent(px, py, x, y) {
		return true
	}
	g := mapObjectReachabilityGrid(romData, mapID)
	if g == nil {
		return true
	}
	return reachableOnGrid(g, px, py, x, y, stationaryHomeTiles(romData, mapID))
}

func personReachable(romData []byte, mapID, px, py, x, y uint8) bool {
	g := mapObjectReachabilityGrid(romData, mapID)
	if g == nil {
		return true
	}
	return personReachableOnGrid(g, px, py, x, y, stationaryHomeTiles(romData, mapID))
}

func observedMoveDealsDamage(mv rom.Move) bool {
	return mv.Power > 0 || mv.Effect == rom.SpecialDamageEffect || mv.Effect == rom.SuperFangEffect || mv.Effect == rom.OHKOEffect
}

func MapObjects(romData []byte, mapID uint8) []MapObject {
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return []MapObject{}
	}
	out := make([]MapObject, 0, len(h.Objects))
	for _, o := range h.Objects {
		mo := MapObject{X: o.X, Y: o.Y}
		switch {
		case o.TextID&0x80 != 0:
			mo.Kind = "item"
			if name, ok := ItemName(o.ItemID); ok {
				mo.Item = name
			} else if machine, err := rom.LookupTMHM(romData, o.ItemID); err == nil {
				mo.Item = string(machineItemID(machine))
			} else {
				mo.Item = "unknown"
			}
		case o.TextID&0x40 != 0:
			mo.Kind = "trainer"
		default:
			mo.Kind = "person"
		}
		out = append(out, mo)
	}
	return out
}
