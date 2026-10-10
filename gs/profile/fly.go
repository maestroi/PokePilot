package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
)

// The Gold/Silver Fly menu is divided by region and offers only visited
// destinations (pokegold/engine/pokegear/pokegear.asm). The name shown in
// the actual "Where?" picker is the authority for selection; static map IDs
// alone cannot establish whether a destination has been visited.
var gen2FlyDisplayNames = map[string]string{
	"NEW_BARK_TOWN":       "NEW BARK TOWN",
	"CHERRYGROVE_CITY":    "CHERRYGROVE CITY",
	"VIOLET_CITY":         "VIOLET CITY",
	"AZALEA_TOWN":         "AZALEA TOWN",
	"GOLDENROD_CITY":      "GOLDENROD CITY",
	"ECRUTEAK_CITY":       "ECRUTEAK CITY",
	"OLIVINE_CITY":        "OLIVINE CITY",
	"CIANWOOD_CITY":       "CIANWOOD CITY",
	"MAHOGANY_TOWN":       "MAHOGANY TOWN",
	"LAKE_OF_RAGE":        "LAKE OF RAGE",
	"BLACKTHORN_CITY":     "BLACKTHORN CITY",
	"SILVER_CAVE_OUTSIDE": "MT.SILVER",
	"PALLET_TOWN":         "PALLET TOWN",
	"VIRIDIAN_CITY":       "VIRIDIAN CITY",
	"PEWTER_CITY":         "PEWTER CITY",
	"CERULEAN_CITY":       "CERULEAN CITY",
	"VERMILION_CITY":      "VERMILION CITY",
	"ROUTE_10_NORTH":      "ROCK TUNNEL",
	"LAVENDER_TOWN":       "LAVENDER TOWN",
	"CELADON_CITY":        "CELADON CITY",
	"SAFFRON_CITY":        "SAFFRON CITY",
	"FUCHSIA_CITY":        "FUCHSIA CITY",
	"CINNABAR_ISLAND":     "CINNABAR ISLAND",
	"ROUTE_23":            "INDIGO PLATEAU",
}

func (*Profile) FlyDestinationLabel(nativeMapID uint16) (string, bool) {
	info, ok := gsdata.MapByNative(nativeMapID)
	if !ok {
		return "", false
	}
	label, ok := gen2FlyDisplayNames[info.Name]
	return label, ok
}

// DecodeFlyMenu only accepts the cartridge's explicit "Where?" prompt.
// The rendered town label is adjacent to that prompt in the tilemap; this
// works without trusting wTownMapPlayerIconLandmark while its buffer is shared
// with several other Pokegear menus. A caller must still verify the landing.
func (*Profile) DecodeFlyMenu(reader game.MemoryReader) game.FlyMenuState {
	if reader == nil {
		return game.FlyMenuState{}
	}
	text := strings.ToUpper(gsScreenText(reader))
	index := strings.Index(text, "WHERE?")
	if index < 0 {
		return game.FlyMenuState{}
	}
	label := strings.TrimSpace(text[index+len("WHERE?"):])
	return game.FlyMenuState{Visible: true, SelectedLabel: label}
}

var _ game.FlyDestinationDecoder = (*Profile)(nil)
