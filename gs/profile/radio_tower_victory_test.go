package profile

import "testing"

func TestGSRadioTowerFinaleRetailEventsRemainIndependent(t *testing.T) {
	if eventRadioTowerExecutiveM2Beaten != 1394 || eventRadioTowerExecutiveF1Beaten != 1391 ||
		eventRadioTowerBossBeaten != 1393 || eventRadioTowerCleared != 33 ||
		eventRadioTowerGoldWingReceived != 120 || eventRadioTowerSilverWingReceived != 121 {
		t.Fatal("Radio Tower final story event indices drifted from pret/pokegold retail")
	}
	for _, tc := range []struct {
		name            string
		profile         *Profile
		wing, wrongWing uint16
	}{
		{"gold", NewGold(), eventRadioTowerGoldWingReceived, eventRadioTowerSilverWingReceived},
		{"silver", NewSilver(), eventRadioTowerSilverWingReceived, eventRadioTowerGoldWingReceived},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := fakeGSReader{}
			if tc.profile.DecodeRadioTowerVictory(mem).Complete() {
				t.Fatal("fresh save already cleared the tower")
			}
			setGSEvent(mem, eventRadioTowerExecutiveM2Beaten)
			setGSEvent(mem, eventRadioTowerExecutiveF1Beaten)
			if v := tc.profile.DecodeRadioTowerVictory(mem); v.Complete() || v.BossBeaten {
				t.Fatalf("executive victories forged final boss: %+v", v)
			}
			setGSEvent(mem, eventRadioTowerBossBeaten)
			setGSEvent(mem, eventRadioTowerCleared)
			if v := tc.profile.DecodeRadioTowerVictory(mem); !v.Cleared || !v.BossBeaten || v.Complete() {
				t.Fatalf("pre-Director-clear falsely counted as settled: %+v", v)
			}
			if s := tc.profile.DecodeFirstBadgeProgress(mem); !s.Has(ProgressRadioTowerCleared) ||
				s.Has(ProgressRadioTowerTakeoverSettled) || s.Has(ProgressRisingBadgeEarned) {
				t.Fatal("cleared flag must not complete gift or eighth badge")
			}
			setGSEvent(mem, tc.wrongWing)
			if tc.profile.DecodeRadioTowerVictory(mem).Complete() {
				t.Fatal("other game's wing was accepted as Director's gift")
			}
			setGSEvent(mem, tc.wing)
			if !tc.profile.DecodeRadioTowerVictory(mem).Complete() ||
				!tc.profile.DecodeFirstBadgeProgress(mem).Has(ProgressRadioTowerTakeoverSettled) {
				t.Fatal("complete boss, clear and correct wing events were not projected")
			}
			if tc.profile.DecodeFirstBadgeProgress(mem).Has(ProgressRisingBadgeEarned) {
				t.Fatal("Radio Tower success cannot mint Clair's Rising Badge")
			}
		})
	}
}

func TestGSRadioTowerMissingExecutiveCannotProveTakeover(t *testing.T) {
	p := NewGold()
	mem := fakeGSReader{}
	setGSEvent(mem, eventRadioTowerBossBeaten)
	setGSEvent(mem, eventRadioTowerCleared)
	setGSEvent(mem, eventRadioTowerGoldWingReceived)
	if p.DecodeRadioTowerVictory(mem).Complete() {
		t.Fatal("final boss and reward do not imply previous executive fights")
	}
}
