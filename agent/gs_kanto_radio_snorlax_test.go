package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSKantoPokeFluteRadioActionsAreBoundedAndDoNotAnswerChoices(t *testing.T) {
	for _, tc := range []struct {
		name       string
		page, knob byte
		want       emu.Button
		press      bool
		err        bool
	}{
		{"clock", 1, 0, emu.Right, true, false},
		{"map-init", 3, 0, 0, false, false},
		{"map-ready", 6, 0, emu.Right, true, false},
		{"phone-ready", 8, 0, emu.Right, true, false},
		{"radio-init", 11, 0, 0, false, false},
		{"tune-up", 12, 76, emu.Up, true, false},
		{"tune-down", 12, 80, emu.Down, true, false},
		{"tuned", 12, 78, 0, false, false},
		{"unsafe-phone-call", 9, 0, 0, false, true},
		{"unsafe-menu-exit", 0x8c, 78, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			btn, press, err := gsKantoRadioDirection(gsprofile.PokegearRadioState{
				Page: tc.page, Tuning: tc.knob, HasRadio: true, HasExpansion: true,
			})
			if (err != nil) != tc.err || btn != tc.want || press != tc.press {
				t.Fatalf("radio decision = %d,%v,%v; want %d,%v,err=%v",
					btn, press, err, tc.want, tc.press, tc.err)
			}
		})
	}
	_, _, err := gsKantoRadioDirection(gsprofile.PokegearRadioState{
		Page: gsKantoRadioPage, Tuning: gsKantoPokeFluteKnob, HasRadio: true,
	})
	if err == nil {
		t.Fatal("radio cannot be tuned in Kanto without expansion card")
	}
}

func TestGSKantoSnorlaxAndExpansionUseRealMapsAndExecutableStages(t *testing.T) {
	if gsKantoSnorlaxStandX != 34 || gsKantoSnorlaxStandY != 10 ||
		gsKantoSnorlaxX != 34 || gsKantoSnorlaxY != 8 {
		t.Fatal("Snorlax proximity does not match VermilionCity.asm / SnorlaxAwake special")
	}
	for _, name := range []string{"LAV_RADIO_TOWER_1F", "LAVENDER_TOWN", "VERMILION_CITY", "DIGLETTS_CAVE"} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("unowned radio/Snorlax map %s: %#04x err=%v", name, id, err)
		}
	}
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		for _, stage := range []game.ProgressID{gsprofile.ProgressKantoExpansionCard, gsprofile.ProgressKantoSnorlaxCleared} {
			o := Objective{Kind: KindProgress, Progress: stage}
			if !gsSupportedProgress(stage) || !adapter.SupportsObjective(o) {
				t.Fatalf("%s: missing controller for %s", id, stage)
			}
			if b := gsObjectiveFrameBudget(o); b != gsFirstBadgeObjectiveFrameBudget {
				t.Fatalf("%s: %s watchdog budget %d", id, stage, b)
			}
		}
	}
	if err := executeGSKantoExpansionCard(nil, nil); err == nil {
		t.Fatal("EXPn card objective must not proceed without ROM")
	}
	if err := executeGSKantoSnorlax(nil, nil); err == nil {
		t.Fatal("Snorlax objective must not proceed without ROM")
	}
}

func TestGSKantoBoulderOfferChecksBothRadioCardsAndSnorlaxFirst(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	obs := Observation{GameID: gsprofile.GoldGameID, Story: ProgressState{
		{ID: gsprofile.ProgressKantoThunderBadgeEarned, Complete: true},
		{ID: gsprofile.ProgressKantoMarshBadgeEarned, Complete: true},
		{ID: gsprofile.ProgressKantoRainbowBadgeEarned, Complete: true},
		{ID: gsprofile.ProgressKantoSoulBadgeEarned, Complete: true},
		{ID: gsprofile.ProgressKantoCascadeBadgeEarned, Complete: true},
	}}
	for _, want := range []game.ProgressID{
		gsprofile.ProgressRadioCardReceived,
		gsprofile.ProgressKantoExpansionCard,
		gsprofile.ProgressKantoSnorlaxCleared,
		gsprofile.ProgressKantoBoulderBadgeEarned,
	} {
		offered := adapter.ProgressionObjectives(obs)
		if len(offered) != 1 || offered[0].Progress != want {
			t.Fatalf("Brock approach offered %+v, want %s", offered, want)
		}
		if !adapter.SupportsObjective(offered[0]) {
			t.Fatalf("Brock approach offered unowned controller %s", offered[0].Progress)
		}
		obs.Story = append(obs.Story, ProgressFact{ID: want, Complete: true})
	}
}
