package skill

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Farm #1970: bag maintenance beside the Secret Key walked toward a Mart to
// sell a Nugget, could not return, and the swallowed error let Pickup face the
// key from Route 19. Local capacity must be tried before a detour, and a
// stranded detour must never be treated as an optional failure.
func TestBagMaintenanceTriesLocalCandyBeforeDetourAndSurfacesStranding(t *testing.T) {
	srcBytes, err := os.ReadFile("bag_pressure.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "func ensureBagFreeSlotsManaged")
	if start < 0 {
		t.Fatal("ensureBagFreeSlotsManaged not found")
	}
	body := src[start:]
	candy := strings.Index(body, "useRareCandyForBagSpace(")
	sell := strings.Index(body, "sellNuggetsForBagSpace(")
	if candy < 0 || sell < 0 || candy > sell {
		t.Fatal("Rare Candy (local) must be tried before the Nugget Mart detour")
	}
	if !strings.Contains(body, "sellNuggetsForBagSpace(m, romData, policy); errors.Is(err, ErrInventoryDetourStranded)") {
		t.Fatal("a stranded Nugget detour must be returned, not swallowed")
	}

	joined := errors.Join(errors.New("sell failed"), fmt.Errorf("%w: map 0xd8", ErrInventoryDetourStranded))
	if !errors.Is(joined, ErrInventoryDetourStranded) {
		t.Fatal("runInventoryDetour's joined error must still identify stranding")
	}
}

// Farm #2873: Safari Zone blocks the Player PC detour, every bag TM is
// incompatible with the party, and the consumable toss whitelist is empty.
// An unteachable TM must still be discarded in place before teaching or the
// whitelist reports ErrNoSafeBagSpace.
func TestBagMaintenanceTossesUnteachableTMBeforeTeachingOrWhitelist(t *testing.T) {
	srcBytes, err := os.ReadFile("bag_pressure.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "func ensureBagFreeSlotsManaged")
	if start < 0 {
		t.Fatal("ensureBagFreeSlotsManaged not found")
	}
	body := src[start:]
	storeTM := strings.Index(body, "storeTMForBagSpace(")
	tossUnteachable := strings.Index(body, "tossUnteachableTMForBagSpace(")
	teach := strings.Index(body, "teachTMForBagSpace(")
	whitelist := strings.Index(body, "EnsureBagFreeSlots(")
	if storeTM < 0 || tossUnteachable < 0 || teach < 0 || whitelist < 0 {
		t.Fatal("bag-pressure ladder missing store-TM, toss-unteachable, teach, or whitelist step")
	}
	if !(storeTM < tossUnteachable && tossUnteachable < teach && teach < whitelist) {
		t.Fatalf("order store-TM=%d toss-unteachable=%d teach=%d whitelist=%d; want store < toss-unteachable < teach < whitelist",
			storeTM, tossUnteachable, teach, whitelist)
	}
}
