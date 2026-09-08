package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// ObjectiveFailureArtifactName is the structured, bounded summary of
	// objective failures observed during one run. It rides FinishReport's
	// generic artifact channel so older walls/runners remain wire-compatible.
	ObjectiveFailureArtifactName = "objective-failures.json"
	objectiveFailureVersion      = 3
)

// ObjectiveFailure is one normalized failure group from a run. Count is how
// many rounds hit the same structured identity; FirstRound/LastRound bound the
// sightings. Error is diagnostic prose only and is explicitly NOT an identity
// input. Version-1 artifacts omit the structured fields and remain readable for
// historical evidence.
type ObjectiveFailure struct {
	Objective      string    `json:"objective"`
	Error          string    `json:"error"`
	Count          int       `json:"count"`
	FirstRound     int       `json:"first_round"`
	LastRound      int       `json:"last_round"`
	Map            uint8     `json:"map"`
	X              uint8     `json:"x"`
	Y              uint8     `json:"y"`
	Recovered      bool      `json:"recovered"`
	RecoveredCount int       `json:"recovered_count,omitempty"`
	TerminalCount  int       `json:"terminal_count,omitempty"`
	Blocking       bool      `json:"blocking,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`

	Key          string           `json:"key,omitempty"`
	Fingerprint  string           `json:"fingerprint,omitempty"`
	Identity     *FailureIdentity `json:"identity,omitempty"`
	Build        string           `json:"build,omitempty"`
	Outcome      string           `json:"outcome,omitempty"`
	Cause        string           `json:"cause,omitempty"`
	CauseContext []string         `json:"cause_context,omitempty"`
	Checkpoint   string           `json:"checkpoint,omitempty"`
}

type objectiveFailureEnvelope struct {
	Version  int                `json:"version"`
	Failures []ObjectiveFailure `json:"failures"`
}

// NewObjectiveFailureArtifact encodes failures as one small JSON artifact.
// Empty telemetry produces no artifact so old/no-failure runs stay compact.
// Structured entries are self-checking: identity and persisted fingerprint
// must agree before evidence is accepted.
func NewObjectiveFailureArtifact(failures []ObjectiveFailure) (Artifact, error) {
	if len(failures) == 0 {
		return Artifact{}, nil
	}
	for i := range failures {
		if err := validateObjectiveFailure(failures[i]); err != nil {
			return Artifact{}, fmt.Errorf("farm: objective failure %d: %w", i, err)
		}
	}
	data, err := json.Marshal(objectiveFailureEnvelope{
		Version:  objectiveFailureVersion,
		Failures: failures,
	})
	if err != nil {
		return Artifact{}, fmt.Errorf("farm: encode objective failures: %w", err)
	}
	sum := sha256.Sum256(data)
	return Artifact{
		Name:      ObjectiveFailureArtifactName,
		MediaType: "application/json",
		SHA256:    hex.EncodeToString(sum[:]),
		Data:      data,
	}, nil
}

// DecodeObjectiveFailures extracts objective-failure telemetry from a finish
// report. A missing artifact is the backward-compatible empty case. Duplicate
// telemetry artifacts are rejected because choosing one would make issue
// deduplication depend on artifact order. Version 1 remains supported as
// historical prose-only evidence; version 2 carries canonical identities.
func DecodeObjectiveFailures(report FinishReport) ([]ObjectiveFailure, error) {
	var data []byte
	for _, a := range report.Artifacts {
		if a.Name != ObjectiveFailureArtifactName {
			continue
		}
		if data != nil {
			return nil, fmt.Errorf("farm: duplicate %s artifact", ObjectiveFailureArtifactName)
		}
		data = a.Data
	}
	if data == nil {
		return nil, nil
	}
	var env objectiveFailureEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("farm: decode objective failures: %w", err)
	}
	if env.Version != 1 && env.Version != 2 && env.Version != objectiveFailureVersion {
		return nil, fmt.Errorf("farm: objective failure telemetry version %d, want 1, 2 or %d", env.Version, objectiveFailureVersion)
	}
	if env.Version < 3 {
		for i := range env.Failures {
			if env.Failures[i].Recovered {
				env.Failures[i].RecoveredCount = env.Failures[i].Count
			} else if env.Failures[i].Blocking && env.Failures[i].Count > 0 {
				env.Failures[i].TerminalCount = 1
			}
		}
	}
	if env.Version >= 2 {
		for i := range env.Failures {
			if err := validateObjectiveFailure(env.Failures[i]); err != nil {
				return nil, fmt.Errorf("farm: objective failure %d: %w", i, err)
			}
		}
	}
	return append([]ObjectiveFailure(nil), env.Failures...), nil
}

func validateObjectiveFailure(f ObjectiveFailure) error {
	if f.RecoveredCount < 0 || f.TerminalCount < 0 || f.RecoveredCount+f.TerminalCount > f.Count {
		return fmt.Errorf("invalid recovery impact counts recovered=%d terminal=%d count=%d", f.RecoveredCount, f.TerminalCount, f.Count)
	}
	if f.Identity == nil {
		// Legacy/manual entries remain legal. New runner-generated entries carry
		// Identity, Key and Fingerprint together.
		return nil
	}
	key, fingerprint, err := FingerprintFailureIdentity(*f.Identity)
	if err != nil {
		return err
	}
	if f.Key != key || f.Fingerprint != fingerprint {
		return fmt.Errorf("structured fingerprint mismatch")
	}
	if f.Outcome != "" && f.Outcome != f.Identity.Outcome {
		return fmt.Errorf("outcome does not match identity")
	}
	if f.Cause != "" && f.Cause != f.Identity.Cause {
		return fmt.Errorf("cause does not match identity")
	}
	return nil
}
