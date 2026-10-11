package agent

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// POKEPILOT_QUALIFICATION_CORPUS points at a PRIVATE, writable filesystem
// shared by the Gold/Silver farm runner and the ROM-equipped qualification
// runner. If it is unset, regular gameplay performs no extra state captures.
const gsQualificationCorpusEnv = "POKEPILOT_QUALIFICATION_CORPUS"

type gsQualificationProvenance struct {
	Schema      int    `json:"schema"`
	Game        string `json:"game"`
	Objective   string `json:"objective"`
	RunID       string `json:"run_id,omitempty"`
	ROM_SHA1    string `json:"rom_sha1"`
	StateSHA256 string `json:"state_sha256"`
	Frame       uint64 `json:"frame"`
	CapturedAt  string `json:"captured_at"`
}

// gsPostCureCaptureEligible deliberately requires the first *successful*
// Jasmine-cure transaction, not a loaded state merely claiming the flag.
// HM04 must still be missing, so this checkpoint can genuinely exercise the
// complete Olivine/Cianwood qualification chain.
func gsPostCureCaptureEligible(o Objective, before, after Observation) bool {
	return o.Kind == KindProgress && o.Progress == gsprofile.ProgressJasmineReturnedToGym &&
		(before.GameID == gsprofile.GoldGameID || before.GameID == gsprofile.SilverGameID) &&
		after.GameID == before.GameID &&
		before.Controllable && !before.InBattle &&
		after.Controllable && !after.InBattle &&
		!before.Story.Has(gsprofile.ProgressJasmineReturnedToGym) &&
		after.Story.Has(gsprofile.ProgressJasmineReturnedToGym) &&
		after.Story.Has(gsprofile.ProgressSecretPotionAcquired) &&
		!after.Story.Has(gsprofile.ProgressHM04StrengthAcquired) &&
		!after.Story.Has(gsprofile.ProgressStormBadgeEarned) &&
		!after.Story.Has(gsprofile.ProgressMineralBadgeEarned)
}

// CaptureQualificationCheckpoint is an opt-in capability of the Gen2 adapter.
// The generic run loop invokes it only AFTER the objective transaction has
// passed its own postcondition. A failed diagnostic write does not invalidate
// the game action; the caller reports it to the run log.
func (a *gsObjectiveAdapter) CaptureQualificationCheckpoint(o Objective, before, after Observation) (string, error) {
	root := strings.TrimSpace(os.Getenv(gsQualificationCorpusEnv))
	if root == "" || !gsPostCureCaptureEligible(o, before, after) {
		return "", nil
	}
	if a.m == nil {
		return "", fmt.Errorf("gen2 qualification capture: nil emulator")
	}
	overworld := gsprofile.New(a.gameID).DecodeOverworld(a.m)
	lighthouse, err := gsOpeningMapID("OLIVINE_LIGHTHOUSE_6F")
	if err != nil {
		return "", err
	}
	if overworld.NativeMapID != lighthouse || !overworld.Controllable {
		return "", fmt.Errorf("gen2 qualification capture: cure milestone is not settled in Lighthouse 6F")
	}
	var variant string
	switch after.GameID {
	case gsprofile.GoldGameID:
		variant = "gold"
	case gsprofile.SilverGameID:
		variant = "silver"
	default:
		return "", fmt.Errorf("gen2 qualification capture: unsupported game %s", after.GameID)
	}
	target := filepath.Join(root, "gs-coastal", variant, "post-cure", "start.state")
	// The first complete, tested save in this private corpus becomes the fixed
	// regression input. Another concurrent runner cannot overwrite it.
	if _, err := os.Stat(target); err == nil {
		return "", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	state, err := a.m.SaveStateChecked()
	if err != nil {
		return "", fmt.Errorf("gen2 qualification capture: SaveStateChecked: %w", err)
	}
	if len(state) == 0 {
		return "", fmt.Errorf("gen2 qualification capture: empty checked state")
	}
	romSum := sha1.Sum(a.romData)
	stateSum := sha256.Sum256(state)
	provenance := gsQualificationProvenance{
		Schema:      1,
		Game:        variant,
		Objective:   string(o.Progress),
		RunID:       strings.TrimSpace(os.Getenv("POKEPILOT_RUN_ID")),
		ROM_SHA1:    hex.EncodeToString(romSum[:]),
		StateSHA256: hex.EncodeToString(stateSum[:]),
		Frame:       a.m.FrameCount(),
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	newState, err := gsPublishQualificationStateOnce(target, state, provenance)
	if err != nil {
		return "", fmt.Errorf("gen2 qualification capture: publish: %w", err)
	}
	if !newState {
		return "", nil
	}
	return target, nil
}

// gsPublishQualificationStateOnce writes private bytes to a temporary file
// and publishes them with an atomic, no-clobber hard link. A partial state is
// never visible as start.state, even if the worker terminates mid-write.
// Metadata is informational; it cannot make an incomplete checkpoint valid.
func gsPublishQualificationStateOnce(target string, state []byte, provenance gsQualificationProvenance) (bool, error) {
	if len(state) == 0 {
		return false, fmt.Errorf("empty qualification state")
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, ".qualification-*.state")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return false, err
	}
	if _, err := tmp.Write(state); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Link(tmp.Name(), target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}

	meta, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return true, err
	}
	meta = append(meta, '\n')
	metaTarget := filepath.Join(dir, "source.json")
	metaFile, err := os.CreateTemp(dir, ".qualification-*.json")
	if err != nil {
		return true, err
	}
	defer os.Remove(metaFile.Name())
	if err := metaFile.Chmod(0o600); err != nil {
		metaFile.Close()
		return true, err
	}
	if _, err := metaFile.Write(meta); err != nil {
		metaFile.Close()
		return true, err
	}
	if err := metaFile.Close(); err != nil {
		return true, err
	}
	if err := os.Rename(metaFile.Name(), metaTarget); err != nil {
		return true, err
	}
	return true, nil
}


// gsVerifyQualificationProvenance verifies the source manifest against the
// *actual* checked state bytes and cartridge identity. The self-hosted
// qualification runner never accepts a forged or stale metadata claim as
// evidence that a trusted first-cure checkpoint was captured.
func gsVerifyQualificationProvenance(raw []byte, variant, romSHA1 string, state []byte) (gsQualificationProvenance, error) {
	var source gsQualificationProvenance
	if err := json.Unmarshal(raw, &source); err != nil {
		return source, fmt.Errorf("parse Gen2 checkpoint provenance: %w", err)
	}
	if source.Schema != 1 || source.Game != variant ||
		source.Objective != string(gsprofile.ProgressJasmineReturnedToGym) ||
		!strings.EqualFold(source.ROM_SHA1, romSHA1) {
		return source, fmt.Errorf("Gen2 checkpoint provenance does not match the first %s cure transaction", variant)
	}
	hash := sha256.Sum256(state)
	if !strings.EqualFold(source.StateSHA256, hex.EncodeToString(hash[:])) {
		return source, fmt.Errorf("Gen2 checkpoint provenance SHA-256 does not match the checkpoint bytes")
	}
	return source, nil
}
