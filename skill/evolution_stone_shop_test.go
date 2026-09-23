package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// waterStoneItem is WATER_STONE ($22) in the Gen 1 item table.
const waterStoneItem uint8 = 0x22

func TestEvolutionStoneShopUsesInteractionDestination(t *testing.T) {
	dest, ok := Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("Celadon Mart 4F stone shop destination missing")
	}
	if dest.Kind != DestinationInteraction {
		t.Fatalf("stone shop destination kind = %s, want interaction", dest.KindName())
	}
	if dest.Map != celadonMart4FMap || dest.X != celadonMart4FClerkX || dest.Y != celadonMart4FClerkY {
		t.Fatalf("stone shop target = map %02x (%d,%d), want clerk %02x (%d,%d)",
			dest.Map, dest.X, dest.Y,
			celadonMart4FMap, celadonMart4FClerkX, celadonMart4FClerkY)
	}
}

// TestEvolutionStoneShopClerkSitsBehindCounter pins the ROM fact the purchase
// depends on. Run-jxh8lk19wv6on burned 45 identical retries inside
// BuyEvolutionStone because the skill faced the clerk's own tile while Travel
// had correctly parked Red in the service-counter approach two tiles away, so
// every attempt failed the same way and the planner could never make progress.
//
// The interaction target must be a real map object, and the counter approach
// Travel actually lands on must resolve to the counter tile between Red and
// the clerk. Facing the clerk's own tile from there is two tiles away, which
// Face rejects outright.
func TestEvolutionStoneShopClerkSitsBehindCounter(t *testing.T) {
	romData := badgeFourROM(t)
	dest, ok := Place("celadon mart 4f stones")
	if !ok {
		t.Fatal("celadon mart 4f stones place is missing")
	}

	h, err := rom.ParseMap(romData, dest.Map)
	if err != nil {
		t.Fatalf("ParseMap(%#04x): %v", dest.Map, err)
	}
	// An interaction destination names the object's tile, not a standing tile,
	// so the target must be a real map object. This is the assertion that would
	// have caught the original off-map (5,8) destination.
	objectAt := false
	for _, object := range h.Objects {
		if object.X == dest.X && object.Y == dest.Y {
			objectAt = true
			break
		}
	}
	if !objectAt {
		t.Fatalf("interaction target (%d,%d) is not a map object on map %#04x", dest.X, dest.Y, dest.Map)
	}

	g, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build(%#04x): %v", dest.Map, err)
	}
	approaches := 0
	for _, s := range counterSteps {
		cx, cy := int(dest.X)+s.DX, int(dest.Y)+s.DY
		sx, sy := int(dest.X)+2*s.DX, int(dest.Y)+2*s.DY
		if !g.IsCounterTile(cx, cy) || !g.InBounds(sx, sy) || !g.Walkable(sx, sy) {
			continue
		}
		approaches++
		faceX, faceY, ok := counterFacing(g, uint8(sx), uint8(sy), dest.X, dest.Y)
		if !ok {
			t.Fatalf("standing at (%d,%d), counterFacing found no counter tile toward the clerk at (%d,%d)", sx, sy, dest.X, dest.Y)
		}
		if int(faceX) != cx || int(faceY) != cy {
			t.Fatalf("counterFacing from (%d,%d) = (%d,%d), want the counter tile (%d,%d)", sx, sy, faceX, faceY, cx, cy)
		}
	}
	if approaches == 0 {
		t.Fatalf("clerk at (%d,%d) on map %#04x has no service-counter approach", dest.X, dest.Y, dest.Map)
	}
}

// TestBuyEvolutionStoneRealROM replays the exact checkpoint that produced
// run-jxh8lk19wv6on's endless "buy 1 WATER STONE" loop and asserts the
// objective's positive postcondition: the money leaves the wallet, the stone
// lands in the bag, and Red returns controllable on Celadon Mart 4F. Before
// the counter-aware facing fix this died at "Face: tile (5,7) is not
// orthogonally adjacent to (5,5)" on every attempt, so the failure fingerprint
// never changed and the farm's circuit breaker opened.
//
// The state is external because repository policy forbids committing
// ROM-derived .state files; skill/safari_flee_real_test.go uses the same
// pattern. It is the smallest deterministic replay of the defect: travel,
// counter approach and purchase, from one checkpoint, in about a second.
func TestBuyEvolutionStoneRealROM(t *testing.T) {
	path := os.Getenv("POKEPILOT_MART4F_STONE_TEST_STATE")
	if path == "" {
		t.Skip("POKEPILOT_MART4F_STONE_TEST_STATE not set (real-ROM prepared-state test)")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read POKEPILOT_MART4F_STONE_TEST_STATE=%s: %v", path, err)
	}
	romData, err := os.ReadFile(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatalf("read POKEMON_RED_ROM: %v", err)
	}
	m := openEmuCGB(t)
	if err := m.LoadState(blob); err != nil {
		t.Fatalf("load POKEPILOT_MART4F_STONE_TEST_STATE=%s: %v", path, err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	start := state.DecodeInventory(&before)

	travel, err := BuyEvolutionStone(m, romData, waterStoneItem, StatAwareMove(romData))
	if err != nil {
		t.Fatalf("BuyEvolutionStone(water stone): %v (travel=%+v)", err, travel)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	end := state.DecodeInventory(&after)
	player := state.DecodePlayer(&after)

	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after the purchase: the objective leaked a menu")
	}
	if player.MapID != celadonMart4FMap {
		t.Fatalf("ended on map %#04x, want Celadon Mart 4F (%#04x)", player.MapID, celadonMart4FMap)
	}
	// The clerk is behind a counter, so success must come from the counter
	// approach. If a future change walks Red next to the clerk, this is no
	// longer exercising the counter rule under test.
	if _, adjacent := directionTo(player.X, player.Y, celadonMart4FClerkX, celadonMart4FClerkY); adjacent {
		t.Fatalf("expected the service-counter approach, but Red ended orthogonally adjacent to the clerk at (%d,%d)", player.X, player.Y)
	}
	if got, want := quantityOf(end.Items, waterStoneItem), quantityOf(start.Items, waterStoneItem)+1; got != want {
		t.Fatalf("water stone quantity = %d, want %d (bag %v)", got, want, end.Items)
	}
	if end.Money >= start.Money {
		t.Fatalf("money %d -> %d: a purchase must debit the wallet", start.Money, end.Money)
	}
}

func quantityOf(items []state.BagItem, id uint8) int {
	total := 0
	for _, item := range items {
		if item.ID == id {
			total += int(item.Quantity)
		}
	}
	return total
}
