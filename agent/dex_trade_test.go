package agent

import "testing"

const testTradeMonsOffset = 0x71B7B
const testTradeRowSize = 14

func testNPCTradeROM(index int, give, get uint8) []byte {
	romData := make([]byte, 0x72000)
	off := testTradeMonsOffset + index*testTradeRowSize
	romData[off] = give
	romData[off+1] = get
	return romData
}

func TestAppendDexTradeObjectivesOffersMrMimeWithoutBalls(t *testing.T) {
	romData := testNPCTradeROM(1, 0x94, 0x2A) // Abra -> Mr. Mime
	obs := Observation{
		Map:   0x30, // ROUTE_2_TRADE_HOUSE
		Party: []PartyMon{{Species: "abra", Level: 12}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "mr.mime",
			Sources: []DexSource{{Kind: AcquireInGameTrade, Give: "abra"}},
		}}},
	}

	got := appendDexTradeObjectives(romData, obs, NewKnowledge(nil), nil)
	if len(got) != 1 {
		t.Fatalf("trade objectives = %+v, want one", got)
	}
	o := got[0]
	if o.Kind != KindCatch || o.Species != "mr.mime" || o.Place != "route 2 trade house trade" || o.Intent != dexTradeIntent || !o.Flee {
		t.Fatalf("Mr. Mime trade objective = %+v", o)
	}
}

func TestAppendDexTradeObjectivesRequiresGiveSpeciesInParty(t *testing.T) {
	romData := testNPCTradeROM(1, 0x94, 0x2A)
	obs := Observation{
		Map:   0x30,
		Party: []PartyMon{{Species: "pikachu", Level: 12}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "mr.mime",
			Sources: []DexSource{{Kind: AcquireInGameTrade, Give: "abra"}},
		}}},
	}
	if got := appendDexTradeObjectives(romData, obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("trade objectives without Abra = %+v, want none", got)
	}
}

func TestAppendDexTradeObjectivesProtectsFieldMoveCarrier(t *testing.T) {
	romData := testNPCTradeROM(1, 0x94, 0x2A)
	obs := Observation{
		Map:   0x30,
		Party: []PartyMon{{Species: "abra", Level: 12}},
		FieldCapabilities: []FieldCapability{{
			Name:      "cut",
			Learned:   true,
			PartySlot: 0,
		}},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "mr.mime",
			Sources: []DexSource{{Kind: AcquireInGameTrade, Give: "abra"}},
		}}},
	}
	if got := appendDexTradeObjectives(romData, obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("trade objectives sacrificing field carrier = %+v, want none", got)
	}
}

func TestAppendDexTradeObjectivesSuppressesOwnedTarget(t *testing.T) {
	romData := testNPCTradeROM(1, 0x94, 0x2A)
	obs := Observation{
		Map:          0x30,
		Party:        []PartyMon{{Species: "abra", Level: 12}},
		PokedexOwned: []SpeciesID{"mr.mime"},
		Dex: DexCatalog{Targets: []DexEntry{{
			Species: "mr.mime",
			Sources: []DexSource{{Kind: AcquireInGameTrade, Give: "abra"}},
		}}},
	}
	if got := appendDexTradeObjectives(romData, obs, NewKnowledge(nil), nil); len(got) != 0 {
		t.Fatalf("owned Mr. Mime trade objectives = %+v, want none", got)
	}
}

func TestDexTradeSitePreservesROMTradeIndex(t *testing.T) {
	// Row 0 is deliberately empty. Row 1 is the Route 2 trade. If NPCTrades
	// renumbers filtered rows, this would incorrectly resolve to Route 11.
	romData := testNPCTradeROM(1, 0x94, 0x2A)
	site, ok := dexTradeSite(romData, "mr.mime", "abra")
	if !ok {
		t.Fatal("dexTradeSite did not resolve row 1")
	}
	if site.Index != 1 || site.Map != 0x30 || site.Place != "route 2 trade house trade" {
		t.Fatalf("site = %+v, want Route 2 row 1", site)
	}
}
