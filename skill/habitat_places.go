package skill

// habitatPlaces makes every ordinary wild-encounter map travel-addressable.
// Training-area learning (agent rememberTrainingArea) can only record a
// habitat whose location has a catalog Place, so a cave or route missing here
// is invisible to combat preparation no matter how often the run walks it.
// run-1nnzti2l332xm2pdodobrapjlh lost to Lorelei, respawned at Indigo, and was
// told "no viable local training" while Victory Road sat one warp away: it had
// crossed all three floors but none of them had a Place.
//
// Names are the semantic location of the ROM map name, so the Place and the
// learned TrainingArea location line up. Map arrival is the goal; any tile of
// the map the router reaches first satisfies it. X/Y are only a measured
// standable hint (beside the map's first usable warp). TestEveryWildHabitatHasPlace
// keeps this list complete against the ROM.
var habitatPlaces = map[string]Destination{
	"route 5":             {Map: 0x10, X: 10, Y: 28},
	"route 6":             {Map: 0x11, X: 8, Y: 1},
	"route 11":            {Map: 0x16, X: 48, Y: 8},
	"route 12":            {Map: 0x17, X: 10, Y: 14},
	"route 16":            {Map: 0x1B, X: 16, Y: 10},
	"route 17":            {Map: 0x1C, X: 1, Y: 0},
	"route 18":            {Map: 0x1D, X: 33, Y: 7},
	"route 21":            {Map: 0x20, X: 0, Y: 1},
	"route 23":            {Map: 0x22, X: 7, Y: 138},
	"power plant":         {Map: 0x53, X: 4, Y: 34},
	"victory road 1f":     {Map: 0x6C, X: 8, Y: 16},
	"seafoam islands b1f": {Map: 0x9F, X: 4, Y: 3},
	"seafoam islands b2f": {Map: 0xA0, X: 5, Y: 4},
	"seafoam islands b3f": {Map: 0xA1, X: 5, Y: 11},
	"seafoam islands b4f": {Map: 0xA2, X: 11, Y: 8},
	"seafoam islands 1f":  {Map: 0xC0, X: 4, Y: 16},
	"victory road 2f":     {Map: 0xC2, X: 0, Y: 9},
	"digletts cave":       {Map: 0xC5, X: 5, Y: 6},
	"victory road 3f":     {Map: 0xC6, X: 23, Y: 8},
	"pokemon mansion 2f":  {Map: 0xD6, X: 5, Y: 11},
	"pokemon mansion 3f":  {Map: 0xD7, X: 7, Y: 11},
	"pokemon mansion b1f": {Map: 0xD8, X: 23, Y: 23},
	"cerulean cave 2f":    {Map: 0xE2, X: 29, Y: 0},
	"cerulean cave b1f":   {Map: 0xE3, X: 3, Y: 5},
	"cerulean cave 1f":    {Map: 0xE4, X: 24, Y: 16},
	"rock tunnel b1f":     {Map: 0xE8, X: 33, Y: 26},
}

func init() {
	for name, d := range habitatPlaces {
		if _, ok := places[name]; !ok {
			d.Kind = DestinationMap
			places[name] = d
		}
	}
}
