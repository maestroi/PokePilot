package rom

import (
	"testing"

	gsdata "github.com/maestroi/pokepilot/gs/data"
	"github.com/maestroi/pokepilot/world"
)

func TestParseNewBarkTownAgainstDecomp(t *testing.T) {
	rom := loadGold(t)
	newBark := nativeID(t, "NEW_BARK_TOWN")
	h, err := ParseMap(rom, newBark)
	if err != nil {
		t.Fatalf("ParseMap(NEW_BARK_TOWN): %v", err)
	}
	if h.WidthBlocks != 10 || h.HeightBlocks != 9 {
		t.Fatalf("NEW_BARK_TOWN dimensions = %dx%d, want 10x9", h.WidthBlocks, h.HeightBlocks)
	}
	if len(h.Warps) != 4 {
		t.Fatalf("NEW_BARK_TOWN warps = %d, want 4: %+v", len(h.Warps), h.Warps)
	}
	elm := nativeID(t, "ELMS_LAB")
	foundElm := false
	for _, w := range h.Warps {
		if w.X == 6 && w.Y == 3 && w.DestMap == elm && w.DestWarpID == 0 {
			foundElm = true
		}
	}
	if !foundElm {
		t.Fatalf("NEW_BARK_TOWN missing Elm lab warp: %+v", h.Warps)
	}
	route29 := nativeID(t, "ROUTE_29")
	foundWest := false
	for _, c := range h.Connections {
		if c.MapID == route29 && c.Dir == dirWest && c.Offset == 0 {
			foundWest = true
		}
	}
	if !foundWest {
		t.Fatalf("NEW_BARK_TOWN missing west Route 29: %+v", h.Connections)
	}
	if len(h.Objects) != 3 {
		t.Fatalf("NEW_BARK_TOWN objects = %d, want 3", len(h.Objects))
	}
	if h.Objects[0].X != 6 || h.Objects[0].Y != 8 {
		t.Fatalf("teacher home = (%d,%d), want (6,8)", h.Objects[0].X, h.Objects[0].Y)
	}
	if h.Objects[0].Type != ObjectTypeScript {
		t.Fatalf("teacher type = %d, want script", h.Objects[0].Type)
	}
}

func TestParseVioletCityObjectsAndWarps(t *testing.T) {
	rom := loadGold(t)
	violet := nativeID(t, "VIOLET_CITY")
	h, err := ParseMap(rom, violet)
	if err != nil {
		t.Fatalf("ParseMap(VIOLET_CITY): %v", err)
	}
	gym := nativeID(t, "VIOLET_GYM")
	tower := nativeID(t, "SPROUT_TOWER_1F")
	foundGym, foundTower := false, false
	for _, w := range h.Warps {
		if w.X == 18 && w.Y == 17 && w.DestMap == gym && w.DestWarpID == 0 {
			foundGym = true
		}
		if w.X == 23 && w.Y == 5 && w.DestMap == tower && w.DestWarpID == 0 {
			foundTower = true
		}
	}
	if !foundGym || !foundTower {
		t.Fatalf("Violet warps gym=%v tower=%v: %+v", foundGym, foundTower, h.Warps)
	}
}

func TestSpeciesTableNationalDexOrder(t *testing.T) {
	rom := loadGold(t)
	for _, tc := range []struct {
		raw  uint8
		name string
		id   string
	}{
		{1, "BULBASAUR", "bulbasaur"},
		{151, "MEW", "mew"},
		{152, "CHIKORITA", "chikorita"},
		{251, "CELEBI", "celebi"},
	} {
		got, err := SpeciesName(rom, tc.raw)
		if err != nil {
			t.Fatalf("SpeciesName(%d): %v", tc.raw, err)
		}
		if got != tc.name {
			t.Fatalf("SpeciesName(%d)=%q, want %q", tc.raw, got, tc.name)
		}
		id, ok := Species(rom, tc.raw)
		if !ok || string(id) != tc.id {
			t.Fatalf("Species(%d)=(%q,%v), want %q", tc.raw, id, ok, tc.id)
		}
	}
}

func TestWildEncountersHaveDayNightSlots(t *testing.T) {
	rom := loadGold(t)
	enc, err := WildEncounters(rom)
	if err != nil {
		t.Fatalf("WildEncounters: %v", err)
	}
	route29 := nativeID(t, "ROUTE_29")
	var mornPidgey, niteHoothoot, daySentret bool
	for _, e := range enc {
		if e.MapID != route29 || e.Habitat != HabitatGrass {
			continue
		}
		if e.Time == TimeMorning && e.Species == 0x10 && e.Level == 2 {
			mornPidgey = true
		}
		if e.Time == TimeDay && e.Species == 0xa1 && e.Level == 3 {
			daySentret = true
		}
		if e.Time == TimeNight && e.Species == 0xa3 && e.Level == 2 {
			niteHoothoot = true
		}
	}
	if !mornPidgey || !daySentret || !niteHoothoot {
		t.Fatalf("Route 29 grass slots: mornPidgey=%v daySentret=%v niteHoothoot=%v (n=%d)",
			mornPidgey, daySentret, niteHoothoot, len(enc))
	}
}

func TestMovesIncludeDarkSteelAndCut(t *testing.T) {
	rom := loadGold(t)
	cut, err := LookupMove(rom, 0x0f)
	if err != nil {
		t.Fatalf("LookupMove(CUT): %v", err)
	}
	if cut.Power != 50 || cut.Type != TypeNormal || cut.PP != 30 {
		t.Fatalf("CUT = %+v, want power 50 type NORMAL pp 30", cut)
	}
	crunch, err := LookupMove(rom, 242) // CRUNCH
	if err != nil {
		t.Fatalf("LookupMove(CRUNCH): %v", err)
	}
	if crunch.Type != TypeDark {
		t.Fatalf("CRUNCH type = %#02x, want Dark %#02x", crunch.Type, TypeDark)
	}
	ironTail, err := LookupMove(rom, 231) // IRON_TAIL
	if err != nil {
		t.Fatalf("LookupMove(IRON_TAIL): %v", err)
	}
	if ironTail.Type != TypeSteel {
		t.Fatalf("IRON_TAIL type = %#02x, want Steel %#02x", ironTail.Type, TypeSteel)
	}
}

func TestTMHMListIncludesWhirlpoolAndWaterfall(t *testing.T) {
	rom := loadGold(t)
	cut, err := TMHMMove(rom, 51)
	if err != nil {
		t.Fatalf("HM01: %v", err)
	}
	if cut != 0x0f {
		t.Fatalf("HM01 move = %#02x, want CUT", cut)
	}
	whirl, err := TMHMMove(rom, 56)
	if err != nil {
		t.Fatalf("HM06: %v", err)
	}
	if whirl != 0xfa {
		t.Fatalf("HM06 move = %#02x, want WHIRLPOOL", whirl)
	}
	water, err := TMHMMove(rom, 57)
	if err != nil {
		t.Fatalf("HM07: %v", err)
	}
	if water != 0x7f {
		t.Fatalf("HM07 move = %#02x, want WATERFALL", water)
	}
}

func TestEvolutionsIncludeFriendshipTimeAndTradeItem(t *testing.T) {
	rom := loadGold(t)
	evos, err := Evolutions(rom)
	if err != nil {
		t.Fatalf("Evolutions: %v", err)
	}
	var eeveeEspeon, pichuPika, tyrogue, onixSteel bool
	for _, e := range evos {
		if e.From == 0x85 && e.Method == EvoHappiness && e.TimeOrStat == HappinessMornDay && e.To == 196 {
			eeveeEspeon = true
		}
		if e.From == 0xac && e.Method == EvoHappiness && e.TimeOrStat == HappinessAnytime && e.To == 0x19 {
			pichuPika = true
		}
		if e.From == 0xec && e.Method == EvoStat && e.Level == 20 && e.To == 0x6b {
			tyrogue = true
		}
		if e.From == 0x5f && e.Method == EvoTrade && e.Item != 0 && e.Item != 0xff && e.To == 0xd0 {
			onixSteel = true
		}
	}
	if !eeveeEspeon || !pichuPika || !tyrogue || !onixSteel {
		t.Fatalf("evo flags: espeon=%v pichu=%v tyrogue=%v steelix=%v (n=%d)",
			eeveeEspeon, pichuPika, tyrogue, onixSteel, len(evos))
	}
}

func TestMartsAndItemAttributes(t *testing.T) {
	rom := loadGold(t)
	items, err := MartItems(rom, 0) // MART_CHERRYGROVE
	if err != nil {
		t.Fatalf("MartItems(Cherrygrove): %v", err)
	}
	want := []uint8{0x12, 0x09, 0x0d, 0x0c}
	if len(items) != len(want) {
		t.Fatalf("Cherrygrove items = %v, want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("Cherrygrove items = %v, want %v", items, want)
		}
	}
	ball, err := LookupItem(rom, 0x05)
	if err != nil {
		t.Fatalf("LookupItem(POKE_BALL): %v", err)
	}
	if ball.Price != 200 || ball.Pocket != 3 {
		t.Fatalf("POKE_BALL = %+v, want price 200 pocket BALL(3)", ball)
	}
	bright, err := LookupItem(rom, 0x03)
	if err != nil {
		t.Fatalf("LookupItem(BRIGHTPOWDER): %v", err)
	}
	if bright.HeldEffect == 0 {
		t.Fatal("BRIGHTPOWDER should have a held-item effect")
	}
}

func TestExperienceCurvesFromBaseData(t *testing.T) {
	rom := loadGold(t)
	// Chikorita is Medium Slow (same family as Bulbasaur).
	got, err := LookupSpeciesExperience(rom, 0x98)
	if err != nil {
		t.Fatalf("LookupSpeciesExperience(Chikorita): %v", err)
	}
	if got.Growth != GrowthMediumSlow {
		t.Fatalf("Chikorita growth = %d, want Medium Slow", got.Growth)
	}
}

func TestWorldGraphCoversBothRegions(t *testing.T) {
	for _, env := range []string{"POKEMON_GOLD_ROM", "POKEMON_SILVER_ROM"} {
		t.Run(env, func(t *testing.T) {
			rom := loadROM(t, env)
			provider, err := NewWorldProvider(rom)
			if err != nil {
				t.Fatalf("NewWorldProvider: %v", err)
			}
			ids := provider.MapIDs()
			if len(ids) != len(gsdata.Maps()) {
				t.Fatalf("MapIDs = %d, catalog %d", len(ids), len(gsdata.Maps()))
			}
			graph, err := world.BuildNativeGraph(provider)
			if err != nil {
				t.Fatalf("BuildNativeGraph: %v", err)
			}
			var johto, kanto bool
			for _, id := range ids {
				info, _ := gsdata.Map(id)
				switch info.Name {
				case "NEW_BARK_TOWN":
					johto = true
				case "PALLET_TOWN":
					kanto = true
				}
				header, err := provider.ParseMap(id)
				if err != nil {
					t.Fatalf("ParseMap(%s): %v", info.Name, err)
				}
				for _, w := range header.Warps {
					if w.DestMap&0xff == 0xff || w.DestMap>>8 == 0xff {
						t.Fatalf("%s warp dest is 0xFF: %+v", info.Name, w)
					}
				}
			}
			if !johto || !kanto {
				t.Fatal("graph missing Johto or Kanto start towns")
			}
			if _, err := world.FindNativeRoute(graph, nativeID(t, "NEW_BARK_TOWN"), nativeID(t, "PALLET_TOWN")); err != nil {
				t.Fatalf("FindNativeRoute(New Bark -> Pallet): %v", err)
			}
			if _, err := world.FindNativeRoute(graph, nativeID(t, "PLAYERS_HOUSE_2F"), nativeID(t, "VIOLET_GYM")); err != nil {
				t.Fatalf("FindNativeRoute(boot -> Violet Gym): %v", err)
			}
		})
	}
}

func TestFirstBadgeSliceStillMatchesROM(t *testing.T) {
	rom := loadGold(t)
	hand := NewFirstBadgeWorldProvider(rom)
	full, err := NewWorldProvider(rom)
	if err != nil {
		t.Fatalf("NewWorldProvider: %v", err)
	}
	newBark := nativeID(t, "NEW_BARK_TOWN")
	want, err := hand.ParseMap(newBark)
	if err != nil {
		t.Fatalf("first-badge ParseMap: %v", err)
	}
	got, err := full.ParseMap(newBark)
	if err != nil {
		t.Fatalf("ROM ParseMap: %v", err)
	}
	if len(got.Warps) != len(want.Warps) {
		t.Fatalf("warp count ROM %d vs first-badge %d", len(got.Warps), len(want.Warps))
	}
	for i := range want.Warps {
		if got.Warps[i] != want.Warps[i] {
			t.Fatalf("warp[%d] ROM %+v vs first-badge %+v", i, got.Warps[i], want.Warps[i])
		}
	}
}
