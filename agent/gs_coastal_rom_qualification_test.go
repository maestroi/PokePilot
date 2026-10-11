package agent

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
)

// This is a PRIVATE ROM-backed qualification barrier, never a ROM-free unit
// test. The self-hosted runner explicitly enables it and provides its retail
// ROM and a checkpoint immediately after the Lighthouse medicine quest.
//
// The same test also accepts an intermediate stage checkpoint for fast failure
// replay. A green result requires all remaining real story transactions,
// including an observed Strength boulder movement inside Chuck's Gym.
func TestGSCoastalROMCheckpointReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("real cartridge checkpoint journey, not the ROM-free short suite")
	}
	if os.Getenv("POKEPILOT_GS_COASTAL_QUAL") != "1" {
		t.Skip("opt-in private Gold/Silver ROM qualification")
	}
	gameName := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_GS_COASTAL_GAME")))
	romPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_COASTAL_ROM"))
	checkpointPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_COASTAL_START"))
	from := strings.TrimSpace(os.Getenv("POKEPILOT_GS_COASTAL_FROM"))
	if from == "" {
		from = "post-cure"
	}
	startStages := map[string]int{
		"post-cure": 0, "post-strength": 1, "post-storm": 2, "post-fly": 3,
	}
	start, ok := startStages[from]
	if !ok {
		t.Fatalf("unsupported POKEPILOT_GS_COASTAL_FROM %q", from)
	}
	var gameID game.GameID
	var expectedSHA string
	switch gameName {
	case "gold":
		gameID, expectedSHA = gsprofile.GoldGameID, sym.GoldSHA1
	case "silver":
		gameID, expectedSHA = gsprofile.SilverGameID, sym.SilverSHA1
	default:
		t.Fatalf("POKEPILOT_GS_COASTAL_GAME must be gold or silver, got %q", gameName)
	}
	if romPath == "" || checkpointPath == "" {
		t.Fatal("POKEPILOT_GS_COASTAL_ROM and POKEPILOT_GS_COASTAL_START are required: missing private fixtures must fail, not skip")
	}
	rawROM, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read private ROM: %v", err)
	}
	sum := sha1.Sum(rawROM)
	if len(rawROM) != sym.ROMSize || hex.EncodeToString(sum[:]) != expectedSHA {
		t.Fatalf("unsupported %s ROM: size=%d sha1=%x; required size=%d sha1=%s",
			gameName, len(rawROM), sum, sym.ROMSize, expectedSHA)
	}
	rawState, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read private checkpoint: %v", err)
	}
	out := os.Getenv("POKEPILOT_GS_COASTAL_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	startHash := sha256.Sum256(rawState)
	t.Logf("ROM=%s sha1=%s from=%s start_sha256=%x evidence=%s", gameName, expectedSHA, from, startHash, out)

	stages := []struct {
		name string
		goal game.ProgressID
		mapName string
	}{
		{"strength", gsprofile.ProgressHM04StrengthAcquired, "OLIVINE_CAFE"},
		{"storm", gsprofile.ProgressStormBadgeEarned, "CIANWOOD_GYM"},
		{"fly", gsprofile.ProgressHM02FlyAcquired, "CIANWOOD_CITY"},
		{"mineral", gsprofile.ProgressMineralBadgeEarned, "OLIVINE_GYM"},
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open %s ROM: %v", gameName, err)
	}
	defer func() { _ = m.Close() }()
	if err := m.LoadState(rawState); err != nil {
		t.Fatalf("load %s checkpoint: %v", from, err)
	}
	profile := gsprofile.New(gameID)
	initial, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatalf("observe initial checkpoint: %v", err)
	}
	if initial.GameID != gameID || !initial.Controllable || initial.InBattle ||
		!initial.Story.Has(gsprofile.ProgressJasmineReturnedToGym) {
		t.Fatalf("checkpoint is not settled %s after the Lighthouse cure: game=%s control=%t battle=%t cure=%t",
			gameName, initial.GameID, initial.Controllable, initial.InBattle,
			initial.Story.Has(gsprofile.ProgressJasmineReturnedToGym))
	}
	for i, stage := range stages {
		have := initial.Story.Has(stage.goal)
		if (i < start) != have {
			t.Fatalf("%s checkpoint contradicts stage %s: completed=%t (from=%s)", from, stage.goal, have, from)
		}
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "initial.json"), initial); err != nil {
		t.Fatal(err)
	}
	// Keep a failure checkpoint on the private runner even if an objective
	// errors partway through a scripted battle or a navigation transaction.
	defer func() {
		if !t.Failed() {
			return
		}
		if state, err := m.SaveStateChecked(); err == nil {
			_ = os.WriteFile(filepath.Join(out, "failure.state"), state, 0o600)
		}
		if obs, err := ObserveChecked(m, m.ROM()); err == nil {
			_ = gsCoastalWriteJSON(filepath.Join(out, "failure.json"), obs)
		}
	}()

	for i := start; i < len(stages); i++ {
		stage := stages[i]
		t.Run(stage.name, func(t *testing.T) {
			adapter := newGSObjectiveAdapter(m, m.ROM(), gameID)
			o := Objective{Kind: KindProgress, Progress: stage.goal}
			if err := adapter.Validate(o, Observation{}); err != nil {
				t.Fatal(err)
			}
			before, err := adapter.Observe()
			if err != nil {
				t.Fatal(err)
			}
			if before.Story.Has(stage.goal) || !before.Controllable || before.InBattle {
				t.Fatalf("not a fresh stable %s transaction: controllable=%t battle=%t complete=%t",
					stage.name, before.Controllable, before.InBattle, before.Story.Has(stage.goal))
			}
			// A replay that starts inside Chuck's Gym, already past the boulders,
			// cannot count as qualification of the actual Strength traversal.
			if stage.name == "storm" {
				gym, err := gsOpeningMapID("CIANWOOD_GYM")
				if err != nil {
					t.Fatal(err)
				}
				if profile.DecodeOverworld(m).NativeMapID == gym {
					t.Fatal("Storm checkpoint starts inside the Gym; it does not prove boulder approach")
				}
			}
			var pushObserved bool
			var firstBoulders = map[int]game.MapPoint{}
			if stage.name == "storm" {
				gym, err := gsOpeningMapID("CIANWOOD_GYM")
				if err != nil {
					t.Fatal(err)
				}
				// Sample only every 30 frames; other frames have no instrumentation
				// overhead beyond a counter. Boulder slots 7-9 are the three
				// native Strength objects; slot 6 is Chuck's scripted throw.
				m.OnFrame(func(e *emu.Emu) {
					if e.FrameCount()%30 != 0 || profile.DecodeOverworld(e).NativeMapID != gym {
						return
					}
					live, err := profile.DecodeLiveTopology(e)
					if err != nil || !live.BlocksSettled {
						return
					}
					for slot := 7; slot <= 9; slot++ {
						point, found := live.ObjectPositions[slot]
						if !found {
							continue
						}
						if first, seen := firstBoulders[slot]; seen && first != point {
							pushObserved = true
						} else if !seen {
							firstBoulders[slot] = point
						}
					}
				})
			}
			if err := adapter.NormalizeBoundary(); err != nil {
				t.Fatalf("normalize initial boundary: %v", err)
			}
			startFrame := m.FrameCount()
			var result ObjectiveResult
			err = adapter.WithinObjectiveBudget(o, func() error {
				var runErr error
				result, runErr = adapter.ExecuteOwned(o)
				return runErr
			})
			m.OnFrame(nil)
			if err != nil {
				t.Fatalf("%s objective failed after %d frames: %v", stage.name, m.FrameCount()-startFrame, err)
			}
			if err := adapter.NormalizeBoundary(); err != nil {
				t.Fatalf("normalize final boundary: %v", err)
			}
			after, err := adapter.Observe()
			if err != nil {
				t.Fatalf("observe final: %v", err)
			}
			if err := adapter.VerifyPostcondition(o, before, after, result); err != nil {
				t.Fatalf("%s did not satisfy semantic postcondition: %v", stage.name, err)
			}
			if !after.Story.Has(stage.goal) || !after.Controllable || after.InBattle {
				t.Fatalf("unstable or uncommitted %s: %+v", stage.name, after.Story)
			}
			wantMap, err := gsOpeningMapID(stage.mapName)
			if err != nil {
				t.Fatal(err)
			}
			if actual := profile.DecodeOverworld(m).NativeMapID; actual != wantMap {
				info, _ := gsdata.Map(actual)
				t.Fatalf("%s ended in %s (%#04x); want %s (%#04x)",
					stage.name, info.Name, actual, stage.mapName, wantMap)
			}
			if stage.name == "storm" && !pushObserved {
				t.Fatal("Storm Badge obtained without observing any of the three native Strength boulders move")
			}
			if err := gsCoastalWriteJSON(filepath.Join(out, fmt.Sprintf("%02d-%s.json", i+1, stage.name)), after); err != nil {
				t.Fatal(err)
			}
			state, err := m.SaveStateChecked()
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(out, fmt.Sprintf("%02d-%s.state", i+1, stage.name))
			if err := os.WriteFile(file, state, 0o600); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(state)
			t.Logf("PASS %s frames=%d checkpoint_sha256=%x native_strength_push=%t",
				stage.goal, m.FrameCount()-startFrame, hash, pushObserved)
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			m, err = emu.Open(romPath)
			if err != nil {
				t.Fatalf("reopen emulator after %s: %v", stage.name, err)
			}
			if err := m.LoadState(state); err != nil {
				t.Fatalf("reload durable %s checkpoint: %v", stage.name, err)
			}
			resumed, err := ObserveChecked(m, m.ROM())
			if err != nil {
				t.Fatal(err)
			}
			if !resumed.Story.Has(stage.goal) || !resumed.Controllable || resumed.InBattle {
				t.Fatalf("checkpoint restore lost %s or stable control", stage.goal)
			}
		})
		if t.Failed() {
			return
		}
	}
	final, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	if !final.Story.Has(gsprofile.ProgressSupportedFrontier) ||
		!final.Story.Has(gsprofile.ProgressStormBadgeEarned) ||
		!final.Story.Has(gsprofile.ProgressMineralBadgeEarned) {
		t.Fatalf("completed coastal replay lacks both badges/six-badge frontier")
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, "final.json"), final); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS %s six-badge coastal ROM checkpoint replay", gameName)
}

func gsCoastalWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
