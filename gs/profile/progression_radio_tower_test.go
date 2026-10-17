package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestGSRadioTowerRetailEventFactsAreIndependent(t *testing.T) {
	if eventBeatRadioTowerFakeDirector != 1395 ||
		eventRadioTowerBasementUnlocked != 73 ||
		eventRadioTowerCardKeyReceived != 74 ||
		eventRadioTowerShutterOpened != 37 ||
		eventRadioTowerCleared != 33 {
		t.Fatal("Radio Tower story events drifted from pret/pokegold retail indices")
	}
	for _, profile := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		steps := []struct {
			event    uint16
			progress game.ProgressID
		}{
			{eventBeatRadioTowerFakeDirector, ProgressRadioTowerFakeDirectorBeaten},
			{eventRadioTowerBasementUnlocked, ProgressRadioTowerBasementUnlocked},
			{eventRadioTowerCardKeyReceived, ProgressRadioTowerCardKeyReceived},
			{eventRadioTowerShutterOpened, ProgressRadioTowerShutterOpened},
			{eventRadioTowerExecutiveM2Beaten, ProgressRadioTowerExecutiveM2Beaten},
			{eventRadioTowerExecutiveF1Beaten, ProgressRadioTowerExecutiveF1Beaten},
			{eventRadioTowerCleared, ProgressRadioTowerCleared},
		}
		for i, stage := range steps {
			setGSEvent(mem, stage.event)
			story := profile.DecodeFirstBadgeProgress(mem)
			if !story.Has(steps[i].progress) {
				t.Fatalf("Radio Tower event %d did not project %s", stage.event, stage.progress)
			}
			for _, later := range steps[i+1:] {
				if story.Has(later.progress) {
					t.Fatalf("%s incorrectly completes future %s", stage.progress, later.progress)
				}
			}
			if story.Has(ProgressRisingBadgeEarned) || story.Has(ProgressGlacierBadgeEarned) {
				t.Fatal("Radio Tower events cannot forge a Johto badge")
			}
		}
	}
}
