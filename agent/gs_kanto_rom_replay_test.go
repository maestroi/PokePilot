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

// Replay uses the first genuine completion of the PREVIOUS milestone rather
// than building an impossible new save with the next event already set.
type gsKantoReplaySpec struct {
	name   string
	source string
	goal   game.ProgressID
}

var gsKantoReplaySpecs = [...]gsKantoReplaySpec{
	{"first-landing", "post-ticket", gsprofile.ProgressKantoFirstLanding},
	{"thunder", "post-landing", gsprofile.ProgressKantoThunderBadgeEarned},
	{"marsh", "post-thunder", gsprofile.ProgressKantoMarshBadgeEarned},
	{"rainbow", "post-marsh", gsprofile.ProgressKantoRainbowBadgeEarned},
	{"soul", "post-rainbow", gsprofile.ProgressKantoSoulBadgeEarned},
	{"manager", "post-soul", gsprofile.ProgressKantoManagerMet},
	{"rocket", "post-manager", gsprofile.ProgressKantoRocketAtGym},
	{"machine-part", "post-rocket", gsprofile.ProgressKantoMachinePartFound},
	{"restore-power", "post-machine-part", gsprofile.ProgressKantoPowerRestored},
	{"copycat", "post-power", gsprofile.ProgressKantoCopycatMet},
	{"lost-doll", "post-copycat", gsprofile.ProgressKantoLostDollReceived},
	{"rail-pass", "post-lost-doll", gsprofile.ProgressKantoRailPassReceived},
	{"misty-date", "post-rail-pass", gsprofile.ProgressKantoMistyReturned},
	{"cascade", "post-misty-date", gsprofile.ProgressKantoCascadeBadgeEarned},
	{"expn-card", "post-cascade", gsprofile.ProgressKantoExpansionCard},
	{"snorlax", "post-expn-card", gsprofile.ProgressKantoSnorlaxCleared},
	{"boulder", "post-snorlax", gsprofile.ProgressKantoBoulderBadgeEarned},
	{"volcano", "post-boulder", gsprofile.ProgressKantoVolcanoBadgeEarned},
	{"invite-blue", "post-volcano", gsprofile.ProgressKantoBlueInvited},
	{"earth", "post-blue-invited", gsprofile.ProgressKantoEarthBadgeEarned},
	{"mt-silver-permit", "post-earth", gsprofile.ProgressMtSilverOpened},
	// Red's event occurs during the scripted credits sequence, when the
	// overworld is not controllable; only its PRE-fight checkpoint is saved.
	{"red", "post-mt-silver-permit", gsprofile.ProgressMtSilverRedDefeated},
}

func gsKantoReplaySelection(name string) (gsKantoCheckpointBoundary, game.ProgressID, bool) {
	for _, spec := range gsKantoReplaySpecs {
		if spec.name != name {
			continue
		}
		for _, source := range gsKantoCheckpointBoundaries {
			if source.directory == spec.source {
				return source, spec.goal, true
			}
		}
	}
	return gsKantoCheckpointBoundary{}, "", false
}

// TestGSKantoROMCheckpointReplay is intentionally absent from public short
// CI: it needs a verified retail ROM and a private, immutable farm checkpoint.
// It fails rather than green-skipping an absent or forged input when enabled.
// Each selector runs one real story transaction and proves its event/badge
// through the production adapter's postcondition, with no RAM modifications.
func TestGSKantoROMCheckpointReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("private Gold/Silver Kanto ROM replay")
	}
	if os.Getenv("POKEPILOT_GS_KANTO_REPLAY") != "1" {
		t.Skip("opt-in Kanto checkpoint replay")
	}
	stageName := strings.TrimSpace(os.Getenv("POKEPILOT_GS_KANTO_STAGE"))
	source, target, ok := gsKantoReplaySelection(stageName)
	if !ok {
		t.Fatalf("unsupported POKEPILOT_GS_KANTO_STAGE %q", stageName)
	}
	variant := strings.ToLower(strings.TrimSpace(os.Getenv("POKEPILOT_GS_KANTO_GAME")))
	var id game.GameID
	var expectedROM string
	switch variant {
	case "gold":
		id, expectedROM = gsprofile.GoldGameID, sym.GoldSHA1
	case "silver":
		id, expectedROM = gsprofile.SilverGameID, sym.SilverSHA1
	default:
		t.Fatalf("expected gold or silver cartridge, got %q", variant)
	}
	romPath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_KANTO_ROM"))
	statePath := strings.TrimSpace(os.Getenv("POKEPILOT_GS_KANTO_START"))
	if romPath == "" || statePath == "" {
		t.Fatal("private ROM and authentic milestone start.state are mandatory")
	}
	romBytes, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
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
	out := strings.TrimSpace(os.Getenv("POKEPILOT_GS_KANTO_OUT"))
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(state)
	t.Logf("Kanto replay %s/%s from run=%s frame=%d state_sha256=%x",
		variant, stageName, provenance.RunID, provenance.Frame, sum)

	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(state); err != nil {
		t.Fatalf("restore %s checkpoint: %v", source.directory, err)
	}
	profile := gsprofile.New(id)
	initial, err := ObserveChecked(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	wantMap, err := gsOpeningMapID(source.mapName)
	if err != nil {
		t.Fatal(err)
	}
	if initial.GameID != id || !initial.Controllable || initial.InBattle ||
		!initial.Story.Has(gsprofile.ProgressLeagueHallOfFameEntered) ||
		!initial.Story.Has(source.progress) || !initial.Story.Has(source.previous) ||
		initial.Story.Has(target) ||
		profile.DecodeOverworld(m).NativeMapID != wantMap {
		t.Fatalf("untrusted Kanto replay input: require settled %s on %s, prerequisite %s and unearned %s",
			source.progress, source.mapName, source.previous, target)
	}
	if (source.progress == gsprofile.ProgressSSTicketReceived && initial.Region != string(game.RegionJohto)) ||
		(source.progress != gsprofile.ProgressSSTicketReceived && initial.Region != string(game.RegionKanto)) {
		t.Fatalf("Kanto replay source in unexpected region: %s", initial.Region)
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

	adapter := newGSObjectiveAdapter(m, m.ROM(), id)
	objective := Objective{Kind: KindProgress, Progress: target}
	before, err := adapter.Observe()
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Validate(objective, before); err != nil {
		t.Fatal(err)
	}
	startFrame := m.FrameCount()
	result, err := ExecuteWithAdapter(adapter, objective)
	if err != nil {
		t.Fatalf("%s execution failed after %d frames: %v",
			target, m.FrameCount()-startFrame, err)
	}
	after, err := adapter.Observe()
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.VerifyPostcondition(objective, before, after, result); err != nil {
		t.Fatalf("%s did not prove the retail event: %v", target, err)
	}
	if !after.Story.Has(target) || after.InBattle || !after.Controllable ||
		after.Region != string(game.RegionKanto) {
		t.Fatalf("%s did not finish at a stable Kanto story boundary", target)
	}
	if err := gsCoastalWriteJSON(filepath.Join(out, fmt.Sprintf("%s.json", stageName)), after); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS %s/%s from authentic %s fixture in %d frames",
		variant, stageName, source.directory, m.FrameCount()-startFrame)
}
