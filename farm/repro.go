package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// FailureReproArtifactName is the small, ROM-free contract handed to a
	// fixer alongside the pre-objective checkpoint that reproduced a structured
	// farm failure. The ROM remains local to the fixer/qualification host.
	FailureReproArtifactName = "failure-repro.json"
	failureReproVersion      = 1
)

// FailureReproCheckpoint names the exact pre-objective state needed by a
// fixer. Storage metadata is a locator only; ROM bytes and credentials are
// never part of this contract.
type FailureReproCheckpoint struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Store     string `json:"store,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	ObjectKey string `json:"object_key,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// FailureReproBundle is an executable handoff for one structured objective
// failure. A fixer downloads this JSON and the named checkpoint, keeps its ROM
// local, then runs cmd/pokerepro. The canonical failure identity is preserved
// verbatim so the observed outcome/cause can be compared without parsing prose.
type FailureReproBundle struct {
	Version          int                    `json:"version"`
	RunID            string                 `json:"run_id"`
	Attempt          int                    `json:"attempt"`
	ObservedRevision string                 `json:"observed_revision,omitempty"`
	Fingerprint      string                 `json:"fingerprint"`
	Checkpoint       FailureReproCheckpoint `json:"checkpoint"`
	Identity         FailureIdentity        `json:"identity"`
	Diagnostic       string                 `json:"diagnostic,omitempty"`
	SuggestedCommand string                 `json:"suggested_command"`
}

// NewFailureReproArtifact builds a repro contract when a failure has both a
// canonical identity and an exact pre-objective checkpoint in the finish
// evidence. Legacy/prose-only failures deliberately return an empty artifact.
func NewFailureReproArtifact(report FinishReport, f ObjectiveFailure) (Artifact, error) {
	if f.Identity == nil || strings.TrimSpace(f.Checkpoint) == "" {
		return Artifact{}, nil
	}
	if err := validateObjectiveFailure(f); err != nil {
		return Artifact{}, err
	}
	var checkpoint *Artifact
	for i := range report.Artifacts {
		if report.Artifacts[i].Name == f.Checkpoint {
			checkpoint = &report.Artifacts[i]
			break
		}
	}
	if checkpoint == nil {
		return Artifact{}, nil
	}
	bundle := FailureReproBundle{
		Version:          failureReproVersion,
		RunID:            report.RunID,
		Attempt:          maxInt(1, report.Attempt),
		ObservedRevision: firstNonEmpty(strings.TrimSpace(f.Build), strings.TrimSpace(report.RunnerVersion)),
		Fingerprint:      f.Fingerprint,
		Checkpoint: FailureReproCheckpoint{
			Name:      checkpoint.Name,
			SHA256:    checkpoint.SHA256,
			Store:     checkpoint.Store,
			Bucket:    checkpoint.Bucket,
			ObjectKey: checkpoint.ObjectKey,
			Size:      checkpoint.Size,
		},
		Identity:         *f.Identity,
		Diagnostic:       strings.TrimSpace(f.Error),
		SuggestedCommand: "go run ./cmd/pokerepro -bundle " + FailureReproArtifactName,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("farm: encode failure repro: %w", err)
	}
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      FailureReproArtifactName,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}, nil
}

// DecodeFailureRepro validates a fixer handoff before it is allowed to drive an
// emulator. In particular the embedded fingerprint must still agree with the
// canonical identity.
func DecodeFailureRepro(data []byte) (FailureReproBundle, error) {
	var bundle FailureReproBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return bundle, fmt.Errorf("farm: decode failure repro: %w", err)
	}
	if bundle.Version != failureReproVersion {
		return bundle, fmt.Errorf("farm: failure repro version %d, want %d", bundle.Version, failureReproVersion)
	}
	if strings.TrimSpace(bundle.RunID) == "" || strings.TrimSpace(bundle.Checkpoint.Name) == "" || strings.TrimSpace(bundle.Checkpoint.SHA256) == "" {
		return bundle, fmt.Errorf("farm: incomplete failure repro")
	}
	_, fingerprint, err := FingerprintFailureIdentity(bundle.Identity)
	if err != nil {
		return bundle, err
	}
	if bundle.Fingerprint != fingerprint {
		return bundle, fmt.Errorf("farm: failure repro fingerprint mismatch")
	}
	return bundle, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
