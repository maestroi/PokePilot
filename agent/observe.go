package agent

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Observation is the complete planner-facing view of one settled game state.
// Raw game encodings stay available to Red-owned runtime code where necessary,
// but they do not cross the planner JSON contract: Location, party species,
// respawn place and progression are semantic values.
type Observation struct {
	// Map is Red's internal map byte and remains runtime-only during the
	// incremental adapter migration. Location is the portable planner identity.
	Map      uint8 `json:"-"`
	Location PlaceID
	MapName  string // display name retained for prompt/backward readability
	X, Y     uint8
	Facing   string

	Controllable bool
	InBattle     bool
	PartyCount   int
	Party        []PartyMon
	Badges       []string
	Money        uint32
	RespawnPlace PlaceID
	Events       []string
	Story        ProgressState
	BlackedOut   bool

	LeadMoves         []Move
	LeadPP            []uint8
	Bag               []Item
	FieldCapabilities []FieldCapability
	RecentDialogue    []string
	History           []RoundRecord
	Failures          []Failure

	Round      int
	RoundsLeft int
	Intent     string
	IntentAge  int

	WildGrass  []WildSpecies
	HasGrass   bool
	Training   *TrainingEstimate `json:"training,omitempty"`
	MartStock  []string
	MapObjects []MapObject

	Requirements   []Requirement
	RouteBlockages []RouteBlockage
	Unroutable     []string `json:"-"`
}

// MapObject is one observable object on the current map. Item is a semantic
// item name when Kind is "item"; unknown Red item bytes are never exposed.
type MapObject struct {
	X, Y uint8
	Kind string
	Item string
}

type Move struct {
	Power uint8
	Type  string
}

type Item struct {
	Name     string
	Quantity int
}

type FieldCapability struct {
	Name       CapabilityID
	Badge      string
	BadgeOwned bool
	HMOwned    bool
	Learned    bool
	PartySlot  int
	Usable     bool
	Preparable bool
}

type RoundRecord struct {
	Objective string
	Outcome   string
}

var moveTypeNames = map[uint8]string{
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

type PartyMon struct {
	Species    SpeciesID
	Level      uint8
	Experience uint32
	HP         uint16
	MaxHP      uint16
	Status     string
}

type WildSpecies struct {
	Name     string
	MinLevel uint8
	MaxLevel uint8
	Slots    int
}

// Events remain a compact compatibility list while #137 migrates progression
// verbs. Their raw bit/index encoding remains private to red/state.
var knownEvents = []state.Event{
	state.EventFollowedOakIntoLab,
	state.EventOakAskedToChooseMon,
	state.EventGotStarter,
	state.EventBattledRivalInOaksLab,
	state.EventGotPokeballsFromOak,
	state.EventGotPokedex,
	state.EventOakAppearedInPallet,
	state.EventBeatChampionRival,
}

// Observe is Pokémon Red's projection into the portable planner contract.
func Observe(m *emu.Emu, romData []byte) Observation {
	var mem state.Mem
	gs := state.Read(m, &mem)
	mapName := state.MapName(gs.Player.MapID)

	obs := Observation{
		Map:               gs.Player.MapID,
		Location:          semanticLocation(mapName),
		MapName:           mapName,
		X:                 gs.Player.X,
		Y:                 gs.Player.Y,
		Facing:            gs.Player.Facing.String(),
		Controllable:      state.Controllable(&mem),
		InBattle:          gs.Battle != nil,
		PartyCount:        int(gs.Party.Count),
		Money:             gs.Inventory.Money,
		RespawnPlace:      semanticLocation(state.MapName(mem.U8(sym.LastBlackoutMap))),
		Party:             make([]PartyMon, len(gs.Party.Mons)),
		Badges:            []string{},
		Events:            []string{},
		Story:             redProgressState(state.DecodeStoryFacts(&mem, gs.Inventory)),
		LeadMoves:         []Move{},
		LeadPP:            []uint8{},
		Bag:               []Item{},
		FieldCapabilities: []FieldCapability{},
		RecentDialogue:    []string{},
		History:           []RoundRecord{},
		Failures:          []Failure{},
		Requirements:      []Requirement{},
	}
	for i, mon := range gs.Party.Mons {
		experience, _ := state.PartyExperience(&mem, i)
		obs.Party[i] = PartyMon{
			Species:    semanticSpeciesFromRed(mon.Species),
			Level:      mon.Level,
			Experience: experience,
			HP:         mon.HP,
			MaxHP:      mon.MaxHP,
			Status:     mon.StatusName(),
		}
	}

	obs.BlackedOut = mem.U8(sym.StatusFlags4)&(1<<5) != 0
	for b := state.BadgeBoulder; b <= state.BadgeEarth; b++ {
		if gs.Progress.Has(b) {
			obs.Badges = append(obs.Badges, b.String())
		}
	}
	for _, e := range knownEvents {
		if state.HasEvent(&mem, e) {
			obs.Events = append(obs.Events, e.String())
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
			obs.LeadMoves = append(obs.LeadMoves, Move{Power: mv.Power, Type: moveTypeNames[mv.Type]})
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

	if grass, err := skill.HasGrass(romData, obs.Map); err == nil {
		obs.HasGrass = grass
	}
	routes := routeAvailabilityFor(m, romData)
	obs.Unroutable = routes.Unroutable
	obs.RouteBlockages = routes.Blockages
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
	obs.MapObjects = make([]MapObject, 0, len(objects))
	for i, object := range objects {
		if hidden[uint8(i+1)] {
			continue
		}
		if object.Kind == "item" && !reachableOnFoot(romData, obs.Map, obs.X, obs.Y, object.X, object.Y) {
			continue
		}
		if object.Kind == "person" && !personReachable(romData, obs.Map, obs.X, obs.Y, object.X, object.Y) {
			continue
		}
		obs.MapObjects = append(obs.MapObjects, object)
	}
	return obs
}

func unroutablePlaces(m *emu.Emu, romData []byte) []string {
	return routeAvailabilityFor(m, romData).Unroutable
}

func reachableOnFoot(romData []byte, mapID, px, py, x, y uint8) bool {
	if (px == x && (py == y+1 || py+1 == y)) || (py == y && (px == x+1 || px+1 == x)) {
		return true
	}
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return true
	}
	g, err := world.Build(romData, h)
	if err != nil {
		return true
	}
	_, _, err = world.FindPathAdjacent(g, int(px), int(py), int(x), int(y), nil)
	return err == nil
}

func personReachable(romData []byte, mapID, px, py, x, y uint8) bool {
	if reachableOnFoot(romData, mapID, px, py, x, y) {
		return true
	}
	h, err := rom.ParseMap(romData, mapID)
	if err != nil {
		return true
	}
	g, err := world.Build(romData, h)
	if err != nil {
		return true
	}
	for _, s := range []world.Step{world.StepUp, world.StepDown, world.StepLeft, world.StepRight} {
		midX, midY := int(x)+s.DX, int(y)+s.DY
		farX, farY := int(x)+2*s.DX, int(y)+2*s.DY
		if g.Walkable(midX, midY) || !g.Walkable(farX, farY) {
			continue
		}
		if farX == int(px) && farY == int(py) {
			return true
		}
		if _, err := world.FindPath(g, int(px), int(py), farX, farY, nil); err == nil {
			return true
		}
	}
	return false
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
