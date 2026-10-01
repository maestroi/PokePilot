package profiles

import (
	"testing"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/game"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// TestTMHMPartyMenuMarker pins the per-game wording of the TM/HM party-select
// prompt. The shared Gen-I engine matches this profile fact, so a regression
// that re-hardcodes one cartridge's phrase in the generic layer would fail here
// for the other cartridges. The expected strings come from the vendored decomp:
//
//	pokered/data/text/text_2.asm  _PartyMenuUseTMText  "Use TM on which #MON?"
//	pokeyellow/data/text/text_3.asm _PartyMenuUseTMText "Teach to which #MON?"
//
// Blue shares Red's text (no pokeblue decomp), so it reports the same marker.
func TestTMHMPartyMenuMarker(t *testing.T) {
	// Compile-time contract: every Gen-I profile exposes the TM/HM menu wording.
	var _ game.TMHMMenuProfile = redprofile.New()
	var _ game.TMHMMenuProfile = blueprofile.New()
	var _ game.TMHMMenuProfile = yellowprofile.New()

	cases := []struct {
		name   string
		marker string
	}{
		{"red", redprofile.New().TMHMPartyMenuMarker()},
		{"blue", blueprofile.New().TMHMPartyMenuMarker()},
		{"yellow", yellowprofile.New().TMHMPartyMenuMarker()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.marker == "" {
				t.Fatalf("%s TM/HM party menu marker is empty", tc.name)
			}
		})
	}

	// Red and Blue share the party-select wording; Yellow renamed it.
	if redprofile.New().TMHMPartyMenuMarker() != "Use TM" {
		t.Errorf("red marker = %q, want %q", redprofile.New().TMHMPartyMenuMarker(), "Use TM")
	}
	if blueprofile.New().TMHMPartyMenuMarker() != "Use TM" {
		t.Errorf("blue marker = %q, want %q", blueprofile.New().TMHMPartyMenuMarker(), "Use TM")
	}
	if yellowprofile.New().TMHMPartyMenuMarker() != "Teach to which" {
		t.Errorf("yellow marker = %q, want %q", yellowprofile.New().TMHMPartyMenuMarker(), "Teach to which")
	}

	// The whole point of the split: Red/Blue and Yellow must NOT share a marker,
	// otherwise the generic layer would be matching one cartridge's phrase.
	if redprofile.New().TMHMPartyMenuMarker() == yellowprofile.New().TMHMPartyMenuMarker() {
		t.Errorf("red and yellow markers are both %q; the per-game wording fact is lost", redprofile.New().TMHMPartyMenuMarker())
	}
}
