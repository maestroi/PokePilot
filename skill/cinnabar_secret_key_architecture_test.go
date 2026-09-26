package skill

import (
	"os"
	"strings"
	"testing"
)

// Farm #1970 reopened on the exact revision that added Route 19 -> Fuchsia
// restaging. Keep that recovery mechanically different from ordinary TravelFlee:
// the story corridor must not start an optional Fly/Dig/Rope controller before
// it has escaped the southern-sea route.
func TestSecretKeySouthernSeaRecoveryBypassesOptionalFastTravel(t *testing.T) {
	srcBytes, err := os.ReadFile("cinnabar_secret_key.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	start := strings.Index(src, "func travelSecretKeyCorridor")
	end := strings.Index(src, "func AcquireCinnabarSecretKey")
	if start < 0 || end <= start {
		t.Fatal("Secret Key corridor/recovery functions not found")
	}
	recovery := src[start:end]

	if !strings.Contains(recovery, "cutAwareGoTo(m, romData, dest),") {
		t.Fatal("Secret Key corridor no longer builds direct GoTo without fast-travel policy")
	}
	if strings.Contains(recovery, "useFlyTo(") {
		t.Fatal("southern-sea recovery must not directly attempt optional Fly")
	}
	if strings.Contains(recovery, "if _, err := TravelFlee(") {
		t.Fatal("southern-sea recovery drifted back to fast-travel-enabled TravelFlee")
	}
	if !strings.Contains(recovery, ""vermilion city", "viridian city", "pallet town"") {
		t.Fatal("mainland restage must deterministically return all the way to Pallet")
	}
}
