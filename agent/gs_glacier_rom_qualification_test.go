package agent

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
)

type gsGlacierStage struct {
	name string
	progress game.ProgressID
}

// Every actual cartridge story transaction is separately checked and restored;
// the final badge goal is run ONLY by this private barrier while it is still
// withheld from the production objective catalog.
func gsGlacierStages() []gsGlacierStage {
	return []gsGlacierStage{
		{"red-gyarados", gsprofile.ProgressRedGyaradosDefeated},
		{"lance", gsprofile.ProgressLanceHelpAccepted},
		{"mart-stairs", gsprofile.ProgressMahoganyStairsOpened},
		{"rocket-lance-heal", gsprofile.ProgressRocketLanceHealing},
		{"passwords", gsprofile.ProgressRocketPasswordsLearned},
		{"office", gsprofile.ProgressRocketOfficeOpened},
		{"rocket-executive-m", gsprofile.ProgressRocketExecutiveBeaten},
		{"murkrow-password", gsprofile.ProgressRocketMurkrowPassword},
		{"transmitter-door", gsprofile.ProgressRocketTransmitterDoorOpened},
		{"rocket-executive-f", gsprofile.ProgressRocketExecutiveFBeaten},
		{"rocket-hideout-clear", gsprofile.ProgressRocketHideoutCleared},
		{"glacier-badge", gsprofile.ProgressGlacierBadgeEarned},
	}
}

func TestGSGlacierQualificationStagesKeepBadgeLocked(t *testing.T) {
	stages := gsGlacierStages()
	if len(stages) != 12 || stages[len(stages)-1].progress != gsprofile.ProgressGlacierBadgeEarned {
		t.Fatalf("Glacier qualification does not cover all retail milestones: %+v", stages)
	}
	seen := map[game.ProgressID]bool{}
	for _, stage := range stages {
		if stage.name == "" || seen[stage.progress] {
			t.Fatalf("duplicate/empty qualification stage: %+v", stage)
		}
		seen[stage.progress] = true
		if stage.progress != gsprofile.ProgressGlacierBadgeEarned && !gsSupportedProgress(stage.progress) {
			t.Fatalf("unexecutable interim stage %s", stage.progress)
		}
	}
	if gsSupportedProgress(gsprofile.ProgressGlacierBadgeEarned) {
		t.Fatal("badge 7 cannot be offered until Gold AND Silver real-ROM qualification")
	}
}

// TestGSGlacierROMCheckpointReplay never runs in public PR CI. The ROM-equipped
// runner must supply an authentic post-Mineral state captured from a farm run,
// including source.json, to prove the complete six-to-seven-badge journey.
func TestGSGlacierROMCheckpointReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("private retail Gold/Silver cartridge qualification")
	}
	if os.Getenv("POKEPILOT_GS_GLACIER_QUAL") != "1" {
		t.Skip("opt-in Gen2 Glacier checkpoint replay")
	}
	variant := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_GS_GLACIER_GAME")))
	romPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_GLACIER_ROM"))
	checkpointPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_GLACIER_START"))
	var gameID game.GameID
	var sha string
	switch variant {
	case "gold":
		gameID, sha = gsprofile.GoldGameID, sym.GoldSHA1
	case "silver":
		gameID, sha = gsprofile.SilverGameID, sym.SilverSHA1
	default:
		t.Fatalf("expected gold or silver game, got %q", variant)
	}
	if romPath == "" || checkpointPath == "" {
		t.Fatal("missing private ROM or verified post-Mineral checkpoint; never green-skip")
	}
	romBytes, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	sum := sha1.Sum(romBytes)
	if len(romBytes) != sym.ROMSize || hex.EncodeToString(sum[:]) != sha {
		t.Fatalf("unexpected cartridge size or SHA-1 for %s", variant)
	}
	rawState, err := os.ReadFile(checkpointPath)
	if err != nil || len(rawState) == 0 {
		t.Fatalf("read private checkpoint: %v (empty=%t)", err, len(rawState) == 0)
	}
	sourcePath := filepath.Join(filepath.Dir(checkpointPath), "source.json")
	rawSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("verified farm provenance is required at %s: %v", sourcePath, err)
	}
	provenance, err := gsVerifyQualificationProvenanceForObjective(rawSource, variant, sha, rawState,
		string(gsprofile.ProgressMineralBadgeEarned))
	if err != nil {
		t.Fatalf("untrusted sixth-badge checkpoint: %v", err)
	}
	out := strings.TrimSpace(os.Getenv("POKEPILOT_GS_GLACIER_OUT"))
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Logf("Gold/Silver Glacier replay game=%s run=%s frame=%d private_output=%s",
		variant, provenance.RunID, provenance.Frame, out)
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer func() { if m != nil { _ = m.Close() } }()
	if err := m.LoadState(rawState); err != nil {
		t.Fatalf("load sixth-badge checkpoint: %v", err)
	}
	initial, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	if initial.GameID != gameID || !initial.Controllable || initial.InBattle ||
		!initial.Story.Has(gsprofile.ProgressMineralBadgeEarned) {
		t.Fatal("sixth-badge checkpoint does not provide a stable Mineral Badge boundary")
	}
	for _, badge := range gsSixBadgeFacts {
		if !initial.Story.Has(badge) {
			t.Fatalf("sixth-badge fixture is missing %s", badge)
		}
	}
	for _, stage := range gsGlacierStages() {
		if initial.Story.Has(stage.progress) {
			t.Fatalf("not an unstarted sixth-to-seventh badge replay: %s is already complete", stage.progress)
		}
	}
	gym, err := gsOpeningMapID("OLIVINE_GYM")
	if err != nil {
		t.Fatal(err)
	}
	if gsprofile.New(gameID).DecodeOverworld(m).NativeMapID != gym {
		t.Fatal("sixth-badge checkpoint must be captured in Olivine Gym, not near Mahogany")
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "initial.json"), initial); err != nil {
		t.Fatal(err)
	}
	startDigest := sha256.Sum256(rawState)
	t.Logf("checked start state sha256=%x", startDigest)
	defer func() {
		if !t.Failed() { return }
		if state, e := m.SaveStateChecked(); e == nil {
			_ = os.WriteFile(filepath.Join(out, "failure.state"), state, 0o600)
		}
		if obs, e := ObserveChecked(m, m.ROM()); e == nil {
			_ = gsCoastalWriteJSON(filepath.Join(out, "failure.json"), obs)
		}
	}()
	var completed []game.ProgressID
	for i, stage := range gsGlacierStages() {
		stage := stage
		ok := t.Run(stage.name, func(t *testing.T) {
			adapter := newGSObjectiveAdapter(m, m.ROM(), gameID)
			o := Objective{Kind: KindProgress, Progress: stage.progress}
			if stage.progress != gsprofile.ProgressGlacierBadgeEarned {
				if err := adapter.Validate(o, Observation{}); err != nil { t.Fatal(err) }
			} else if gsSupportedProgress(stage.progress) {
				t.Fatal("Glacier unexpectedly enabled before verified full qualification")
			}
			if err := adapter.NormalizeBoundary(); err != nil { t.Fatal(err) }
			before, err := adapter.Observe()
			if err != nil { t.Fatal(err) }
			if before.GameID != gameID || !before.Controllable || before.InBattle ||
				before.Story.Has(stage.progress) {
				t.Fatalf("unstable or pre-completed stage %s", stage.name)
			}
			for _, prior := range completed {
				if !before.Story.Has(prior) {
					t.Fatalf("lost earlier completion %s before %s", prior, stage.name)
				}
			}
			startFrame := m.FrameCount()
			var result ObjectiveResult
			err = adapter.WithinObjectiveBudget(o, func() error {
				if stage.progress == gsprofile.ProgressGlacierBadgeEarned {
					// The retail leader controller exists, but production must
					// stay fail-closed until replay evidence qualifies it.
					if err := executeGSPryce(m, m.ROM()); err != nil { return err }
					result = ObjectiveResult{Objective: o}
					return nil
				}
				var e error
				result, e = adapter.ExecuteOwned(o)
				return e
			})
			if err != nil {
				t.Fatalf("stage %s failed after %d frames: %v", stage.name, m.FrameCount()-startFrame, err)
			}
			if err := adapter.NormalizeBoundary(); err != nil { t.Fatal(err) }
			after, err := adapter.Observe()
			if err != nil { t.Fatal(err) }
			if stage.progress == gsprofile.ProgressGlacierBadgeEarned {
				_, err = verifyObjectivePostcondition(o, before, after, result)
			} else {
				err = adapter.VerifyPostcondition(o, before, after, result)
			}
			if err != nil { t.Fatalf("%s did not satisfy semantic postcondition: %v", stage.name, err) }
			if !after.Story.Has(stage.progress) || !after.Controllable || after.InBattle {
				t.Fatalf("stage %s did not settle into committed cartridge state", stage.name)
			}
			state, err := m.SaveStateChecked()
			if err != nil { t.Fatal(err) }
			file := fmt.Sprintf("%02d-%s", i+1, stage.name)
			if err := os.WriteFile(filepath.Join(out, file+".state"), state, 0o600); err != nil { t.Fatal(err) }
			if err := gsCoastalWriteJSON(filepath.Join(out, file+".json"), after); err != nil { t.Fatal(err) }
			hash := sha256.Sum256(state)
			t.Logf("PASS %s frames=%d checkpoint_sha256=%x", stage.name, m.FrameCount()-startFrame, hash)
			if err := m.Close(); err != nil { t.Fatal(err) }
			m, err = emu.Open(romPath)
			if err != nil { t.Fatal(err) }
			if err := m.LoadState(state); err != nil { t.Fatalf("reload saved stage %s: %v", stage.name, err) }
			restarted, err := ObserveChecked(m, m.ROM())
			if err != nil { t.Fatal(err) }
			if !restarted.Controllable || restarted.InBattle || restarted.GameID != gameID ||
				!restarted.Story.Has(stage.progress) {
				t.Fatalf("lost stage %s on emulator reopen", stage.name)
			}
			for _, prior := range completed {
				if !restarted.Story.Has(prior) { t.Fatalf("lost %s on stage restart", prior) }
			}
		})
		if !ok { return }
		completed = append(completed, stage.progress)
	}
	final, err := ObserveChecked(m, m.ROM())
	if err != nil { t.Fatal(err) }
	if !final.Story.Has(gsprofile.ProgressGlacierBadgeEarned) ||
		!final.Story.Has(gsprofile.ProgressRocketHideoutCleared) ||
		final.Story.Has(gsprofile.ProgressRisingBadgeEarned) {
		t.Fatal("badge-7 replay did not earn the retail Glacier Badge or skipped directly to Rising")
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "final.json"), final); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS complete six-to-seven-badge retail %s replay", variant)
}
