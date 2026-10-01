package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestPostSurgeLavenderReachedIncludesForwardCorridor(t *testing.T) {
	for _, mapID := range []uint8{
		0x04, // Lavender Town
		0x13, // Route 8
		0x79, // Underground Path west-east
		0x12, // Route 7
		0x0A, // Saffron City
		0x06, // Celadon City
		0x85, // Celadon Pokemon Center
		0x87, // Game Corner
		0xC7, // Rocket Hideout 1F
	} {
		if !postSurgeLavenderReached(mapID) {
			t.Errorf("map %#04x (%s) did not count as past the Lavender checkpoint", mapID, state.MapName(mapID))
		}
	}
	for _, mapID := range []uint8{0x05, 0x03, 0x14, 0x52} {
		if postSurgeLavenderReached(mapID) {
			t.Errorf("map %#04x (%s) incorrectly counted as past the Lavender checkpoint", mapID, state.MapName(mapID))
		}
	}
}

func TestPostSurgeCeladonAreaIncludesRelevantInteriors(t *testing.T) {
	for _, mapID := range []uint8{0x06, 0x7A, 0x85, 0x86, 0x87, 0x89, 0xC7, 0xCA} {
		if !postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x (%s) did not count as Celadon area", mapID, state.MapName(mapID))
		}
	}
	for _, mapID := range []uint8{0x04, 0x13, 0x0A} {
		if postSurgeCeladonArea(mapID) {
			t.Errorf("map %#04x (%s) incorrectly counted as Celadon area", mapID, state.MapName(mapID))
		}
	}
}

// Regression for #2304: Rainbow Badge must complete the bounded post-Surge
// stages even with an unrecovered party, matching PostSurgeReachCeladon /
// PostSurgeReachLavender early-return once Erika is already beaten.
func TestPostSurgeStagesSupersededByRainbowBadge(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = 0x06 // CELADON_CITY
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeRainbow)
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonHP+1] = 10
	mem[base+sym.MonMaxHP+1] = 20
	mem[base+sym.MonStatus] = 1 << 6 // paralyzed

	story := ProjectStory(&mem, state.StoryFacts{})
	if !story.Has(ProgressPostSurgeLavenderReached) {
		t.Fatal("Rainbow Badge did not supersede post_surge_lavender_reached")
	}
	if !story.Has(ProgressPostSurgeCeladonReady) {
		t.Fatal("Rainbow Badge with unrecovered Celadon party left post_surge_celadon_ready incomplete")
	}
	if !story.Has(ProgressRainbowBadge) {
		t.Fatal("Rainbow Badge bit was not projected")
	}
}

func TestPostSurgeCeladonReadyStillRequiresRecoveryBeforeRainbow(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = 0x06
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeThunder)
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonHP+1] = 10
	mem[base+sym.MonMaxHP+1] = 20
	mem[base+sym.MonMoves] = 1
	mem[base+sym.MonPP] = 10

	if ProjectStory(&mem, state.StoryFacts{}).Has(ProgressPostSurgeCeladonReady) {
		t.Fatal("unrecovered Celadon party counted as ready before Rainbow Badge")
	}
	mem[base+sym.MonHP+1] = 20
	if !ProjectStory(&mem, state.StoryFacts{}).Has(ProgressPostSurgeCeladonReady) {
		t.Fatal("fully recovered Celadon party did not project post_surge_celadon_ready")
	}
}

func TestPartyCenterRecoveredRequiresFullHPStatusAndPP(t *testing.T) {
	healthy := state.PartyState{
		Count: 1,
		Mons: []state.Mon{{
			HP:    30,
			MaxHP: 30,
			Moves: [4]uint8{1, 2},
			PP:    [4]uint8{10, 5},
		}},
	}
	if !partyCenterRecovered(healthy) {
		t.Fatal("fully recovered party was not recognized")
	}

	damaged := healthy
	damaged.Mons = append([]state.Mon(nil), healthy.Mons...)
	damaged.Mons[0].HP = 29
	if partyCenterRecovered(damaged) {
		t.Fatal("damaged party counted as Center-recovered")
	}

	statused := healthy
	statused.Mons = append([]state.Mon(nil), healthy.Mons...)
	statused.Mons[0].Status = 1
	if partyCenterRecovered(statused) {
		t.Fatal("statused party counted as Center-recovered")
	}

	exhausted := healthy
	exhausted.Mons = append([]state.Mon(nil), healthy.Mons...)
	exhausted.Mons[0].PP[1] = 0
	if partyCenterRecovered(exhausted) {
		t.Fatal("party with an exhausted known move counted as Center-recovered")
	}
}
