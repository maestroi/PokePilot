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

// These replay selections start from the *preceding* verified retail
// milestone, not from a fabricated state or from the result of the fight.
// The final selection intentionally covers both Lance and the Hall of Fame:
// the Lance flag is transient, and the second objective resumes its in-flight
// Oak/Mary cutscene without returning to an ordinary overworld boundary.
type gsLeagueReplaySpec struct {
	name       string
	source     string
	objectives []game.ProgressID
}

var gsLeagueReplaySpecs = [...]gsLeagueReplaySpec{
	{"indigo", "post-rising", []game.ProgressID{gsprofile.ProgressIndigoPlateauReached}},
	{"will", "post-indigo", []game.ProgressID{gsprofile.ProgressLeagueWillDefeated}},
	{"koga", "post-will", []game.ProgressID{gsprofile.ProgressLeagueKogaDefeated}},
	{"bruno", "post-koga", []game.ProgressID{gsprofile.ProgressLeagueBrunoDefeated}},
	{"karen", "post-bruno", []game.ProgressID{gsprofile.ProgressLeagueKarenDefeated}},
	{"hall-of-fame", "post-karen", []game.ProgressID{
		gsprofile.ProgressLeagueLanceDefeated,
		gsprofile.ProgressLeagueHallOfFameEntered,
	}},
}

func gsLeagueReplaySelection(name string) (gsLeagueCheckpointBoundary, []game.ProgressID, bool) {
	for _, replay := range gsLeagueReplaySpecs {
		if replay.name != name {
			continue
		}
		for _, source := range gsLeagueCheckpointBoundaries {
			if source.directory == replay.source {
				return source, replay.objectives, true
			}
		}
	}
	return gsLeagueCheckpointBoundary{}, nil, false
}

// TestGSLeagueROMCheckpointReplay is a private, explicitly opt-in replay of
// one Gold/Silver League leg. It fails closed on missing ROM, source.json or
// a wrong checkpoint; ordinary public short CI never runs retail cartridges.
func TestGSLeagueROMCheckpointReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("private Gold/Silver retail ROM replay")
	}
	if os.Getenv("POKEPILOT_GS_LEAGUE_REPLAY") != "1" {
		t.Skip("opt-in League checkpoint replay")
	}
	stageName := strings.TrimSpace(os.Getenv("POKEPILOT_GS_LEAGUE_STAGE"))
	source, goals, ok := gsLeagueReplaySelection(stageName)
	if !ok {
		t.Fatalf("unsupported POKEPILOT_GS_LEAGUE_STAGE %q", stageName)
	}
	variant := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_GS_LEAGUE_GAME")))
	var id game.GameID
	var expectedROM string
	switch variant {
	case "gold":
		id, expectedROM = gsprofile.GoldGameID, sym.GoldSHA1
	case "silver":
		id, expectedROM = gsprofile.SilverGameID, sym.SilverSHA1
	default:
		t.Fatalf("expected gold or silver cartridge; got %q", variant)
	}
	romPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_LEAGUE_ROM"))
	statePath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_LEAGUE_START"))
	if romPath == "" || statePath == "" {
		t.Fatal("private ROM and genuine milestone start.state are mandatory; never green-skip missing fixtures")
	}
	romBytes, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read %s ROM: %v", variant, err)
	}
	romHash := sha1.Sum(romBytes)
	if len(romBytes) != sym.ROMSize || hex.EncodeToString(romHash[:]) != expectedROM {
		t.Fatalf("unexpected %s retail ROM size or SHA-1", variant)
	}
	state, err := os.ReadFile(statePath)
	if err != nil || len(state) == 0 {
		t.Fatalf("read private checkpoint: size=%d err=%v", len(state), err)
	}
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "source.json"))
	if err != nil {
		t.Fatalf("private checkpoint needs source.json: %v", err)
	}
	provenance, err := gsVerifyQualificationProvenanceForObjective(
		manifest, variant, expectedROM, state, string(source.progress))
	if err != nil {
		t.Fatalf("untrusted %s source checkpoint: %v", source.directory, err)
	}
	out := strings.TrimSpace(os.Getenv("POKEPILOT_GS_LEAGUE_OUT"))
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	startHash := sha256.Sum256(state)
	t.Logf("League replay %s / %s, original run=%s frame=%d state_sha256=%x",
		variant, stageName, provenance.RunID, provenance.Frame, startHash)
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(state); err != nil {
		t.Fatalf("restore private %s checkpoint: %v", source.directory, err)
	}
	initial, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	profile := gsprofile.New(id)
	wantMap, err := gsOpeningMapID(source.mapName)
	if err != nil {
		t.Fatal(err)
	}
	if initial.GameID != id || !initial.Controllable || initial.InBattle ||
		!initial.Story.Has(source.progress) ||
		!initial.Story.Has(source.previous) ||
		initial.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) ||
		profile.DecodeOverworld(m).NativeMapID != wantMap {
		t.Fatalf("untrusted League input: expected settled %s on %s with prerequisite %s",
			source.progress, source.mapName, source.previous)
	}
	for _, target := range goals {
		if initial.Story.Has(target) {
			t.Fatalf("checkpoint already has target %s; would falsely green-pass", target)
		}
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "initial.json"), initial); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !t.Failed() {
			return
		}
		if raw, e := m.SaveStateChecked(); e == nil {
			_ = os.WriteFile(filepath.Join(out, "failure.state"), raw, 0o600)
		}
		if obs, e := ObserveChecked(m, m.ROM()); e == nil {
			_ = gsCoastalWriteJSON(filepath.Join(out, "failure.json"), obs)
		}
	}()

	for _, target := range goals {
		t.Run(string(target), func(t *testing.T) {
			adapter := newGSObjectiveAdapter(m, m.ROM(), id)
			o := Objective{Kind: KindProgress, Progress: target}
			before, err := adapter.Observe()
			if err != nil {
				t.Fatal(err)
			}
			if before.Story.Has(target) {
				t.Fatalf("replay target %s already complete", target)
			}
			if err := adapter.Validate(o, before); err != nil {
				t.Fatal(err)
			}
			startFrame := m.FrameCount()
			var result ObjectiveResult
			if target == gsprofile.ProgressLeagueHallOfFameEntered {
				// Lance stops as soon as his battle event appears. His automatic
				// ending sequence can still be running, so the ordinary generic
				// start/finish boundary is not the right interface here.
				if !before.Story.Has(gsprofile.ProgressLeagueLanceDefeated) {
					t.Fatal("Hall of Fame continuation requires a proven Lance victory")
				}
				err = adapter.WithinObjectiveBudget(o, func() error {
					var executeErr error
					result, executeErr = adapter.ExecuteOwned(o)
					return executeErr
				})
			} else {
				result, err = ExecuteWithAdapter(adapter, o)
			}
			if err != nil {
				t.Fatalf("%s execution failed after %d frames: %v",
					target, m.FrameCount()-startFrame, err)
			}
			after, err := adapter.Observe()
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.VerifyPostcondition(o, before, after, result); err != nil {
				t.Fatalf("%s did not prove the real retail event: %v", target, err)
			}
			if !after.Story.Has(target) {
				t.Fatalf("%s returned without its actual cartridge flag", target)
			}
			if target != gsprofile.ProgressLeagueLanceDefeated &&
				target != gsprofile.ProgressLeagueHallOfFameEntered &&
				(!after.Controllable || after.InBattle) {
				t.Fatalf("%s did not settle into a replayable overworld boundary", target)
			}
			if err := gsCoastalWriteJSON(filepath.Join(out, fmt.Sprintf("%s.json", stageName)), after); err != nil {
				t.Fatal(err)
			}
			t.Logf("PASS %s in %d frames", target, m.FrameCount()-startFrame)
		})
		if t.Failed() {
			return
		}
	}
	final, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	if !final.Story.Has(goals[len(goals)-1]) {
		t.Fatal("League replay ended without target progress")
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "final.json"), final); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS %s/%s from authentic %s fixture", variant, stageName, source.directory)
}
