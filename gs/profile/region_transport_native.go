package profile

import (
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
)

// NativeRegionTransportPlace translates a semantic departure/arrival place to
// the corresponding cartridge-native map id. No map names cross the generic
// routing boundary; this is also compatible with an eventual Crystal adapter.
func (*Profile) NativeRegionTransportPlace(place game.PlaceID) (uint16, bool) {
	if place == "" {
		return 0, false
	}
	for _, info := range gsdata.Maps() {
		if info.Location == place {
			return gsdata.NativeMapID(info.Group, info.Number), true
		}
	}
	return 0, false
}

// NativeRegionTransportRegion classifies a static map's region from its ROM
// landmark. Special/unknown landmarks stay unknown: a planner cannot infer
// that a ship or transitional map belongs to the departure region.
func (*Profile) NativeRegionTransportRegion(romData []byte, mapID uint16) (game.RegionID, bool) {
	if len(romData) == 0 {
		return "", false
	}
	landmark, err := gsrom.MapLandmark(romData, mapID)
	if err != nil {
		return "", false
	}
	region := regionFromLandmark(landmark)
	return region, region != ""
}
