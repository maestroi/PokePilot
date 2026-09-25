package skill

import "testing"

// Travel leaves an active Safari session only for destinations outside it
// (triage 5a248584293ac9fc): the habitats and rest houses keep the session,
// while the gate and everything beyond it are reached by leaving.
func TestSafariSessionMapBoundary(t *testing.T) {
	for _, mapID := range []uint8{
		safariZoneEastMap, safariZoneNorthMap, safariZoneWestMap, safariZoneCenterMap,
		safariZoneCenterRestHouseMap, safariZoneSecretHouse, safariZoneWestRestHouseMap,
		safariZoneEastRestHouseMap, safariZoneNorthRestHouseMap,
	} {
		if !isSafariSessionMap(mapID) {
			t.Errorf("map %#04x should keep the Safari session", mapID)
		}
	}
	for _, mapID := range []uint8{safariZoneGateMap, fuchsiaMartMap, wardensHouseMap, 0x21 /* Route 22 */, 0xD8, 0xE2} {
		if isSafariSessionMap(mapID) {
			t.Errorf("map %#04x is outside the Safari session", mapID)
		}
	}
	if exit, ok := Place("safari exit approach"); !ok || !isSafariSessionMap(exit.Map) {
		t.Fatalf("safari exit approach %+v must be inside the session, or leaving would recurse", exit)
	}
}
