package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Version 5 replaces presentation-string durable identity with ObjectiveKey
// for completion/failure records and persisted plan steps. The loader retains
// an explicit v4 migration path so existing checkpoints remain resumable.
const (
	memoryVersion       = 5
	legacyMemoryVersion = 4
)

func knowledgeFileNameVersion(stateBase string, version int) string {
	return fmt.Sprintf("%s.knowledge-v%d.json", stateBase, version)
}

func knowledgeFileName(stateBase string) string {
	return knowledgeFileNameVersion(stateBase, memoryVersion)
}

func knowledgePathForStateVersion(statePath string, version int) string {
	base := strings.TrimSuffix(filepath.Base(statePath), ".state")
	return filepath.Join(filepath.Dir(statePath), knowledgeFileNameVersion(base, version))
}

func knowledgePathForState(statePath string) string {
	return knowledgePathForStateVersion(statePath, memoryVersion)
}

func isKnowledgeName(name string) bool {
	return strings.HasSuffix(name, ".json") && strings.Contains(name, "knowledge-v")
}

type storedCompletion struct {
	Key       ObjectiveKey `json:"key,omitempty"`
	Objective string       `json:"objective,omitempty"` // v4 compatibility only
	Times     int          `json:"times"`
}

type storedFailure struct {
	Key       ObjectiveKey `json:"key,omitempty"`
	Mode      string       `json:"mode,omitempty"`
	Objective string       `json:"objective"`
	Times     int          `json:"times"`
	Last      string       `json:"last"`
}

// memoryFile is the serialised form of a run's knowledge, captured beside the
// checkpoint save state it describes. Adjacency is deliberately rebuilt from
// the ROM; observations/history/offers are re-derived after resume.
type memoryFile struct {
	Version      int                `json:"version"`
	Visited      []uint8            `json:"visited"`
	Places       []string           `json:"places"`
	Completed    []storedCompletion `json:"completed"`
	Talked       []talkedKey        `json:"talked"`
	Requirements []Requirement      `json:"requirements,omitempty"`
	Failures     []storedFailure    `json:"failures,omitempty"`
	Intent       string             `json:"intent,omitempty"`
	IntentAge    int                `json:"intent_age,omitempty"`
	Plan         Plan               `json:"plan,omitempty"`
}

type talkedKey struct {
	Map uint8 `json:"map"`
	X   uint8 `json:"x"`
	Y   uint8 `json:"y"`
}

func encodeMemoryFile(k *Knowledge, intent string, intentAge int, plans ...Plan) ([]byte, error) {
	mem := memoryFile{Version: memoryVersion, Intent: intent, IntentAge: intentAge}
	if len(plans) > 0 {
		mem.Plan = plans[0].clone()
	}
	for id := range k.Visited {
		mem.Visited = append(mem.Visited, id)
	}
	for name := range k.Places {
		mem.Places = append(mem.Places, name)
	}
	for storage, times := range k.Completed {
		entry := storedCompletion{Times: times}
		if key, ok := parseObjectiveKeyID(storage); ok {
			entry.Key = key
		} else {
			// A resumed v4 entry may not have been encountered again yet.
			entry.Objective = storage
		}
		mem.Completed = append(mem.Completed, entry)
	}
	for mapID, tiles := range k.Talked {
		for tile := range tiles {
			mem.Talked = append(mem.Talked, talkedKey{Map: mapID, X: tile[0], Y: tile[1]})
		}
	}
	mem.Requirements = append(mem.Requirements, k.Requirements...)
	for storage, failure := range k.Failures {
		entry := storedFailure{Objective: failure.Objective, Times: failure.Times, Last: failure.Last}
		if key, mode, ok := parseFailureStorageKey(storage); ok {
			entry.Key, entry.Mode = key, mode
		}
		mem.Failures = append(mem.Failures, entry)
	}
	return json.Marshal(mem)
}

func writeMemoryFile(statePath string, k *Knowledge, intent string, intentAge int, plans ...Plan) error {
	data, err := encodeMemoryFile(k, intent, intentAge, plans...)
	if err != nil {
		return fmt.Errorf("encode knowledge: %w", err)
	}
	target := knowledgePathForState(statePath)
	tmp, err := os.CreateTemp(filepath.Dir(statePath), ".knowledge-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp knowledge file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write knowledge: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close knowledge: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("rename knowledge into place: %w", err)
	}
	return nil
}

type ResumedMemory struct {
	Knowledge *Knowledge
	Intent    string
	IntentAge int
	Plan      Plan
}

func LoadCheckpointMemory(statePath string, adjacency map[uint8][]uint8, log io.Writer) ResumedMemory {
	empty := ResumedMemory{Knowledge: NewKnowledge(adjacency)}
	base := filepath.Base(statePath)
	if !strings.HasSuffix(base, ".state") {
		logMemory(log, "%s is not a checkpoint .state file; starting with empty knowledge", statePath)
		return empty
	}

	path := knowledgePathForState(statePath)
	data, err := os.ReadFile(path)
	migratingV4 := false
	if errors.Is(err, os.ErrNotExist) {
		legacyPath := knowledgePathForStateVersion(statePath, legacyMemoryVersion)
		if legacy, legacyErr := os.ReadFile(legacyPath); legacyErr == nil {
			data, err, path, migratingV4 = legacy, nil, legacyPath, true
		}
	}
	if err != nil {
		logMemory(log, "no readable knowledge file beside %s (%v); starting with empty knowledge", statePath, err)
		return empty
	}

	var mem memoryFile
	if err := json.Unmarshal(data, &mem); err != nil {
		logMemory(log, "knowledge file beside %s is unreadable (%v); starting with empty knowledge", statePath, err)
		return empty
	}
	if mem.Version != memoryVersion && !(migratingV4 && mem.Version == legacyMemoryVersion) {
		logMemory(log, "knowledge file %s is version %d, want %d (or migratable v%d); starting with empty knowledge",
			path, mem.Version, memoryVersion, legacyMemoryVersion)
		return empty
	}
	if len(mem.Intent) > IntentCap {
		logMemory(log, "knowledge file beside %s carries an intent of %d bytes, over the cap of %d; starting with empty knowledge",
			statePath, len(mem.Intent), IntentCap)
		return empty
	}
	if err := validateStoredPlan(mem.Plan); err != nil {
		logMemory(log, "knowledge file beside %s carries an invalid plan (%v); starting with empty knowledge", statePath, err)
		return empty
	}
	if migratingV4 {
		logMemory(log, "migrating v4 presentation-keyed checkpoint memory beside %s; legacy plan sentences remain valid until the next strategic plan", statePath)
	}
	k := NewKnowledge(adjacency)
	k.restore(mem)
	return ResumedMemory{Knowledge: k, Intent: mem.Intent, IntentAge: mem.IntentAge, Plan: mem.Plan.clone()}
}

func logMemory(log io.Writer, format string, args ...any) {
	if log != nil {
		fmt.Fprintf(log, "agent: memory: "+format+"\n", args...)
	}
}
