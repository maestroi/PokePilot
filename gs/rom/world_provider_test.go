package rom

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/world"
)

func nativeID(t *testing.T, name string) uint16 {
	t.Helper()
	info, ok := gsdata.MapByName(name)
	if !ok {
		t.Fatalf("generated catalog has no map %q", name)
	}
	return gsdata.NativeMapID(info.Group, info.Number)
}

func TestFirstBadgeWorldProviderNewBarkTopology(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	newBark := nativeID(t, "NEW_BARK_TOWN")
	route29 := nativeID(t, "ROUTE_29")
	elmsLab := nativeID(t, "ELMS_LAB")

	header, err := provider.ParseMap(newBark)
	if err != nil {
		t.Fatalf("ParseMap(NEW_BARK_TOWN): %v", err)
	}
	if header.WidthBlocks != 10 || header.HeightBlocks != 9 {
		t.Fatalf("NEW_BARK_TOWN dimensions = %dx%d, want 10x9 blocks", header.WidthBlocks, header.HeightBlocks)
	}

	foundRoute29 := false
	for _, connection := range header.Connections {
		if connection.MapID == route29 && connection.Dir == dirWest && connection.Offset == 0 {
			foundRoute29 = true
		}
	}
	if !foundRoute29 {
		t.Fatal("NEW_BARK_TOWN missing verified west connection to ROUTE_29")
	}

	foundElm := false
	for _, warp := range header.Warps {
		if warp.X == 6 && warp.Y == 3 && warp.DestMap == elmsLab && warp.DestWarpID == 0 {
			foundElm = true
		}
	}
	if !foundElm {
		t.Fatal("NEW_BARK_TOWN missing verified Elm lab warp")
	}
}

func TestFirstBadgeWorldProviderVioletEntrances(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	violet := nativeID(t, "VIOLET_CITY")
	gym := nativeID(t, "VIOLET_GYM")
	tower := nativeID(t, "SPROUT_TOWER_1F")
	route31 := nativeID(t, "ROUTE_31")

	header, err := provider.ParseMap(violet)
	if err != nil {
		t.Fatalf("ParseMap(VIOLET_CITY): %v", err)
	}

	assertWarp := func(x, y uint8, dest uint16, destWarp uint8) {
		t.Helper()
		for _, warp := range header.Warps {
			if warp.X == x && warp.Y == y && warp.DestMap == dest && warp.DestWarpID == destWarp {
				return
			}
		}
		t.Fatalf("VIOLET_CITY missing warp (%d,%d) -> %#04x/%d", x, y, dest, destWarp)
	}
	assertWarp(18, 17, gym, 0)
	assertWarp(23, 5, tower, 0)

	foundRoute31 := false
	for _, connection := range header.Connections {
		if connection.MapID == route31 && connection.Dir == dirEast && connection.Offset == 9 {
			foundRoute31 = true
		}
	}
	if !foundRoute31 {
		t.Fatal("VIOLET_CITY missing verified east connection to ROUTE_31")
	}
}

func TestFirstBadgeWorldProviderRoutesBootToFalknerGym(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	names := []string{
		"PLAYERS_HOUSE_2F",
		"PLAYERS_HOUSE_1F",
		"NEW_BARK_TOWN",
		"ROUTE_29",
		"CHERRYGROVE_CITY",
		"ROUTE_30",
		"ROUTE_31",
		"VIOLET_CITY",
		"VIOLET_GYM",
	}
	ids := make([]uint16, len(names))
	for i, name := range names {
		ids[i] = nativeID(t, name)
	}

	route, err := world.FindNativeRoute(graph, ids[0], ids[len(ids)-1])
	if err != nil {
		t.Fatalf("FindNativeRoute(boot -> Violet Gym): %v", err)
	}
	if len(route) != len(ids)-1 {
		t.Fatalf("route has %d transitions, want %d: %#v", len(route), len(ids)-1, route)
	}
	for i, edge := range route {
		if edge.From != ids[i] || edge.To != ids[i+1] {
			t.Fatalf("route[%d] = %#04x -> %#04x, want %#04x -> %#04x", i, edge.From, edge.To, ids[i], ids[i+1])
		}
	}
}

func TestFirstBadgeWorldProviderIncludesSproutTowerPath(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}
	violet := nativeID(t, "VIOLET_CITY")
	tower3 := nativeID(t, "SPROUT_TOWER_3F")
	route, err := world.FindNativeRoute(graph, violet, tower3)
	if err != nil {
		t.Fatalf("FindNativeRoute(Violet -> Sprout Tower 3F): %v", err)
	}
	if len(route) != 3 {
		t.Fatalf("Violet -> Sprout Tower 3F has %d transitions, want 3: %#v", len(route), route)
	}
}

func TestEarlyJohtoWorldProviderRoutesVioletThroughUnionCaveToAzalea(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	names := []string{
		"VIOLET_CITY",
		"ROUTE_32",
		"UNION_CAVE_1F",
		"ROUTE_33",
		"AZALEA_TOWN",
	}
	ids := make([]uint16, len(names))
	for i, name := range names {
		ids[i] = nativeID(t, name)
	}

	route, err := world.FindNativeRoute(graph, ids[0], ids[len(ids)-1])
	if err != nil {
		t.Fatalf("FindNativeRoute(Violet -> Azalea): %v", err)
	}
	if len(route) != len(ids)-1 {
		t.Fatalf("route has %d transitions, want %d: %#v", len(route), len(ids)-1, route)
	}
	for i, edge := range route {
		if edge.From != ids[i] || edge.To != ids[i+1] {
			t.Fatalf("route[%d] = %#04x -> %#04x, want %#04x -> %#04x", i, edge.From, edge.To, ids[i], ids[i+1])
		}
	}
}

func TestEarlyJohtoWorldProviderIncludesAzaleaStoryWarps(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	azalea := nativeID(t, "AZALEA_TOWN")
	center := nativeID(t, "AZALEA_POKECENTER_1F")
	kurt := nativeID(t, "KURTS_HOUSE")
	gym := nativeID(t, "AZALEA_GYM")
	well := nativeID(t, "SLOWPOKE_WELL_B1F")

	header, err := provider.ParseMap(azalea)
	if err != nil {
		t.Fatalf("ParseMap(AZALEA_TOWN): %v", err)
	}

	foundCenter := false
	foundKurt := false
	foundGym := false
	foundWell := false
	for _, warp := range header.Warps {
		switch {
		case warp.X == 15 && warp.Y == 9 && warp.DestMap == center && warp.DestWarpID == 0:
			foundCenter = true
		case warp.X == 9 && warp.Y == 5 && warp.DestMap == kurt && warp.DestWarpID == 0:
			foundKurt = true
		case warp.X == 10 && warp.Y == 15 && warp.DestMap == gym && warp.DestWarpID == 0:
			foundGym = true
		case warp.X == 31 && warp.Y == 7 && warp.DestMap == well && warp.DestWarpID == 0:
			foundWell = true
		}
	}
	if !foundCenter || !foundKurt || !foundGym || !foundWell {
		t.Fatalf("Azalea story warps: Center=%v Kurt=%v Gym=%v Well=%v header=%+v", foundCenter, foundKurt, foundGym, foundWell, header.Warps)
	}

	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}
	route, err := world.FindNativeRoute(graph, kurt, gym)
	if err != nil {
		t.Fatalf("FindNativeRoute(Kurt -> Azalea Gym): %v", err)
	}
	if len(route) != 2 || route[0].From != kurt || route[0].To != azalea || route[1].From != azalea || route[1].To != gym {
		t.Fatalf("Kurt -> Azalea Gym route = %#v, want Kurt -> Azalea -> Gym", route)
	}
}

func TestEarlyJohtoWorldProviderRoutesAzaleaThroughIlexToGoldenrod(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	names := []string{
		"AZALEA_TOWN",
		"ILEX_FOREST_AZALEA_GATE",
		"ILEX_FOREST",
		"ROUTE_34_ILEX_FOREST_GATE",
		"ROUTE_34",
		"GOLDENROD_CITY",
	}
	ids := make([]uint16, len(names))
	for i, name := range names {
		ids[i] = nativeID(t, name)
	}
	route, err := world.FindNativeRoute(graph, ids[0], ids[len(ids)-1])
	if err != nil {
		t.Fatalf("FindNativeRoute(Azalea -> Goldenrod): %v", err)
	}
	if len(route) != len(ids)-1 {
		t.Fatalf("route has %d transitions, want %d: %#v", len(route), len(ids)-1, route)
	}
	for i, edge := range route {
		if edge.From != ids[i] || edge.To != ids[i+1] {
			t.Fatalf("route[%d] = %#04x -> %#04x, want %#04x -> %#04x", i, edge.From, edge.To, ids[i], ids[i+1])
		}
	}
}

func TestEarlyJohtoWorldProviderRoutesGoldenrodToEcruteak(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	names := []string{
		"GOLDENROD_CITY",
		"ROUTE_35",
		"ROUTE_36",
		"ROUTE_37",
		"ECRUTEAK_CITY",
	}
	ids := make([]uint16, len(names))
	for i, name := range names {
		ids[i] = nativeID(t, name)
	}
	route, err := world.FindNativeRoute(graph, ids[0], ids[len(ids)-1])
	if err != nil {
		t.Fatalf("FindNativeRoute(Goldenrod -> Ecruteak): %v", err)
	}
	if len(route) != len(ids)-1 {
		t.Fatalf("route has %d transitions, want %d: %#v", len(route), len(ids)-1, route)
	}
	for i, edge := range route {
		if edge.From != ids[i] || edge.To != ids[i+1] {
			t.Fatalf("route[%d] = %#04x -> %#04x, want %#04x -> %#04x", i, edge.From, edge.To, ids[i], ids[i+1])
		}
	}
}

func TestEarlyJohtoWorldProviderGoldenrodAndEcruteakServices(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	for _, pair := range [][2]string{
		{"GOLDENROD_CITY", "GOLDENROD_GYM"},
		{"GOLDENROD_CITY", "GOLDENROD_POKECENTER_1F"},
		{"GOLDENROD_CITY", "GOLDENROD_DEPT_STORE_1F"},
		{"ECRUTEAK_CITY", "ECRUTEAK_GYM"},
		{"ECRUTEAK_CITY", "ECRUTEAK_POKECENTER_1F"},
		{"ECRUTEAK_CITY", "ECRUTEAK_MART"},
	} {
		from := nativeID(t, pair[0])
		to := nativeID(t, pair[1])
		route, err := world.FindNativeRoute(graph, from, to)
		if err != nil {
			t.Fatalf("FindNativeRoute(%s -> %s): %v", pair[0], pair[1], err)
		}
		if len(route) != 1 || route[0].From != from || route[0].To != to {
			t.Fatalf("%s -> %s route = %#v, want one transition", pair[0], pair[1], route)
		}
	}
}

func TestEarlyJohtoWorldProviderKeepsRealRoute36VioletConnection(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	route36 := nativeID(t, "ROUTE_36")
	violet := nativeID(t, "VIOLET_CITY")

	header, err := provider.ParseMap(route36)
	if err != nil {
		t.Fatalf("ParseMap(ROUTE_36): %v", err)
	}
	for _, connection := range header.Connections {
		if connection.MapID == violet && connection.Dir == dirEast && connection.Offset == 0 {
			return
		}
	}
	t.Fatal("ROUTE_36 missing decomp east connection to VIOLET_CITY")
}

func TestEarlyJohtoWorldProviderNewMapsHaveCollisionTilesets(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	for _, name := range []string{
		"GOLDENROD_GYM",
		"GOLDENROD_FLOWER_SHOP",
		"GOLDENROD_POKECENTER_1F",
		"GOLDENROD_DEPT_STORE_1F",
		"ROUTE_35_GOLDENROD_GATE",
		"ROUTE_35",
		"ROUTE_36",
		"ROUTE_37",
		"ECRUTEAK_CITY",
		"ECRUTEAK_GYM",
		"BURNED_TOWER_1F",
		"BURNED_TOWER_B1F",
		"ECRUTEAK_POKECENTER_1F",
		"ECRUTEAK_MART",
	} {
		id := nativeID(t, name)
		info, ok := gsdata.Map(id)
		if !ok {
			t.Fatalf("generated catalog has no map %q", name)
		}
		blocks := make([]byte, int(info.WidthBlocks)*int(info.HeightBlocks))
		if _, err := provider.Grid(id, blocks, 0); err != nil {
			t.Fatalf("Grid(%s): %v", name, err)
		}
	}
}

func TestEarlyJohtoWorldProviderRoutesStoryServicesForFogBadgeSlice(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	graph, err := world.BuildNativeGraph(provider)
	if err != nil {
		t.Fatalf("BuildNativeGraph: %v", err)
	}

	for _, pair := range [][2]string{
		{"GOLDENROD_CITY", "GOLDENROD_FLOWER_SHOP"},
		{"ECRUTEAK_CITY", "BURNED_TOWER_1F"},
		{"BURNED_TOWER_1F", "BURNED_TOWER_B1F"},
		{"ECRUTEAK_CITY", "ECRUTEAK_GYM"},
	} {
		from := nativeID(t, pair[0])
		to := nativeID(t, pair[1])
		route, err := world.FindNativeRoute(graph, from, to)
		if err != nil {
			t.Fatalf("FindNativeRoute(%s -> %s): %v", pair[0], pair[1], err)
		}
		if len(route) == 0 {
			t.Fatalf("%s -> %s returned empty route", pair[0], pair[1])
		}
	}
}

func TestEarlyJohtoEcruteakGymLandingWarpsAreInert(t *testing.T) {
	provider := NewFirstBadgeWorldProvider(nil)
	gym := nativeID(t, "ECRUTEAK_GYM")
	header, err := provider.ParseMap(gym)
	if err != nil {
		t.Fatalf("ParseMap(ECRUTEAK_GYM): %v", err)
	}

	inert := map[[2]uint8]bool{}
	for _, warp := range header.Warps {
		if warp.Inert {
			inert[[2]uint8{warp.X, warp.Y}] = true
		}
	}
	for _, at := range [][2]uint8{{4, 14}, {6, 7}} {
		if !inert[at] {
			t.Fatalf("Ecruteak Gym landing warp (%d,%d) is not inert", at[0], at[1])
		}
	}
	if inert[[2]uint8{2, 4}] {
		t.Fatal("Ecruteak Gym pit warp (2,4) was incorrectly marked inert")
	}
}
