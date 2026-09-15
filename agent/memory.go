package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Version 6 replaces native uint8 map identity with semantic LocationID for
// durable visited/talked evidence. v5 and v4 remain explicitly migratable: v5
// already carries ObjectiveKey records, while v4 may still use presentation
// strings for objective identity.
const (
	memoryVersion              = 6
	legacyObjectiveKeyVersion  = 5
	legacyPresentationVersion  = 4
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
	Objective string       `json:"objective,omitempty"`
	Times     int          `json:"times"`
}

type storedFailure struct {
	Key       ObjectiveKey `json:"key,omitempty"`
	Mode      string       `json:"mode,omitempty"`
	Objective string       `json:"objective"`
	Times     int          `json:"times"`
	Last      string       `json:"last"`
}

// memoryFile is the v6 serialised form. Adjacency and native-map translation
// are rebuilt from the active adapter/ROM on resume and are never persisted.
type memoryFile struct {
	Version      int                `json:"version"`
	Visited      []LocationID       `json:"visited"`
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
	Location LocationID `json:"location"`
	X        uint8      `json:"x"`
	Y        uint8      `json:"y"`
}

// legacyMemoryFile matches v4/v5 geography. Objective records intentionally
// reuse the current structures because they already contain both canonical and
// presentation compatibility fields.
type legacyMemoryFile struct {
	Version      int                `json:"version"`
	Visited      []uint8            `json:"visited"`
	Places       []string           `json:"places"`
	Completed    []storedCompletion `json:"completed"`
	Talked       []legacyTalkedKey  `json:"talked"`
	Requirements []Requirement      `json:"requirements,omitempty"`
	Failures     []storedFailure    `json:"failures,omitempty"`
	Intent       string             `json:"intent,omitempty"`
	IntentAge    int                `json:"intent_age,omitempty"`
	Plan         Plan               `json:"plan,omitempty"`
}

type legacyTalkedKey struct {
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
	sort.Slice(mem.Visited, func(i, j int) bool { return mem.Visited[i] < mem.Visited[j] })

	for name := range k.Places {
		mem.Places = append(mem.Places, name)
	}
	sort.Strings(mem.Places)

	completionKeys := make([]string, 0, len(k.Completed))
	for storage := range k.Completed {
		completionKeys = append(completionKeys, storage)
	}
	sort.Strings(completionKeys)
	for _, storage := range completionKeys {
		entry := storedCompletion{Times: k.Completed[storage]}
		if key, ok := parseObjectiveKeyID(storage); ok {
			entry.Key = key
		} else {
			entry.Objective = storage
		}
		mem.Completed = append(mem.Completed, entry)
	}

	for location, tiles := range k.Talked {
		for tile := range tiles {
			mem.Talked = append(mem.Talked, talkedKey{Location: location, X: tile[0], Y: tile[1]})
		}
	}
	sort.Slice(mem.Talked, func(i, j int) bool {
		if mem.Talked[i].Location != mem.Talked[j].Location {
			return mem.Talked[i].Location < mem.Talked[j].Location
		}
		if mem.Talked[i].Y != mem.Talked[j].Y {
			return mem.Talked[i].Y < mem.Talked[j].Y
		}
		return mem.Talked[i].X < mem.Talked[j].X
	})

	mem.Requirements = append(mem.Requirements, k.Requirements...)

	failureKeys := make([]string, 0, len(k.Failures))
	for storage := range k.Failures {
		failureKeys = append(failureKeys, storage)
	}
	sort.Strings(failureKeys)
	for _, storage := range failureKeys {
		failure := k.Failures[storage]
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

func migrateLegacyMemory(legacy legacyMemoryFile, k *Knowledge) memoryFile {
	mem := memoryFile{
		Version:      memoryVersion,
		Places:       append([]string(nil), legacy.Places...),
		Completed:    append([]storedCompletion(nil), legacy.Completed...),
		Requirements: append([]Requirement(nil), legacy.Requirements...),
		Failures:     append([]storedFailure(nil), legacy.Failures...),
		Intent:       legacy.Intent,
		IntentAge:    legacy.IntentAge,
		Plan:         legacy.Plan.clone(),
	}
	for _, native := range legacy.Visited {
		mem.Visited = append(mem.Visited, k.locationForNative(native))
	}
	for _, talked := range legacy.Talked {
		mem.Talked = append(mem.Talked, talkedKey{Location: k.locationForNative(talked.Map), X: talked.X, Y: talked.Y})
	}
	return mem
}

func LoadCheckpointMemory(statePath string, topology any, log io.Writer) ResumedMemory {
	empty := ResumedMemory{Knowledge: NewKnowledge(topology)}
	base := filepath.Base(statePath)
	if !strings.HasSuffix(base, ".state") {
		logMemory(log, "%s is not a checkpoint .state file; starting with empty knowledge", statePath)
		return empty
	}

	path := knowledgePathForState(statePath)
	data, err := os.ReadFile(path)
	version := memoryVersion
	if errors.Is(err, os.ErrNotExist) {
		for _, legacyVersion := range []int{legacyObjectiveKeyVersion, legacyPresentationVersion} {
			legacyPath := knowledgePathForStateVersion(statePath, legacyVersion)
			legacy, legacyErr := os.ReadFile(legacyPath)
			if legacyErr == nil {
				data, err, path, version = legacy, nil, legacyPath, legacyVersion
				break
			}
		}
	}
	if err != nil {
		logMemory(log, "no readable knowledge file beside %s (%v); starting with empty knowledge", statePath, err)
		return empty
	}

	k := NewKnowledge(topology)
	var mem memoryFile
	if version == memoryVersion {
		if err := json.Unmarshal(data, &mem); err != nil {
			logMemory(log, "knowledge file beside %s is unreadable (%v); starting with empty knowledge", statePath, err)
			return empty
		}
		if mem.Version != memoryVersion {
			logMemory(log, "knowledge file %s is version %d, want %d; starting with empty knowledge", path, mem.Version, memoryVersion)
			return empty
		}
	} else {
		var legacy legacyMemoryFile
		if err := json.Unmarshal(data, &legacy); err != nil {
			logMemory(log, "legacy knowledge file beside %s is unreadable (%v); starting with empty knowledge", statePath, err)
			return empty
		}
		if legacy.Version != version {
			logMemory(log, "knowledge file %s is version %d, expected legacy v%d; starting with empty knowledge", path, legacy.Version, version)
			return empty
		}
		mem = migrateLegacyMemory(legacy, k)
		logMemory(log, "migrating v%d checkpoint geography to semantic locations beside %s", version, statePath)
	}

	if len(mem.Intent) > IntentCap {
		logMemory(log, "knowledge file beside %s carries an intent of %d bytes, over the cap of %d; starting with empty knowledge", statePath, len(mem.Intent), IntentCap)
		return empty
	}
	if err := validateStoredPlan(mem.Plan); err != nil {
		logMemory(log, "knowledge file beside %s carries an invalid plan (%v); starting with empty knowledge", statePath, err)
		return empty
	}
	k.restore(mem)
	return ResumedMemory{Knowledge: k, Intent: mem.Intent, IntentAge: mem.IntentAge, Plan: mem.Plan.clone()}
}

func logMemory(log io.Writer, format string, args ...any) {
	if log != nil {
		fmt.Fprintf(log, "agent: memory: "+format+"\n", args...)
	}
}
