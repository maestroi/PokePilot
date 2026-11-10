package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// The Magnet Train is an available region-return option only after the
// Power Plant is restored AND Copycat gives the player the retail PASS. The
// latter is not available from the first interaction: the player must hear
// about her lost doll, collect LOST_ITEM in the Vermilion Fan Club, and return
// it. Each step here is a distinct cartridge-owned transaction.
//
// CopycatsHouse2F.asm sets EVENT_MET_COPYCAT_FOUND_OUT_ABOUT_LOST_ITEM after
// its mimicry scene (only if the Power Plant was restored first).
func executeGSKantoMeetCopycat(m *emu.Emu, romData []byte) error {
	const label = "kanto:copycat-learn-about-doll"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoCopycatMet, gsprofile.ProgressKantoPowerRestored, label)
	if err != nil || done {
		return err
	}
	return gsKantoStoryTalk(m, romData, profile,
		gsprofile.ProgressKantoCopycatMet, "COPYCATS_HOUSE_2F", 4, 4, 4, 3, label)
}

// PokemonFanClub.asm gives LOST_ITEM to the player only after Copycat has
// asked for it. EVENT_GOT_LOST_ITEM_FROM_FAN_CLUB is set after successful
// inventory receipt; the Doll itself is consumed on the return visit.
func executeGSKantoReceiveLostDoll(m *emu.Emu, romData []byte) error {
	const label = "kanto:vermilion-fan-club-lost-doll"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoLostDollReceived, gsprofile.ProgressKantoCopycatMet, label)
	if err != nil || done {
		return err
	}
	// The Fairy Doll object occupies (2,4) and the Fan Club man stands
	// at (2,3). Approach him from (3,3), not through the Doll sprite.
	return gsKantoStoryTalk(m, romData, profile,
		gsprofile.ProgressKantoLostDollReceived, "POKEMON_FAN_CLUB", 3, 3, 2, 3, label)
}

// CopycatsHouse2F.asm first takes LOST_ITEM, then awards PASS. An already
// returned Doll with a full key-item pocket is recoverable: the retail
// EVENT_RETURNED_LOST_ITEM_TO_COPYCAT retains the gift on re-interaction.
// Success is not inferred from this handoff or a text box: the profile
// requires BOTH EVENT_GOT_PASS_FROM_COPYCAT and PASS in the key-item pocket.
func executeGSKantoReceiveRailPass(m *emu.Emu, romData []byte) error {
	const label = "kanto:copycat-magnet-train-pass"
	profile, done, err := gsCoastalProfile(m, romData,
		gsprofile.ProgressKantoRailPassReceived, gsprofile.ProgressKantoLostDollReceived, label)
	if err != nil || done {
		return err
	}
	if err := gsKantoStoryTalk(m, romData, profile,
		gsprofile.ProgressKantoRailPassReceived, "COPYCATS_HOUSE_2F", 4, 4, 4, 3, label); err != nil {
		return fmt.Errorf("gen2 Magnet Train Pass: Copycat's retail receipt: %w", err)
	}
	return nil
}
