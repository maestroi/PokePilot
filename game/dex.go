package game

import "sort"

// Dex acquisition kinds are portable planner vocabulary. Concrete profiles own
// how ROM tables, scripts, version exclusives, trades, and one-time choices map
// into these semantic methods.
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

// DexSource is one legitimate way the active save/profile can obtain a species.
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

// DexEntry is one semantic Pokédex entry as generic planning should see it.
type DexEntry struct {
	Species     SpeciesID   `json:"species"`
	Dex         uint8       `json:"dex"`
	Owned       bool        `json:"owned,omitempty"`
	Seen        bool        `json:"seen,omitempty"`
	Sources     []DexSource `json:"sources,omitempty"`
	Unavailable string      `json:"unavailable,omitempty"`
}

// DexCatalog is the profile-owned completion model for the current save.
type DexCatalog struct {
	Owned       []DexEntry `json:"owned"`
	Targets     []DexEntry `json:"targets"`
	Unavailable []DexEntry `json:"unavailable"`
	// IncompleteReason is set when the adapter knows its source model does not
	// yet cover every acquisition path. Dex completion must not be certified
	// while it is non-empty.
	IncompleteReason string `json:"incomplete_reason,omitempty"`
}

// DexExclusiveChoice describes mutually exclusive single-save acquisition
// branches such as starters or one-time fossil choices.
type DexExclusiveChoice struct {
	Group        string
	Alternatives [][]SpeciesID
}

// DexProfile is an optional GameProfile capability. Profiles that support Dex
// goals classify the active ROM/save into owned, attainable targets, and
// unavailable entries. Generic policy consumes the result without knowing game
// names, native species indexes, ROM layouts, or version-exclusive facts.
type DexProfile interface {
	GameProfile
	BuildDexCatalog(rom []byte, owned, seen []SpeciesID) (DexCatalog, error)
}

// AssembleDexCatalog applies the game-independent closure rules once a concrete
// profile has supplied species, sources, mutually-exclusive choices, and
// event-only facts.
func AssembleDexCatalog(species []DexEntry, sources map[SpeciesID][]DexSource, owned, seen []SpeciesID, exclusives []DexExclusiveChoice, eventOnly map[SpeciesID]bool) DexCatalog {
	ownedSet := dexSpeciesSet(owned)
	seenSet := dexSpeciesSet(seen)
	forfeited := dexForfeitedSpecies(ownedSet, exclusives)

	direct := map[SpeciesID]bool{}
	for id, srcs := range sources {
		if forfeited[id] != "" {
			continue
		}
		if dexHasDirectSource(srcs) {
			direct[id] = true
		}
	}
	local := dexCloseLocal(direct, ownedSet, sources, forfeited)

	cat := DexCatalog{
		Owned:       []DexEntry{},
		Targets:     []DexEntry{},
		Unavailable: []DexEntry{},
	}
	for _, entry := range species {
		entry.Owned = ownedSet[entry.Species]
		entry.Seen = seenSet[entry.Species]
		entry.Sources = append([]DexSource(nil), sources[entry.Species]...)
		dexSortSources(entry.Sources)
		switch {
		case entry.Owned:
			cat.Owned = append(cat.Owned, entry)
		case forfeited[entry.Species] != "":
			entry.Unavailable = UnavailableForfeited + ":" + forfeited[entry.Species]
			cat.Unavailable = append(cat.Unavailable, entry)
		case local[entry.Species]:
			cat.Targets = append(cat.Targets, entry)
		case eventOnly[entry.Species]:
			entry.Unavailable = UnavailableEventOnly
			cat.Unavailable = append(cat.Unavailable, entry)
		case dexOnlyTradeEvo(entry.Sources):
			entry.Unavailable = UnavailableTradeEvolution
			cat.Unavailable = append(cat.Unavailable, entry)
		default:
			entry.Unavailable = UnavailableNoLocalSource
			cat.Unavailable = append(cat.Unavailable, entry)
		}
	}
	return cat
}

func dexSpeciesSet(ids []SpeciesID) map[SpeciesID]bool {
	out := map[SpeciesID]bool{}
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func dexForfeitedSpecies(owned map[SpeciesID]bool, choices []DexExclusiveChoice) map[SpeciesID]string {
	out := map[SpeciesID]string{}
	for _, choice := range choices {
		chosen := -1
		for i, alt := range choice.Alternatives {
			if dexIntersects(owned, alt) {
				chosen = i
				break
			}
		}
		if chosen < 0 {
			continue
		}
		for i, alt := range choice.Alternatives {
			if i == chosen {
				continue
			}
			for _, id := range alt {
				out[id] = choice.Group
			}
		}
	}
	return out
}

func dexIntersects(owned map[SpeciesID]bool, ids []SpeciesID) bool {
	for _, id := range ids {
		if owned[id] {
			return true
		}
	}
	return false
}

func dexHasDirectSource(srcs []DexSource) bool {
	for _, src := range srcs {
		switch src.Kind {
		case AcquireWildGrass, AcquireWildWater, AcquireFishing, AcquireInGameTrade,
			AcquireGift, AcquireStatic, AcquireFossil, AcquireStarter:
			return true
		}
	}
	return false
}

func dexCloseLocal(direct, owned map[SpeciesID]bool, sources map[SpeciesID][]DexSource, forfeited map[SpeciesID]string) map[SpeciesID]bool {
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

func dexOnlyTradeEvo(srcs []DexSource) bool {
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

func dexSortSources(srcs []DexSource) {
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
