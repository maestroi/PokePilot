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

// Replay the full Gold/Silver six-to-eight journey rather than certifying
// isolated event flags. Public short CI runs only TestGSRisingStageCoverage;
// only the private ROM runner can opt into the actual cartridge execution.
func gsRisingStages() []gsGlacierStage {
	stages := append([]gsGlacierStage(nil), gsGlacierStages()...)
	return append(stages,
		gsGlacierStage{"radio-fake-director", gsprofile.ProgressRadioTowerFakeDirectorBeaten},
		gsGlacierStage{"radio-basement-door", gsprofile.ProgressRadioTowerBasementUnlocked},
		gsGlacierStage{"radio-card-key", gsprofile.ProgressRadioTowerCardKeyReceived},
		gsGlacierStage{"radio-3f-shutter", gsprofile.ProgressRadioTowerShutterOpened},
		gsGlacierStage{"radio-4f-executive", gsprofile.ProgressRadioTowerExecutiveM2Beaten},
		gsGlacierStage{"radio-5f-executive", gsprofile.ProgressRadioTowerExecutiveF1Beaten},
		gsGlacierStage{"radio-boss-director", gsprofile.ProgressRadioTowerTakeoverSettled},
		gsGlacierStage{"ice-path-hm07", gsprofile.ProgressIcePathWaterfallAcquired},
		gsGlacierStage{"ice-path-boulders", gsprofile.ProgressIcePathBouldersDropped},
		gsGlacierStage{"blackthorn-boulders", gsprofile.ProgressBlackthornGymBouldersDropped},
		gsGlacierStage{"blackthorn-clair", gsprofile.ProgressClairDefeated},
		gsGlacierStage{"dragons-den-rising", gsprofile.ProgressRisingBadgeEarned},
	)
}

func TestGSRisingStageCoverage(t *testing.T) {
	stages := gsRisingStages()
	if len(stages) != 24 || stages[0].progress != gsprofile.ProgressRedGyaradosDefeated ||
		stages[len(stages)-1].progress != gsprofile.ProgressRisingBadgeEarned {
		t.Fatalf("badge-6-to-8 proof does not include every step: %+v", stages)
	}
	seen := map[game.ProgressID]bool{}
	for _, stage := range stages {
		if stage.name == "" || seen[stage.progress] || !gsSupportedProgress(stage.progress) {
			t.Fatalf("stage %s is empty, repeated or missing an executable controller", stage.name)
		}
		seen[stage.progress] = true
	}
}

// TestGSRisingROMCheckpointReplay requires a private, provenance-verified
// post-Mineral save. On every milestone it checks the actual observed event,
// persists a private checkpoint, closes/reopens the ROM and verifies ALL prior
// events remain true. Failure preserves a private checkpoint for fast repair.
func TestGSRisingROMCheckpointReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("private retail Gold/Silver checkpoint qualification")
	}
	if os.Getenv("POKEPILOT_GS_RISING_QUAL") != "1" {
		t.Skip("opt-in six-to-eight-badge ROM qualification")
	}
	variant := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_GS_RISING_GAME")))
	romPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_RISING_ROM"))
	statePath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_RISING_START"))
	var id game.GameID
	var expectedSHA string
	switch variant {
	case "gold":
		id, expectedSHA = gsprofile.GoldGameID, sym.GoldSHA1
	case "silver":
		id, expectedSHA = gsprofile.SilverGameID, sym.SilverSHA1
	default:
		t.Fatalf("expected Gold or Silver, not %q", variant)
	}
	if romPath == "" || statePath == "" {
		t.Fatal("retail ROM and verified six-badge checkpoint are both mandatory")
	}
	romBytes, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read private ROM: %v", err)
	}
	romHash := sha1.Sum(romBytes)
	if len(romBytes) != sym.ROMSize || hex.EncodeToString(romHash[:]) != expectedSHA {
		t.Fatal("ROM is not the supported Gold/Silver retail revision")
	}
	start, err := os.ReadFile(statePath)
	if err != nil || len(start) == 0 {
		t.Fatalf("read post-Mineral checkpoint: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "source.json"))
	if err != nil {
		t.Fatalf("private qualification provenance missing: %v", err)
	}
	prov, err := gsVerifyQualificationProvenanceForObjective(source, variant, expectedSHA, start,
		string(gsprofile.ProgressMineralBadgeEarned))
	if err != nil {
		t.Fatalf("cannot trust post-Mineral state: %v", err)
	}
	out := strings.TrimSpace(os.Getenv("POKEPILOT_GS_RISING_OUT"))
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Logf("qualifying game=%s run=%s frame=%d output=%s", variant, prov.RunID, prov.Frame, out)
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if m != nil {
			_ = m.Close()
		}
	}()
	if err := m.LoadState(start); err != nil {
		t.Fatalf("load six-badge checkpoint: %v", err)
	}
	startObs, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	if startObs.GameID != id || !startObs.Controllable || startObs.InBattle ||
		!startObs.Story.Has(gsprofile.ProgressMineralBadgeEarned) ||
		startObs.Story.Has(gsprofile.ProgressGlacierBadgeEarned) ||
		startObs.Story.Has(gsprofile.ProgressClairDefeated) ||
		startObs.Story.Has(gsprofile.ProgressRisingBadgeEarned) {
		t.Fatal("state is not a pre-Glacier, six-badge cartridge boundary")
	}
	for _, badge := range gsSixBadgeFacts {
		if !startObs.Story.Has(badge) {
			t.Fatalf("start checkpoint missing required sixth-badge story fact %s", badge)
		}
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "initial.json"), startObs); err != nil {
		t.Fatal(err)
	}
	// Keep the last reachable actual ROM state even if the next controller
	// errors, panics or a postcondition fails. No checkpoint goes to artifacts.
	defer func() {
		if !t.Failed() {
			return
		}
		if saved, e := m.SaveStateChecked(); e == nil {
			_ = os.WriteFile(filepath.Join(out, "failure.state"), saved, 0o600)
		}
		if obs, e := ObserveChecked(m, m.ROM()); e == nil {
			_ = gsCoastalWriteJSON(filepath.Join(out, "failure.json"), obs)
		}
	}()
	completed := make([]game.ProgressID, 0, len(gsRisingStages()))
	for i, stage := range gsRisingStages() {
		if !t.Run(stage.name, func(t *testing.T) {
			adapter := newGSObjectiveAdapter(m, m.ROM(), id)
			o := Objective{Kind: KindProgress, Progress: stage.progress}
			if err := adapter.Validate(o, Observation{}); err != nil {
				t.Fatal(err)
			}
			if err := adapter.NormalizeBoundary(); err != nil {
				t.Fatal(err)
			}
			before, err := adapter.Observe()
			if err != nil {
				t.Fatal(err)
			}
			if !before.Controllable || before.InBattle || before.Story.Has(stage.progress) {
				t.Fatalf("unstable or prematurely completed %s: %+v", stage.name, before)
			}
			for _, prior := range completed {
				if !before.Story.Has(prior) {
					t.Fatalf("%s lost before %s", prior, stage.name)
				}
			}
			startFrame := m.FrameCount()
			var result ObjectiveResult
			if err := adapter.WithinObjectiveBudget(o, func() error {
				var executeErr error
				result, executeErr = adapter.ExecuteOwned(o)
				return executeErr
			}); err != nil {
				t.Fatalf("stage %s failed at frame %d (+%d): %v",
					stage.name, m.FrameCount(), m.FrameCount()-startFrame, err)
			}
			if err := adapter.NormalizeBoundary(); err != nil {
				t.Fatal(err)
			}
			after, err := adapter.Observe()
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.VerifyPostcondition(o, before, after, result); err != nil {
				t.Fatalf("%s did not satisfy retail objective postcondition: %v", stage.name, err)
			}
			if !after.Controllable || after.InBattle || !after.Story.Has(stage.progress) {
				t.Fatalf("%s has no stable cartridge proof after executor", stage.name)
			}
			for _, prior := range completed {
				if !after.Story.Has(prior) {
					t.Fatalf("lost earlier stage %s after %s", prior, stage.name)
				}
			}
			state, err := m.SaveStateChecked()
			if err != nil {
				t.Fatal(err)
			}
			fname := fmt.Sprintf("%02d-%s", i+1, stage.name)
			if err := os.WriteFile(filepath.Join(out, fname+".state"), state, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := gsCoastalWriteJSON(filepath.Join(out, fname+".json"), after); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(state)
			t.Logf("PASS %s frames=%d sha256=%x", stage.name, m.FrameCount()-startFrame, hash)
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			m, err = emu.Open(romPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.LoadState(state); err != nil {
				t.Fatalf("reopen %s: %v", stage.name, err)
			}
			restarted, err := ObserveChecked(m, m.ROM())
			if err != nil {
				t.Fatal(err)
			}
			if restarted.GameID != id || !restarted.Controllable || restarted.InBattle ||
				!restarted.Story.Has(stage.progress) {
				t.Fatalf("lost %s after emulator reopen", stage.name)
			}
			for _, prior := range completed {
				if !restarted.Story.Has(prior) {
					t.Fatalf("lost %s across %s's checkpoint reopen", prior, stage.name)
				}
			}
		}) {
			return
		}
		completed = append(completed, stage.progress)
	}
	final, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	status := EvaluateGoal(Goal{Kind: GoalBadges, Count: 8}, final)
	if !status.Complete || status.Current != 8 ||
		!final.Story.Has(gsprofile.ProgressClairDefeated) ||
		!final.Story.Has(gsprofile.ProgressRisingBadgeEarned) ||
		!final.Story.Has(gsprofile.ProgressRadioTowerTakeoverSettled) {
		t.Fatalf("six-to-eight campaign did not earn all eight actual badges: %+v", status)
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "final.json"), final); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS complete Gold/Silver %s six-to-eight badge replay", variant)
}
