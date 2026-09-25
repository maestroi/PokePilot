package agent

import (
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/red/state"
)

// Re-export the portable Dex vocabulary at the planner boundary. Concrete
// profiles own attainability; these aliases preserve the existing agent API.
const (
	AcquireWildGrass   = gameruntime.AcquireWildGrass
	AcquireWildWater   = gameruntime.AcquireWildWater
	AcquireFishing     = gameruntime.AcquireFishing
	AcquireLevelEvo    = gameruntime.AcquireLevelEvo
	AcquireItemEvo     = gameruntime.AcquireItemEvo
	AcquireTradeEvo    = gameruntime.AcquireTradeEvo
	AcquireInGameTrade = gameruntime.AcquireInGameTrade
	AcquireGift        = gameruntime.AcquireGift
	AcquireStatic      = gameruntime.AcquireStatic
	AcquireFossil      = gameruntime.AcquireFossil
	AcquireStarter     = gameruntime.AcquireStarter
)

const (
	UnavailableTradeEvolution = gameruntime.UnavailableTradeEvolution
	UnavailableEventOnly      = gameruntime.UnavailableEventOnly
	UnavailableNoLocalSource  = gameruntime.UnavailableNoLocalSource
	UnavailableForfeited      = gameruntime.UnavailableForfeited
)

type (
	DexSource  = gameruntime.DexSource
	DexEntry   = gameruntime.DexEntry
	DexCatalog = gameruntime.DexCatalog
)

// ProjectPokedex is the legacy agent-facing wrapper. The authoritative
// generation-specific projection now lives on the Red/Blue profile boundary.
func ProjectPokedex(romData []byte, dex state.PokedexState) (owned, seen []SpeciesID) {
	return redprofile.ProjectPokedex(romData, dex)
}

// BuildDexCatalog is retained for existing callers/tests; live observation
// obtains the catalog from game.ProfileObservation instead.
func BuildDexCatalog(romData []byte, owned, seen []SpeciesID) (DexCatalog, error) {
	return redprofile.New().BuildDexCatalog(romData, owned, seen)
}

// assembleDexCatalogWithPolicy lets a non-Red adapter supply its own
// exclusive-choice and event-only policy to the shared catalog assembler.
func assembleDexCatalogWithPolicy(species []DexEntry, sources map[SpeciesID][]DexSource, owned, seen []SpeciesID, exclusives []exclusiveChoice, eventOnly map[SpeciesID]bool) DexCatalog {
	choices := make([]gameruntime.DexExclusiveChoice, len(exclusives))
	for i, choice := range exclusives {
		choices[i] = gameruntime.DexExclusiveChoice{Group: choice.Group, Alternatives: choice.Alternatives}
	}
	return gameruntime.AssembleDexCatalog(species, sources, owned, seen, choices, eventOnly)
}

func assembleDexCatalog(species []DexEntry, sources map[SpeciesID][]DexSource, owned, seen []SpeciesID, exclusives []exclusiveChoice) DexCatalog {
	choices := make([]gameruntime.DexExclusiveChoice, len(exclusives))
	for i, choice := range exclusives {
		choices[i] = gameruntime.DexExclusiveChoice{Group: choice.Group, Alternatives: choice.Alternatives}
	}
	return gameruntime.AssembleDexCatalog(species, sources, owned, seen, choices, redEventOnly())
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
