package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
)

const (
	// periodicCheckpointFrames is five minutes at 60 fps.
	periodicCheckpointFrames = 18_000
	// periodicCheckpointKeep is the local flight-recorder window.
	periodicCheckpointKeep  = 12
	checkpointUploadTimeout = 2 * time.Second
	farmFinishTimeout       = 30 * time.Second

	// A major checkpoint is the first ordinary objective checkpoint whose
	// paired agent knowledge says another gym leader has been beaten. Because
	// objective checkpoints are taken before the NEXT objective, that pair is
	// post-badge: the emulator state already holds the badge and the knowledge
	// already records the successful gym objective. The wall keeps these on a
	// separate, much longer-lived ring for endless-run recovery.
	majorCheckpointStatePrefix  = "major-badge-"
	majorCheckpointMarkerPrefix = ".major-badge-"
	gymCompletionObjective      = "beat the gym leader here"
)

type periodicSample struct {
	Name                      string
	State                     []byte
	Meta                      []byte
	Frame                     uint64
	Map                       uint8
	X, Y                      uint8
	Question, Decision, Trace string
}

type periodicMeta struct {
	Frame                     uint64 `json:"frame"`
	Map                       uint8  `json:"map"`
	X                         uint8  `json:"x"`
	Y                         uint8  `json:"y"`
	Question                  string `json:"question,omitempty"`
	Decision                  string `json:"decision,omitempty"`
	Trace                     string `json:"trace,omitempty"`
	LatestObjectiveCheckpoint string `json:"latest_objective_checkpoint,omitempty"`
}

type checkpointKnowledgeProgress struct {
	Completed []struct {
		Objective string
		Times     int
	} `json:"completed"`
}

func periodicStateName(frame uint64) string {
	return fmt.Sprintf("periodic-%010d.state", frame)
}

func enqueuePeriodic(ch chan periodicSample, s periodicSample) {
	select {
	case ch <- s:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- s:
	default:
	}
}

func maybeCapturePeriodic(m *emu.Emu, snap *heartbeatSnap, ch chan periodicSample) {
	if ch == nil {
		return
	}
	frame := m.FrameCount()
	if frame == 0 || frame%periodicCheckpointFrames != 0 {
		return
	}
	st, err := m.SaveState()
	if err != nil {
		return
	}
	hb := farm.Heartbeat{}
	if snap != nil {
		hb = snap.load()
	}
	enqueuePeriodic(ch, periodicSample{
		Name:     periodicStateName(frame),
		State:    append([]byte(nil), st...),
		Frame:    frame,
		Map:      hb.Map,
		X:        hb.X,
		Y:        hb.Y,
		Question: hb.Question,
		Decision: hb.Decision,
		Trace:    hb.Trace,
	})
}

func runCheckpointUploader(client *farm.Client, runID string, attempt int, dir string, samples <-chan periodicSample, stop <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		// A resumed attempt may materialize a major checkpoint from an older
		// attempt into this directory. Seed its marker before scanning new
		// round checkpoints so the same badge is not promoted again later in
		// the stage and accidentally drift the rollback point toward a fault.
		seedMajorPromotionMarkers(dir)
		uploaded := map[string]struct{}{}
		for {
			select {
			case <-stop:
				return
			case s := <-samples:
				writeAndUploadPeriodic(client, runID, attempt, dir, s)
				_ = evictPeriodicCheckpoints(dir, periodicCheckpointKeep)
			case <-time.After(200 * time.Millisecond):
				uploadNewObjectivePairs(client, runID, attempt, dir, uploaded)
			}
		}
	}()
	return done
}

func writeAndUploadPeriodic(client *farm.Client, runID string, attempt int, dir string, s periodicSample) {
	if dir == "" || s.Name == "" {
		return
	}
	latest := latestObjectiveState(dir)
	meta, _ := json.Marshal(periodicMeta{
		Frame:                     s.Frame,
		Map:                       s.Map,
		X:                         s.X,
		Y:                         s.Y,
		Question:                  s.Question,
		Decision:                  s.Decision,
		Trace:                     s.Trace,
		LatestObjectiveCheckpoint: latest,
	})
	if len(s.Meta) > 0 {
		meta = s.Meta
	}
	base := strings.TrimSuffix(s.Name, ".state")
	statePath := filepath.Join(dir, s.Name)
	metaPath := filepath.Join(dir, base+".json")
	if err := os.WriteFile(statePath, s.State, 0o644); err != nil {
		log.Printf("farm: %s: write periodic state: %v", runID, err)
		return
	}
	if err := os.WriteFile(metaPath, meta, 0o644); err != nil {
		log.Printf("farm: %s: write periodic meta: %v", runID, err)
		return
	}
	arts, err := artifactsForFiles([]string{filepath.Base(metaPath), s.Name}, dir)
	if err != nil {
		return
	}
	uploadCheckpoint(client, runID, attempt, arts)
}

func uploadNewObjectivePairs(client *farm.Client, runID string, attempt int, dir string, uploaded map[string]struct{}) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	knowledge := map[string]string{}
	var states []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".state") && !strings.HasPrefix(name, "periodic-") && !strings.HasPrefix(name, majorCheckpointStatePrefix) {
			states = append(states, name)
			continue
		}
		if strings.Contains(name, "knowledge-v") && strings.HasSuffix(name, ".json") {
			base := knowledgeBase(name)
			knowledge[base+".state"] = name
		}
	}
	sort.Strings(states)
	for _, st := range states {
		if _, ok := uploaded[st]; ok {
			continue
		}
		kn, ok := knowledge[st]
		if !ok {
			continue
		}
		arts, err := artifactsForFiles([]string{kn, st}, dir)
		if err != nil {
			continue
		}
		if err := uploadCheckpoint(client, runID, attempt, arts); err != nil {
			continue
		}

		badge, majorState, majorKnowledge, err := promoteBadgeCheckpointFiles(dir, st, kn)
		if err != nil {
			uploaded[st] = struct{}{}
			log.Printf("farm: %s: promote major checkpoint from %s: %v", runID, st, err)
			continue
		}
		if badge == 0 {
			uploaded[st] = struct{}{}
			continue
		}
		majorArts, err := artifactsForFiles([]string{majorKnowledge, majorState}, dir)
		if err != nil {
			log.Printf("farm: %s: read major badge %d checkpoint: %v", runID, badge, err)
			return
		}
		if err := uploadCheckpoint(client, runID, attempt, majorArts); err != nil {
			// Do not mark the ordinary source as fully uploaded yet. Return
			// immediately so a later post-badge state in this same scan cannot
			// steal the milestone while the wall is transiently unavailable.
			return
		}
		if err := os.WriteFile(majorPromotionMarker(dir, badge), []byte(st+"\n"), 0o644); err != nil {
			log.Printf("farm: %s: mark major badge %d checkpoint: %v", runID, badge, err)
			return
		}
		uploaded[st] = struct{}{}
	}
}

func gymCompletionCount(data []byte) (int, error) {
	var mem checkpointKnowledgeProgress
	if err := json.Unmarshal(data, &mem); err != nil {
		return 0, err
	}
	badges := 0
	for _, c := range mem.Completed {
		if c.Objective == gymCompletionObjective && c.Times > badges {
			badges = c.Times
		}
	}
	if badges < 0 || badges > 8 {
		return 0, fmt.Errorf("gym completion count %d outside badge range", badges)
	}
	return badges, nil
}

func promoteBadgeCheckpointFiles(dir, stateName, knowledgeName string) (badge int, majorState, majorKnowledge string, err error) {
	knowledgeData, err := os.ReadFile(filepath.Join(dir, knowledgeName))
	if err != nil {
		return 0, "", "", err
	}
	badge, err = gymCompletionCount(knowledgeData)
	if err != nil || badge == 0 {
		return badge, "", "", err
	}
	if _, err := os.Stat(majorPromotionMarker(dir, badge)); err == nil {
		return 0, "", "", nil
	} else if !os.IsNotExist(err) {
		return 0, "", "", err
	}

	sourceBase := strings.TrimSuffix(stateName, ".state")
	knowledgeSuffix := strings.TrimPrefix(knowledgeName, sourceBase)
	if sourceBase == stateName || !strings.HasPrefix(knowledgeSuffix, ".knowledge-v") || !strings.HasSuffix(knowledgeSuffix, ".json") {
		return 0, "", "", fmt.Errorf("checkpoint pair does not share base: %s / %s", stateName, knowledgeName)
	}
	majorBase := fmt.Sprintf("%s%02d-%s", majorCheckpointStatePrefix, badge, sourceBase)
	majorState = majorBase + ".state"
	majorKnowledge = majorBase + knowledgeSuffix
	stateData, err := os.ReadFile(filepath.Join(dir, stateName))
	if err != nil {
		return 0, "", "", err
	}
	// Write the knowledge first and state last. A reader only considers a
	// checkpoint once its .state exists, so a crash cannot expose a state
	// whose paired knowledge was never fully copied.
	if err := os.WriteFile(filepath.Join(dir, majorKnowledge), knowledgeData, 0o644); err != nil {
		return 0, "", "", err
	}
	if err := os.WriteFile(filepath.Join(dir, majorState), stateData, 0o644); err != nil {
		_ = os.Remove(filepath.Join(dir, majorKnowledge))
		return 0, "", "", err
	}
	return badge, majorState, majorKnowledge, nil
}

func majorPromotionMarker(dir string, badge int) string {
	return filepath.Join(dir, fmt.Sprintf("%s%02d.promoted", majorCheckpointMarkerPrefix, badge))
}

func seedMajorPromotionMarkers(dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		badge, ok := majorBadgeFromStateName(e.Name())
		if !ok {
			continue
		}
		marker := majorPromotionMarker(dir, badge)
		if _, err := os.Stat(marker); err == nil {
			continue
		}
		_ = os.WriteFile(marker, []byte(e.Name()+"\n"), 0o644)
	}
}

func majorBadgeFromStateName(name string) (int, bool) {
	if !strings.HasPrefix(name, majorCheckpointStatePrefix) || !strings.HasSuffix(name, ".state") {
		return 0, false
	}
	rest := strings.TrimPrefix(name, majorCheckpointStatePrefix)
	i := strings.IndexByte(rest, '-')
	if i <= 0 {
		return 0, false
	}
	badge, err := strconv.Atoi(rest[:i])
	return badge, err == nil && badge >= 1 && badge <= 8
}

func uploadCheckpoint(client *farm.Client, runID string, attempt int, arts []farm.Artifact) error {
	if client == nil || len(arts) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkpointUploadTimeout)
	defer cancel()
	return client.Checkpoint(ctx, farm.CheckpointReport{RunID: runID, Attempt: attempt, Artifacts: arts})
}

func evictPeriodicCheckpoints(dir string, keep int) error {
	if dir == "" || keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var states []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "periodic-") && strings.HasSuffix(e.Name(), ".state") {
			states = append(states, e.Name())
		}
	}
	sort.Strings(states)
	drop := len(states) - keep
	if drop <= 0 {
		return nil
	}
	for _, name := range states[:drop] {
		base := strings.TrimSuffix(name, ".state")
		_ = os.Remove(filepath.Join(dir, name))
		_ = os.Remove(filepath.Join(dir, base+".json"))
	}
	return nil
}

func latestObjectiveState(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var states []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".state") && !strings.HasPrefix(name, "periodic-") && !strings.HasPrefix(name, majorCheckpointStatePrefix) {
			states = append(states, name)
		}
	}
	if len(states) == 0 {
		return ""
	}
	sort.Strings(states)
	return states[len(states)-1]
}

func collectCheckpointArtifacts(dir string) ([]farm.Artifact, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := map[string]struct{}{}
	var states []string
	var knowledge []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		files[name] = struct{}{}
		switch {
		case strings.HasSuffix(name, ".state"):
			states = append(states, name)
		case strings.Contains(name, "knowledge-v") && strings.HasSuffix(name, ".json"):
			knowledge = append(knowledge, name)
		}
	}
	sort.Strings(states)
	paired := map[string]struct{}{}
	var want []string
	for _, st := range states {
		if err := checkArtifactName(st); err != nil {
			return nil, err
		}
		base := strings.TrimSuffix(st, ".state")
		if strings.HasPrefix(st, "periodic-") {
			meta := base + ".json"
			if _, ok := files[meta]; !ok {
				return nil, fmt.Errorf("farm: missing pair for %s", st)
			}
			if err := checkArtifactName(meta); err != nil {
				return nil, err
			}
			want = append(want, meta, st)
			paired[meta] = struct{}{}
			continue
		}
		kn := findKnowledge(base, knowledge)
		if kn == "" {
			return nil, fmt.Errorf("farm: missing pair for %s", st)
		}
		if err := checkArtifactName(kn); err != nil {
			return nil, err
		}
		want = append(want, kn, st)
		paired[kn] = struct{}{}
	}
	for _, kn := range knowledge {
		if _, ok := paired[kn]; !ok {
			return nil, fmt.Errorf("farm: orphan knowledge %s", kn)
		}
	}
	arts, err := artifactsForFiles(want, dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Name < arts[j].Name })
	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: arts}); err != nil {
		return nil, err
	}
	return arts, nil
}

func artifactsForFiles(names []string, dir string) ([]farm.Artifact, error) {
	out := make([]farm.Artifact, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		out = append(out, farm.Artifact{
			Name:      name,
			MediaType: artifactMediaType(name),
			SHA256:    hex.EncodeToString(sum[:]),
			Data:      data,
		})
	}
	return out, nil
}

func artifactMediaType(name string) string {
	if strings.HasSuffix(name, ".json") {
		return "application/json"
	}
	return "application/octet-stream"
}

func findKnowledge(base string, names []string) string {
	prefix := base + ".knowledge-v"
	for _, n := range names {
		if strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ".json") {
			return n
		}
	}
	return ""
}

func knowledgeBase(name string) string {
	trimmed := strings.TrimSuffix(name, ".json")
	if i := strings.LastIndex(trimmed, ".knowledge-v"); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}

func checkArtifactName(name string) error {
	if name == "" || name == "." || name == ".." || path.Base(name) != name {
		return fmt.Errorf("farm: unsafe artifact name %q", name)
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("farm: unsafe artifact name %q", name)
		}
	}
	return nil
}

func removeCheckpointDir(dir string) {
	if dir == "" {
		return
	}
	if !strings.HasPrefix(filepath.Base(dir), "pokefarm-checkpoints-") {
		return
	}
	_ = os.RemoveAll(dir)
}

func sendFinish(client *farm.Client, report farm.FinishReport, checkpointDir string) {
	defer removeCheckpointDir(checkpointDir)
	arts, err := collectCheckpointArtifacts(checkpointDir)
	if err != nil {
		log.Printf("farm: %s: collect checkpoints: %v", report.RunID, err)
	} else {
		report.Artifacts = arts
	}
	ctx, cancel := context.WithTimeout(context.Background(), farmFinishTimeout)
	defer cancel()
	if err := client.Finish(ctx, report); err != nil {
		log.Printf("farm: %s: finish: %v", report.RunID, err)
		return
	}
	fmt.Printf("run %s finished: %s\n", report.RunID, report.Reason)
}

// finishLeasedRun is the test-facing Finish+cleanup path that does not
// need an emulator: it still collects the checkpoint directory and
// removes it only after Finish returns.
func finishLeasedRun(_ *emu.Emu, client *farm.Client, spec farm.Spec, reason, detail string, burn int, checkpointDir string) {
	report := farm.FinishReport{
		RunID:         spec.RunID,
		Attempt:       spec.Attempt,
		Reason:        reason,
		Detail:        detail,
		RunnerVersion: client.Version,
		SeedBurn:      burn,
	}
	sendFinish(client, report, checkpointDir)
}
