package agent

import (
	"fmt"
	"sort"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Acquisition kinds are portable planner vocabulary. How Red implements
// each kind stays in the adapter (ROM tables and scripted facts).
const (
	AcquireWildGrass   = "wild_grass"
	AcquireWildWater   = "wild_water"
	AcquireFishing     = "fishing"
	AcquireLevelEvo    = "evolution_level"
	AcquireItemEvo     = "evolution_item"
	AcquireTradeEvo    = "evolution_trade"
	AcquireInGameTrade = "in_game_trade"
	AcquireGift        = "gift"
	AcquireStatic      = "static"
	AcquireFossil      = "fossil"
	AcquireStarter     = "starter"
)

const (
	UnavailableTradeEvolution = "trade_evolution"
	UnavailableEventOnly      = "event_only"
	UnavailableNoLocalSource  = "no_local_source"
	UnavailableForfeited      = "forfeited"
)

// DexSource is one legitimate way this save can obtain a species.
type DexSource struct {
	Kind           string    `json:"kind"`
	From           SpeciesID `json:"from,omitempty"`
	Item           ItemID    `json:"item,omitempty"`
	Level          uint8     `json:"level,omitempty"`
	Place          PlaceID   `json:"place,omitempty"`
	Requirement    string    `json:"requirement,omitempty"`
	ExclusiveGroup string    `json:"exclusive_group,omitempty"`
	Give           SpeciesID `json:"give,omitempty"`
}

// DexEntry is one National Dex species as the planner should see it.
type DexEntry struct {
	Species     SpeciesID   `json:"species"`
	Dex         uint8       `json:"dex"`
	Owned       bool        `json:"owned,omitempty"`
	Seen        bool        `json:"seen,omitempty"`
	Sources     []DexSource `json:"sources,omitempty"`
	Unavailable string      `json:"unavailable,omitempty"`
}

// DexCatalog is the deterministic Dex-mode world model: owned species,
// remaining local targets, and species this save cannot produce.
type DexCatalog struct {
	Owned       []DexEntry `json:"owned"`
	Targets     []DexEntry `json:"targets"`
	Unavailable []DexEntry `json:"unavailable"`
}

// ProjectPokedex turns Red Pokédex numbers into semantic species IDs via
// the ROM's PokedexOrder inverse. Dex numbers that do not resolve are
// omitted rather than reported as "unknown".
func ProjectPokedex(romData []byte, dex state.PokedexState) (owned, seen []SpeciesID) {
	return projectDexList(romData, dex.Owned), projectDexList(romData, dex.Seen)
}

func projectDexList(romData []byte, numbers []uint8) []SpeciesID {
	out := make([]SpeciesID, 0, len(numbers))
	for _, n := range numbers {
		id, ok := speciesFromDex(romData, n)
		if !ok {
			continue
		}
		out = append(out, id)
	}
	return out
}

func speciesFromDex(romData []byte, dex uint8) (SpeciesID, bool) {
	internal, err := rom.DexNumberInternalSpecies(romData, dex)
	if err != nil {
		return "", false
	}
	id := semanticSpeciesFromRed(internal)
	if id == "" || id == "unknown" {
		return "", false
	}
	return id, true
}

// BuildDexCatalog classifies every National Dex species the ROM maps.
// Completion uses Owned, never Seen or the current party.
func BuildDexCatalog(romData []byte, owned, seen []SpeciesID) (DexCatalog, error) {
	sources, err := collectDexSources(romData)
	if err != nil {
		return DexCatalog{}, err
	}
	entries, err := dexSpecies(romData)
	if err != nil {
		return DexCatalog{}, err
	}
	return assembleDexCatalog(entries, sources, owned, seen, redExclusiveChoices()), nil
}

func dexSpecies(romData []byte) ([]DexEntry, error) {
	out := make([]DexEntry, 0, sym.PokedexCount)
	for dex := uint8(1); dex <= uint8(sym.PokedexCount); dex++ {
		id, ok := speciesFromDex(romData, dex)
		if !ok {
			continue
		}
		out = append(out, DexEntry{Species: id, Dex: dex})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agent: dex catalog: ROM maps no Pokédex species")
	}
	return out, nil
}

func collectDexSources(romData []byte) (map[SpeciesID][]DexSource, error) {
	out := map[SpeciesID][]DexSource{}
	add := func(internal uint8, src DexSource) {
		id := semanticSpeciesFromRed(internal)
		if id == "" || id == "unknown" {
			return
		}
		out[id] = append(out[id], src)
	}

	wild, err := rom.WildEncounters(romData)
	if err != nil {
		return nil, err
	}
	for _, enc := range wild {
		kind := AcquireWildGrass
		req := ""
		if enc.Habitat == rom.HabitatWater {
			kind = AcquireWildWater
			req = "surf"
		}
		if safariRequirement(enc.MapID) {
			req = joinReq(req, "safari_zone")
		}
		add(enc.Species, DexSource{
			Kind:        kind,
			Place:       mapPlace(enc.MapID),
			Level:       enc.Level,
			Requirement: req,
		})
	}

	fish, err := rom.FishingEncounters(romData)
	if err != nil {
		return nil, err
	}
	for _, enc := range fish {
		src := DexSource{
			Kind:        AcquireFishing,
			Level:       enc.Level,
			Requirement: enc.Rod + "_rod",
		}
		if enc.MapID != 0 {
			src.Place = mapPlace(enc.MapID)
		}
		if safariRequirement(enc.MapID) {
			src.Requirement = joinReq(src.Requirement, "safari_zone")
		}
		add(enc.Species, src)
	}

	evos, err := rom.Evolutions(romData)
	if err != nil {
		return nil, err
	}
	for _, evo := range evos {
		src := DexSource{From: semanticSpeciesFromRed(evo.From), Level: evo.Level}
		switch evo.Method {
		case rom.EvoLevel:
			src.Kind = AcquireLevelEvo
		case rom.EvoItem:
			src.Kind = AcquireItemEvo
			src.Item = evolutionItem(evo.Item)
		case rom.EvoTrade:
			src.Kind = AcquireTradeEvo
		default:
			continue
		}
		add(evo.To, src)
	}

	trades, err := rom.NPCTrades(romData)
	if err != nil {
		return nil, err
	}
	for _, tr := range trades {
		add(tr.Get, DexSource{
			Kind: AcquireInGameTrade,
			Give: semanticSpeciesFromRed(tr.Give),
		})
	}

	for _, src := range redScriptedSources() {
		add(src.internal, src.source)
	}
	return out, nil
}

func assembleDexCatalog(species []DexEntry, sources map[SpeciesID][]DexSource, owned, seen []SpeciesID, exclusives []exclusiveChoice) DexCatalog {
	ownedSet := speciesSet(owned)
	seenSet := speciesSet(seen)
	forfeited := forfeitedSpecies(ownedSet, exclusives)

	direct := map[SpeciesID]bool{}
	for id, srcs := range sources {
		if forfeited[id] != "" {
			continue
		}
		if hasDirectSource(srcs) {
			direct[id] = true
		}
	}
	local := closeLocal(direct, ownedSet, sources, forfeited)

	cat := DexCatalog{
		Owned:       []DexEntry{},
		Targets:     []DexEntry{},
		Unavailable: []DexEntry{},
	}
	for _, entry := range species {
		entry.Owned = ownedSet[entry.Species]
		entry.Seen = seenSet[entry.Species]
		entry.Sources = append([]DexSource(nil), sources[entry.Species]...)
		sortDexSources(entry.Sources)
		switch {
		case entry.Owned:
			cat.Owned = append(cat.Owned, entry)
		case forfeited[entry.Species] != "":
			entry.Unavailable = UnavailableForfeited + ":" + forfeited[entry.Species]
			cat.Unavailable = append(cat.Unavailable, entry)
		case local[entry.Species]:
			cat.Targets = append(cat.Targets, entry)
		case redEventOnly()[entry.Species]:
			entry.Unavailable = UnavailableEventOnly
			cat.Unavailable = append(cat.Unavailable, entry)
		case onlyTradeEvo(entry.Sources):
			entry.Unavailable = UnavailableTradeEvolution
			cat.Unavailable = append(cat.Unavailable, entry)
		default:
			entry.Unavailable = UnavailableNoLocalSource
			cat.Unavailable = append(cat.Unavailable, entry)
		}
	}
	return cat
}

func hasDirectSource(srcs []DexSource) bool {
	for _, src := range srcs {
		switch src.Kind {
		case AcquireWildGrass, AcquireWildWater, AcquireFishing, AcquireInGameTrade,
			AcquireGift, AcquireStatic, AcquireFossil, AcquireStarter:
			return true
		}
	}
	return false
}

func closeLocal(direct, owned map[SpeciesID]bool, sources map[SpeciesID][]DexSource, forfeited map[SpeciesID]string) map[SpeciesID]bool {
	local := map[SpeciesID]bool{}
	for id := range direct {
		local[id] = true
	}
	for id := range owned {
		local[id] = true
	}
	changed := true
	for changed {
		changed = false
		for id, srcs := range sources {
			if local[id] || forfeited[id] != "" {
				continue
			}
			for _, src := range srcs {
				if src.Kind != AcquireLevelEvo && src.Kind != AcquireItemEvo {
					continue
				}
				if src.From != "" && local[src.From] {
					local[id] = true
					changed = true
					break
				}
			}
		}
	}
	return local
}

func onlyTradeEvo(srcs []DexSource) bool {
	if len(srcs) == 0 {
		return false
	}
	for _, src := range srcs {
		if src.Kind != AcquireTradeEvo {
			return false
		}
	}
	return true
}

func speciesSet(ids []SpeciesID) map[SpeciesID]bool {
	out := map[SpeciesID]bool{}
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func sortDexSources(srcs []DexSource) {
	sort.Slice(srcs, func(i, j int) bool {
		a, b := srcs[i], srcs[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Place != b.Place {
			return a.Place < b.Place
		}
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		return a.Level < b.Level
	})
}

func mapPlace(mapID uint8) PlaceID {
	name := state.MapName(mapID)
	if name == "" {
		return ""
	}
	return semanticLocation(name)
}

func safariRequirement(mapID uint8) bool {
	return mapID >= 0xD9 && mapID <= 0xDC
}

func joinReq(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "," + b
	}
}

func annotateDexRouteRequirements(cat DexCatalog, blockages []RouteBlockage) DexCatalog {
	missing := map[PlaceID][]CapabilityID{}
	for _, b := range blockages {
		if b.Destination == "" || len(b.Missing) == 0 {
			continue
		}
		missing[b.Destination] = append([]CapabilityID(nil), b.Missing...)
	}
	annotate := func(entries []DexEntry) {
		for i := range entries {
			for j, src := range entries[i].Sources {
				need, ok := missing[src.Place]
				if !ok {
					continue
				}
				req := src.Requirement
				for _, id := range need {
					req = joinReq(req, string(id))
				}
				entries[i].Sources[j].Requirement = req
			}
		}
	}
	annotate(cat.Owned)
	annotate(cat.Targets)
	annotate(cat.Unavailable)
	return cat
}

func pokedexOwnedSet(obs Observation) map[SpeciesID]bool {
	out := speciesSet(obs.PokedexOwned)
	for _, e := range obs.Dex.Owned {
		out[e.Species] = true
	}
	return out
}

func evolutionItem(id uint8) ItemID {
	if name, ok := ItemName(id); ok {
		return ItemID(name)
	}
	return ItemID(fmt.Sprintf("item_%02x", id))
}
