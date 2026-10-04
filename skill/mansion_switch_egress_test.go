package skill

import (
	"os"
	"strings"
	"testing"
)

// TestTravelRecoversMansionSwitchSeal pins the recovery hook for farm triage
// d3a8d73bd07ae38d: EnsureItemStock/TravelFlee must open live Mansion statue
// seals rather than die on world: no_route while the static graph still offers
// the outdoor warps. Plain GoTo stays unaware so returnToCinnabarIsland can
// still observe ErrNoRoute and own its story handoff.
func TestTravelRecoversMansionSwitchSeal(t *testing.T) {
	src, err := os.ReadFile("travel.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "recoverGoToCompatibilitySwitchSeal(") {
		t.Fatal("Travel recoveringGoTo no longer recovers Gen-I Mansion switch-sealed pockets")
	}
	gotoSrc, err := os.ReadFile("goto.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gotoSrc), "recoverGoToCompatibilitySwitchSeal(") {
		t.Fatal("plain GoTo must not own Mansion switch-seal recovery")
	}
	compat, err := os.ReadFile("goto_gen1_compat.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"openMansionBasementExit(",
		"openMansion1FExit(",
		"pokemonMansionB1FMap",
		"pokemonMansion1FMap",
	} {
		if !strings.Contains(string(compat), want) {
			t.Fatalf("switch-seal recovery missing %q", want)
		}
	}
	keySrc, err := os.ReadFile("cinnabar_secret_key.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keySrc), "openMansionBasementExit(m, romData, policy)") {
		t.Fatal("AcquireCinnabarSecretKey no longer opens the B1F stairs after collecting the key")
	}
}
