package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func gsMineralFixture(id game.GameID, mineral bool) Observation {
	facts := ProgressState{}
	for _, badge := range gsSixBadgeFacts[:len(gsSixBadgeFacts)-1] {
		facts = append(facts, ProgressFact{ID: badge, Complete: true})
	}
	facts = append(facts, ProgressFact{ID: gsprofile.ProgressMineralBadgeEarned, Complete: mineral})
	return Observation{GameID: id, Controllable: true, Story: facts}
}

func TestGSGlacierCaptureRequiresFreshSixthBadgeBoundary(t *testing.T) {
	o := Objective{Kind: KindProgress, Progress: gsprofile.ProgressMineralBadgeEarned}
	for _, gameID := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		before := gsMineralFixture(gameID, false)
		after := gsMineralFixture(gameID, true)
		if !gsPostMineralCaptureEligible(o, before, after) {
			t.Fatalf("%s verified six-badge boundary was not capturable", gameID)
		}
		check := func(name string, b, a Observation, obj Objective) {
			t.Helper()
			if gsPostMineralCaptureEligible(obj, b, a) {
				t.Errorf("%s: %s untrusted six-badge checkpoint accepted", gameID, name)
			}
		}
		check("already earned", after, after, o)
		check("missing badge", before, before, o)
		check("wrong objective", before, after, Objective{Kind: KindProgress, Progress: gsprofile.ProgressStormBadgeEarned})
		check("already fought Gyarados", before, withCureFact(after, gsprofile.ProgressRedGyaradosDefeated), o)
		check("Rocket already cleared", before, withCureFact(after, gsprofile.ProgressRocketHideoutCleared), o)
		check("Glacier already earned", before, withCureFact(after, gsprofile.ProgressGlacierBadgeEarned), o)
		inBattle := after
		inBattle.InBattle = true
		check("script not settled", before, inBattle, o)
		wrongGame := after
		if gameID == gsprofile.GoldGameID {
			wrongGame.GameID = gsprofile.SilverGameID
		} else {
			wrongGame.GameID = gsprofile.GoldGameID
		}
		check("wrong game", before, wrongGame, o)
		missingFirstBadge := before
		missingFirstBadge.Story = ProgressState{
			{ID: gsprofile.ProgressMineralBadgeEarned, Complete: false},
		}
		check("not six badges", missingFirstBadge, after, o)
	}
}

func TestGSGlacierProvenanceCannotReuseCoastalFixture(t *testing.T) {
	state := []byte("private-six-badge-checkpoint")
	hash := sha256.Sum256(state)
	source := gsQualificationProvenance{
		Schema: 1, Game: "gold", Objective: string(gsprofile.ProgressMineralBadgeEarned),
		ROM_SHA1: "gold-rom-sha1", StateSHA256: hex.EncodeToString(hash[:]),
	}
	verify := func(p gsQualificationProvenance, wantOK bool) {
		t.Helper()
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		_, err = gsVerifyQualificationProvenanceForObjective(raw, "gold", "gold-rom-sha1",
			state, string(gsprofile.ProgressMineralBadgeEarned))
		if (err == nil) != wantOK {
			t.Fatalf("source %+v validation = %v, wantOK=%t", p, err, wantOK)
		}
	}
	verify(source, true)
	source.Objective = string(gsprofile.ProgressJasmineReturnedToGym)
	verify(source, false)
	source.Objective = string(gsprofile.ProgressMineralBadgeEarned)
	source.StateSHA256 = "not-this-state"
	verify(source, false)
}
