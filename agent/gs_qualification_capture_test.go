package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func gsCureProgressObservation(id game.GameID, cure bool) Observation {
	story := ProgressState{
		{ID: gsprofile.ProgressSecretPotionAcquired, Complete: true},
		{ID: gsprofile.ProgressJasmineReturnedToGym, Complete: cure},
	}
	return Observation{
		GameID:       id,
		Controllable: true,
		Story:        story,
	}
}

func TestGSQualificationCaptureOnlyAtFreshVerifiedCureBoundary(t *testing.T) {
	obj := Objective{Kind: KindProgress, Progress: gsprofile.ProgressJasmineReturnedToGym}
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		before := gsCureProgressObservation(id, false)
		after := gsCureProgressObservation(id, true)
		if !gsPostCureCaptureEligible(obj, before, after) {
			t.Fatalf("%s first verified cure rejected", id)
		}
		scenarios := []struct {
			name   string
			before Observation
			after  Observation
			obj    Objective
		}{
			{"already cured", after, after, obj},
			{"cure flag absent", before, before, obj},
			{"wrong objective", before, after, Objective{Kind: KindProgress, Progress: gsprofile.ProgressStormBadgeEarned}},
			{"Strength already collected", before, withCureFact(after, gsprofile.ProgressHM04StrengthAcquired), obj},
			{"Storm already earned", before, withCureFact(after, gsprofile.ProgressStormBadgeEarned), obj},
			{"Mineral already earned", before, withCureFact(after, gsprofile.ProgressMineralBadgeEarned), obj},
			{"unsettled battle", before, func() Observation { v := after; v.InBattle = true; return v }(), obj},
			{"wrong game", before, func() Observation {
				v := after
				v.GameID = gsprofile.GoldGameID
				if id == gsprofile.GoldGameID {
					v.GameID = gsprofile.SilverGameID
				}
				return v
			}(), obj},
		}
		for _, tc := range scenarios {
			if gsPostCureCaptureEligible(tc.obj, tc.before, tc.after) {
				t.Errorf("%s: %s falsely eligible", id, tc.name)
			}
		}
	}
}

func withCureFact(obs Observation, id game.ProgressID) Observation {
	obs.Story = append(append(ProgressState{}, obs.Story...), ProgressFact{ID: id, Complete: true})
	return obs
}

func TestGSPublishQualificationStateOnceDoesNotOverwriteOrExposeTemp(t *testing.T) {
	target := filepath.Join(t.TempDir(), "gs-coastal", "gold", "post-cure", "start.state")
	meta := gsQualificationProvenance{Schema: 1, Game: "gold", Objective: "gs_jasmine_returned_to_gym",
		RunID: "run-a", StateSHA256: "sha-A", Frame: 123}
	fresh, err := gsPublishQualificationStateOnce(target, []byte("checked-A"), meta)
	if err != nil || !fresh {
		t.Fatalf("first publication = %v, %v", fresh, err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("qualification state perms = %v, %v", info, err)
	}
	if existing, err := os.ReadFile(target); err != nil || string(existing) != "checked-A" {
		t.Fatalf("checkpoint = %q, %v", existing, err)
	}
	fresh, err = gsPublishQualificationStateOnce(target, []byte("checked-B"), gsQualificationProvenance{RunID: "run-b"})
	if err != nil || fresh {
		t.Fatalf("second publication = %v, %v; must retain first", fresh, err)
	}
	existing, err := os.ReadFile(target)
	if err != nil || string(existing) != "checked-A" {
		t.Fatalf("immutable first checkpoint was replaced: %q, %v", existing, err)
	}
	metadata, err := os.ReadFile(filepath.Join(filepath.Dir(target), "source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got gsQualificationProvenance
	if err := json.Unmarshal(metadata, &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-a" || got.Frame != 123 {
		t.Fatalf("provenance was replaced by later run: %+v", got)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected temporary state files remain: %+v", entries)
	}
}

func TestGSPublishQualificationStateRejectsEmptyAndMissingCorpus(t *testing.T) {
	if fresh, err := gsPublishQualificationStateOnce(filepath.Join(t.TempDir(), "start.state"), nil, gsQualificationProvenance{}); err == nil || fresh {
		t.Fatalf("empty state published: fresh=%t err=%v", fresh, err)
	}
	t.Setenv(gsQualificationCorpusEnv, "")
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	before := gsCureProgressObservation(gsprofile.GoldGameID, false)
	after := gsCureProgressObservation(gsprofile.GoldGameID, true)
	path, err := adapter.CaptureQualificationCheckpoint(Objective{Kind: KindProgress, Progress: gsprofile.ProgressJasmineReturnedToGym}, before, after)
	if path != "" || err != nil {
		t.Fatalf("unset corpus should not capture: path=%q err=%v", path, err)
	}
}
