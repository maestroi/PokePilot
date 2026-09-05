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
	objectiveFailureVersion      = 1
)

// ObjectiveFailure is one normalized failure group from a run. Count is how
// many rounds hit the same objective/error/map shape; FirstRound/LastRound
// bound the sightings. Recovered means meaningful game progress happened
// after the last sighting. Blocking is stronger: the run later stopped
// failed/stuck with this repeated group still unrecovered, making it useful
// as a progression-blocker signal rather than merely a noisy skill edge case.
type ObjectiveFailure struct {
	Objective  string    `json:"objective"`
	Error      string    `json:"error"`
	Count      int       `json:"count"`
	FirstRound int       `json:"first_round"`
	LastRound  int       `json:"last_round"`
	Map        uint8     `json:"map"`
	X          uint8     `json:"x"`
	Y          uint8     `json:"y"`
	Recovered  bool      `json:"recovered"`
	Blocking   bool      `json:"blocking,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

type objectiveFailureEnvelope struct {
	Version  int                `json:"version"`
	Failures []ObjectiveFailure `json:"failures"`
}

// NewObjectiveFailureArtifact encodes failures as one small JSON artifact.
// Empty telemetry produces no artifact so old/no-failure runs stay compact.
func NewObjectiveFailureArtifact(failures []ObjectiveFailure) (Artifact, error) {
	if len(failures) == 0 {
		return Artifact{}, nil
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
// deduplication depend on artifact order.
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
	if env.Version != objectiveFailureVersion {
		return nil, fmt.Errorf("farm: objective failure telemetry version %d, want %d", env.Version, objectiveFailureVersion)
	}
	return append([]ObjectiveFailure(nil), env.Failures...), nil
}
