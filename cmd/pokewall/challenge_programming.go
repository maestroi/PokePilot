package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/farm"
)

const (
	challengeProgrammingStateFile = "challenge-programming.json"
	challengeCapabilityVersion    = "2026-09-28.1"

	programStateScheduled = "scheduled"
	programStateVoting    = "voting"
	programStateQueued    = "queued"
	programStateStarting  = "starting"
	programStateLive      = "live"
	programStateCompleted = "completed"
	programStateFailed    = "failed"
	programStateBlocked   = "blocked"
	programStateSkipped   = "skipped"
	programStateCancelled = "cancelled"
)

var challengeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,95}$`)

type challengeCapability struct {
	Game              string   `json:"game"`
	CapabilityVersion string   `json:"capability_version"`
	Planners          []string `json:"planners"`
	PlayStyles        []string `json:"play_styles,omitempty"`
	StarterPolicy     string   `json:"starter_policy"`
	Features          []string `json:"features"`
}

type challengeDefinition struct {
	ID            string    `json:"id"`
	Version       int       `json:"version"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Run           farm.Spec `json:"run,omitzero"`
	Requirements  []string  `json:"requirements,omitempty"`
	ExperimentRef string    `json:"experiment_ref,omitempty"`
	Ephemeral     bool      `json:"ephemeral,omitempty"`
	CreatedAt     int64     `json:"created_at"`
}

type challengeRunLink struct {
	EntryID          string `json:"entry_id"`
	ChallengeID      string `json:"challenge_id"`
	ChallengeVersion int    `json:"challenge_version"`
	ExperimentID     string `json:"experiment_id,omitempty"`
}

type challengeQueueEntry struct {
	ID               string   `json:"id"`
	ChallengeID      string   `json:"challenge_id"`
	ChallengeVersion int      `json:"challenge_version"`
	ChallengeName    string   `json:"challenge_name"`
	State            string   `json:"state"`
	ScheduledAt      int64    `json:"scheduled_at,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	StartedAt        int64    `json:"started_at,omitempty"`
	EndedAt          int64    `json:"ended_at,omitempty"`
	RunIDs           []string `json:"run_ids,omitempty"`
	ExperimentID     string   `json:"experiment_id,omitempty"`
	Result           string   `json:"result,omitempty"`
	Error            string   `json:"error,omitempty"`
	Pinned           bool     `json:"pinned,omitempty"`
}

type challengeProgrammingState struct {
	Challenges map[string][]challengeDefinition `json:"challenges"`
	Entries    []challengeQueueEntry            `json:"entries"`
	RunLinks   map[string]challengeRunLink      `json:"run_links"`
	Paused     bool                             `json:"paused"`
}

type challengeProgrammingSnapshot struct {
	Paused  bool                  `json:"paused"`
	LiveNow *challengeQueueEntry  `json:"live_now,omitempty"`
	UpNext  *challengeQueueEntry  `json:"up_next,omitempty"`
	Queue   []challengeQueueEntry `json:"queue"`
}

type challengeProgrammingController struct {
	wall *Wall

	mu        sync.Mutex
	advanceMu sync.Mutex
	loaded    bool
	loadErr   error
	state     challengeProgrammingState
	launcher  http.Handler
}

var challengeProgrammingControllers sync.Map // *Wall -> *challengeProgrammingController

func challengeProgrammingFor(w *Wall) *challengeProgrammingController {
	if existing, ok := challengeProgrammingControllers.Load(w); ok {
		c := existing.(*challengeProgrammingController)
		c.ensureLoaded()
		return c
	}
	c := &challengeProgrammingController{
		wall: w,
		state: challengeProgrammingState{
			Challenges: map[string][]challengeDefinition{},
			RunLinks:   map[string]challengeRunLink{},
		},
	}
	actual, _ := challengeProgrammingControllers.LoadOrStore(w, c)
	out := actual.(*challengeProgrammingController)
	out.ensureLoaded()
	return out
}

func (c *challengeProgrammingController) ensureLoaded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	c.loaded = true
	c.loadErr = c.loadLocked()
	if c.state.Challenges == nil {
		c.state.Challenges = map[string][]challengeDefinition{}
	}
	if c.state.RunLinks == nil {
		c.state.RunLinks = map[string]challengeRunLink{}
	}
	c.recoverLocked()
}

func challengeCapabilities() map[string]challengeCapability {
	commonPlayStyles := []string{"adventure", "completionist", "speedrun", "team_builder"}
	return map[string]challengeCapability{
		"pokemon-red": {
			Game: "pokemon-red", CapabilityVersion: challengeCapabilityVersion,
			Planners: []string{"llm", "scripted"}, PlayStyles: commonPlayStyles,
			StarterPolicy: "red_gen1",
			Features: []string{
				"decision_engine", "model_deployment", "pokemon.dex", "pokemon.items",
				"pokemon.starter.fixed_gen1", "pokemon.starter.random", "pokemon.starter.vanilla",
				"pokemon.story", "recovery.resilient",
			},
		},
		"pokemon-blue": {
			Game: "pokemon-blue", CapabilityVersion: challengeCapabilityVersion,
			Planners: []string{"llm", "scripted"}, PlayStyles: commonPlayStyles,
			StarterPolicy: "vanilla_gen1",
			Features:      []string{"decision_engine", "model_deployment", "pokemon.dex", "pokemon.items", "pokemon.starter.vanilla", "pokemon.story", "recovery.resilient"},
		},
		"pokemon-yellow": {
			Game: "pokemon-yellow", CapabilityVersion: challengeCapabilityVersion,
			Planners: []string{"llm", "scripted"}, PlayStyles: commonPlayStyles,
			StarterPolicy: "pikachu_scripted",
			Features:      []string{"decision_engine", "model_deployment", "pokemon.dex", "pokemon.items", "pokemon.starter.pikachu", "pokemon.story", "recovery.resilient"},
		},
		"tetris": {
			Game: "tetris", CapabilityVersion: challengeCapabilityVersion,
			Planners: []string{"policy"}, StarterPolicy: "none",
			Features: []string{"deterministic_policy", "score_goal", "survival_goal"},
		},
		"boxxle": {
			Game: "boxxle", CapabilityVersion: challengeCapabilityVersion,
			Planners: []string{"launch"}, StarterPolicy: "none",
			Features: []string{"deterministic_solver", "puzzle_goal"},
		},
	}
}

func sortedCapabilities() []challengeCapability {
	registry := challengeCapabilities()
	out := make([]challengeCapability, 0, len(registry))
	for _, cap := range registry {
		cap.Planners = append([]string(nil), cap.Planners...)
		cap.PlayStyles = append([]string(nil), cap.PlayStyles...)
		cap.Features = append([]string(nil), cap.Features...)
		sort.Strings(cap.Planners)
		sort.Strings(cap.PlayStyles)
		sort.Strings(cap.Features)
		out = append(out, cap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Game < out[j].Game })
	return out
}

func stringInList(value string, values []string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizeChallengeDefinition(in challengeDefinition) (challengeDefinition, error) {
	in.ID = strings.ToLower(strings.TrimSpace(in.ID))
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.ExperimentRef = strings.TrimSpace(in.ExperimentRef)
	if !challengeIDPattern.MatchString(in.ID) {
		return challengeDefinition{}, errors.New("challenge id must be a lowercase slug using letters, digits, dot, underscore, or hyphen")
	}
	if in.Name == "" {
		return challengeDefinition{}, errors.New("challenge name is required")
	}
	in.Requirements = append([]string(nil), in.Requirements...)
	for i := range in.Requirements {
		in.Requirements[i] = strings.TrimSpace(in.Requirements[i])
		if in.Requirements[i] == "" {
			return challengeDefinition{}, errors.New("challenge requirements may not contain empty values")
		}
	}
	sort.Strings(in.Requirements)
	in.Requirements = compactStrings(in.Requirements)
	if in.ExperimentRef != "" {
		if runSpecConfigured(in.Run) {
			return challengeDefinition{}, errors.New("challenge must define either run or experiment_ref, not both")
		}
		return in, nil
	}
	if !runSpecConfigured(in.Run) {
		return challengeDefinition{}, errors.New("challenge run configuration or experiment_ref is required")
	}
	if err := validateChallengeRun(&in.Run, in.Requirements); err != nil {
		return challengeDefinition{}, err
	}
	return in, nil
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:0]
	last := ""
	for _, value := range values {
		if len(out) > 0 && value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

func runSpecConfigured(spec farm.Spec) bool {
	return spec.RunID != "" || spec.Game != "" || spec.Planner != "" || spec.Starter != "" ||
		spec.Dest != "" || spec.Goal.String() != "" || spec.PlayStyle != "" || spec.Purpose != "" ||
		spec.RiskTolerance != "" || spec.WildEncounters != "" || spec.LLMProfile != "" ||
		spec.LLMDeployment != "" || spec.DecisionEngine != nil || spec.FPS != 0 || spec.MaxRounds != 0 ||
		spec.MaxFrames != 0 || spec.RecoveryProfile != "" || spec.Endless || spec.RandomSeed || spec.Seed != 0
}

func validateChallengeRun(spec *farm.Spec, requirements []string) error {
	if spec == nil {
		return errors.New("challenge run configuration is required")
	}
	if strings.TrimSpace(spec.RunID) != "" {
		return errors.New("challenge run_id must be empty; the programming queue creates a fresh run id")
	}
	if spec.Endless {
		return errors.New("challenge runs may not set endless; the programming queue owns continuation")
	}
	spec.Game = strings.ToLower(strings.TrimSpace(spec.Game))
	if spec.Game == "" {
		return errors.New("challenge run game is required")
	}
	cap, ok := challengeCapabilities()[spec.Game]
	if !ok {
		return fmt.Errorf("game %q has no challenge capability metadata", spec.Game)
	}
	spec.Planner = strings.ToLower(strings.TrimSpace(spec.Planner))
	if spec.Planner == "" {
		switch spec.Game {
		case "tetris":
			spec.Planner = "policy"
		case "boxxle":
			spec.Planner = "launch"
		default:
			spec.Planner = "llm"
		}
	}
	if !stringInList(spec.Planner, cap.Planners) {
		return fmt.Errorf("game %s does not support challenge planner %q; supported: %s", spec.Game, spec.Planner, strings.Join(cap.Planners, ", "))
	}
	spec.PlayStyle = strings.ToLower(strings.TrimSpace(spec.PlayStyle))
	if spec.PlayStyle != "" && !stringInList(spec.PlayStyle, cap.PlayStyles) {
		return fmt.Errorf("game %s does not support play_style %q", spec.Game, spec.PlayStyle)
	}
	if !spec.Purpose.Valid() {
		return errors.New("purpose must be normal or debug_coverage")
	}
	if !spec.RecoveryProfile.Valid() {
		return errors.New("recovery_profile must be strict or resilient")
	}
	decisionEngine, err := spec.DecisionEngine.Normalized()
	if err != nil {
		return err
	}
	spec.DecisionEngine = decisionEngine
	if err := validateChallengeStarter(spec, cap); err != nil {
		return err
	}
	available := make(map[string]bool, len(cap.Features))
	for _, feature := range cap.Features {
		available[feature] = true
	}
	for _, requirement := range requirements {
		if !available[requirement] {
			return fmt.Errorf("game %s capability %s does not provide required feature %q", spec.Game, cap.CapabilityVersion, requirement)
		}
	}
	return nil
}

func validateChallengeStarter(spec *farm.Spec, cap challengeCapability) error {
	starter := strings.ToLower(strings.TrimSpace(spec.Starter))
	switch cap.StarterPolicy {
	case "none":
		if starter != "" {
			return fmt.Errorf("game %s does not use a starter, got %q", spec.Game, spec.Starter)
		}
		spec.Starter = ""
	case "pikachu_scripted":
		if starter != "" && starter != "pikachu" {
			return fmt.Errorf("pokemon-yellow uses the scripted Pikachu starter, got %q", spec.Starter)
		}
		spec.Starter = "pikachu"
	case "vanilla_gen1":
		if starter != "" && starter != "bulbasaur" && starter != "charmander" && starter != "squirtle" {
			return fmt.Errorf("%s challenge starters are limited to vanilla Bulbasaur, Charmander, or Squirtle", spec.Game)
		}
		spec.Starter = starter
	case "red_gen1":
		if strings.HasPrefix(starter, "random:") {
			pool := strings.TrimPrefix(starter, "random:")
			if pool != "reasonable" && pool != "basic" && pool != "any" {
				return fmt.Errorf("pokemon-red random starter pool %q is unsupported; want reasonable, basic, or any", pool)
			}
		}
		spec.Starter = starter
	}
	return nil
}

func (c *challengeProgrammingController) challengeLocked(id string, version int) (challengeDefinition, bool) {
	versions := c.state.Challenges[id]
	if len(versions) == 0 {
		return challengeDefinition{}, false
	}
	if version <= 0 {
		return versions[len(versions)-1], true
	}
	for _, challenge := range versions {
		if challenge.Version == version {
			return challenge, true
		}
	}
	return challengeDefinition{}, false
}

func (c *challengeProgrammingController) saveChallenge(in challengeDefinition) (challengeDefinition, error) {
	normalized, err := normalizeChallengeDefinition(in)
	if err != nil {
		return challengeDefinition{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return challengeDefinition{}, c.loadErr
	}
	versions := c.state.Challenges[normalized.ID]
	if normalized.Version <= 0 {
		normalized.Version = len(versions) + 1
	}
	for _, existing := range versions {
		if existing.Version == normalized.Version {
			return challengeDefinition{}, fmt.Errorf("challenge %s version %d already exists", normalized.ID, normalized.Version)
		}
	}
	if len(versions) > 0 && normalized.Version <= versions[len(versions)-1].Version {
		return challengeDefinition{}, fmt.Errorf("challenge version must be newer than %d", versions[len(versions)-1].Version)
	}
	normalized.CreatedAt = time.Now().Unix()
	c.state.Challenges[normalized.ID] = append(versions, normalized)
	if err := c.persistLocked(); err != nil {
		c.state.Challenges[normalized.ID] = versions
		return challengeDefinition{}, err
	}
	return normalized, nil
}

type enqueueChallengeRequest struct {
	Version     int   `json:"version,omitempty"`
	ScheduledAt int64 `json:"scheduled_at,omitempty"`
	PinNext     bool  `json:"pin_next,omitempty"`
}

func (c *challengeProgrammingController) enqueueChallenge(id string, request enqueueChallengeRequest) (challengeQueueEntry, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	c.mu.Lock()
	challenge, ok := c.challengeLocked(id, request.Version)
	c.mu.Unlock()
	if !ok {
		return challengeQueueEntry{}, fmt.Errorf("challenge %s version %d not found", id, request.Version)
	}
	if _, err := normalizeChallengeDefinition(challenge); err != nil {
		return challengeQueueEntry{}, fmt.Errorf("challenge no longer validates: %w", err)
	}
	return c.enqueueDefinition(challenge, request)
}

func (c *challengeProgrammingController) enqueueDefinition(challenge challengeDefinition, request enqueueChallengeRequest) (challengeQueueEntry, error) {
	now := time.Now().Unix()
	state := programStateQueued
	if request.ScheduledAt > now {
		state = programStateScheduled
	}
	entry := challengeQueueEntry{
		ID:               "slot-" + strings.TrimPrefix(newRunID(), "run-"),
		ChallengeID:      challenge.ID,
		ChallengeVersion: challenge.Version,
		ChallengeName:    challenge.Name,
		State:            state,
		ScheduledAt:      request.ScheduledAt,
		CreatedAt:        now,
		Pinned:           request.PinNext,
	}
	c.mu.Lock()
	if request.PinNext {
		insert := len(c.state.Entries)
		for i, existing := range c.state.Entries {
			if isPendingProgramState(existing.State) {
				insert = i
				break
			}
		}
		c.state.Entries = append(c.state.Entries, challengeQueueEntry{})
		copy(c.state.Entries[insert+1:], c.state.Entries[insert:])
		c.state.Entries[insert] = entry
	} else {
		c.state.Entries = append(c.state.Entries, entry)
	}
	err := c.persistLocked()
	c.mu.Unlock()
	if err != nil {
		return challengeQueueEntry{}, err
	}
	c.advance()
	return entry, nil
}

func isPendingProgramState(state string) bool {
	return state == programStateQueued || state == programStateScheduled || state == programStateVoting
}

func isLiveProgramState(state string) bool {
	return state == programStateStarting || state == programStateLive
}

func (c *challengeProgrammingController) snapshot() challengeProgrammingSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := challengeProgrammingSnapshot{Paused: c.state.Paused, Queue: append([]challengeQueueEntry(nil), c.state.Entries...)}
	for i := range out.Queue {
		out.Queue[i].RunIDs = append([]string(nil), out.Queue[i].RunIDs...)
		if out.LiveNow == nil && isLiveProgramState(out.Queue[i].State) {
			copy := out.Queue[i]
			out.LiveNow = &copy
			continue
		}
	}
	for i := range out.Queue {
		if isPendingProgramState(out.Queue[i].State) {
			copy := out.Queue[i]
			out.UpNext = &copy
			break
		}
	}
	return out
}

func (c *challengeProgrammingController) listChallenges() []challengeDefinition {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []challengeDefinition
	for _, versions := range c.state.Challenges {
		for _, challenge := range versions {
			out = append(out, challenge)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version > out[j].Version
	})
	return out
}

func (c *challengeProgrammingController) setPaused(paused bool) error {
	c.mu.Lock()
	c.state.Paused = paused
	err := c.persistLocked()
	c.mu.Unlock()
	if err == nil && !paused {
		c.advance()
	}
	return err
}

func (c *challengeProgrammingController) reorder(ids []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := map[string]challengeQueueEntry{}
	var fixed []challengeQueueEntry
	for _, entry := range c.state.Entries {
		if isPendingProgramState(entry.State) {
			pending[entry.ID] = entry
		} else {
			fixed = append(fixed, entry)
		}
	}
	if len(ids) != len(pending) {
		return fmt.Errorf("reorder must name every pending entry exactly once")
	}
	ordered := make([]challengeQueueEntry, 0, len(pending))
	for _, id := range ids {
		entry, ok := pending[id]
		if !ok {
			return fmt.Errorf("entry %s is not pending or was repeated", id)
		}
		ordered = append(ordered, entry)
		delete(pending, id)
	}
	// Preserve already-terminal/live history in its current relative order and
	// put the newly ordered pending program after it.
	c.state.Entries = append(fixed, ordered...)
	return c.persistLocked()
}

func (c *challengeProgrammingController) mutateEntry(id, action string) (challengeQueueEntry, error) {
	now := time.Now().Unix()
	c.mu.Lock()
	index := -1
	for i := range c.state.Entries {
		if c.state.Entries[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		c.mu.Unlock()
		return challengeQueueEntry{}, fmt.Errorf("programming entry %s not found", id)
	}
	entry := &c.state.Entries[index]
	switch action {
	case "skip":
		if !isPendingProgramState(entry.State) {
			c.mu.Unlock()
			return challengeQueueEntry{}, fmt.Errorf("entry %s is not pending", id)
		}
		entry.State, entry.EndedAt, entry.Result = programStateSkipped, now, "skipped"
	case "cancel":
		if isPendingProgramState(entry.State) {
			entry.State, entry.EndedAt, entry.Result = programStateCancelled, now, "cancelled"
		} else if isLiveProgramState(entry.State) {
			for _, runID := range entry.RunIDs {
				c.wall.mu.Lock()
				tile := c.wall.tiles[runID]
				if tile != nil && !tile.Finished {
					c.wall.cancel[runID] = true
				}
				c.wall.mu.Unlock()
			}
			// Keep the slot live until every underlying run actually settles.
			// Otherwise the next programmed challenge could overlap a runner that
			// is still cooperatively cancelling.
			entry.Result = "cancelling"
		} else {
			c.mu.Unlock()
			return challengeQueueEntry{}, fmt.Errorf("entry %s cannot be cancelled from state %s", id, entry.State)
		}
	case "retry":
		if entry.State != programStateBlocked && entry.State != programStateFailed {
			c.mu.Unlock()
			return challengeQueueEntry{}, fmt.Errorf("entry %s is not blocked or failed", id)
		}
		entry.State = programStateQueued
		entry.StartedAt, entry.EndedAt = 0, 0
		entry.RunIDs = nil
		entry.ExperimentID, entry.Result, entry.Error = "", "", ""
	case "pin-next":
		if !isPendingProgramState(entry.State) {
			c.mu.Unlock()
			return challengeQueueEntry{}, fmt.Errorf("entry %s is not pending", id)
		}
		entry.Pinned = true
		copyEntry := *entry
		c.state.Entries = append(c.state.Entries[:index], c.state.Entries[index+1:]...)
		insert := len(c.state.Entries)
		for i, candidate := range c.state.Entries {
			if isPendingProgramState(candidate.State) {
				insert = i
				break
			}
		}
		c.state.Entries = append(c.state.Entries, challengeQueueEntry{})
		copy(c.state.Entries[insert+1:], c.state.Entries[insert:])
		c.state.Entries[insert] = copyEntry
		entry = &c.state.Entries[insert]
	default:
		c.mu.Unlock()
		return challengeQueueEntry{}, fmt.Errorf("unknown programming action %q", action)
	}
	out := *entry
	err := c.persistLocked()
	c.mu.Unlock()
	if err == nil && (action == "skip" || action == "cancel" || action == "pin-next" || action == "retry") {
		c.advance()
	}
	return out, err
}

func (c *challengeProgrammingController) nextDueLocked(now int64) int {
	if c.state.Paused {
		return -1
	}
	for _, entry := range c.state.Entries {
		if isLiveProgramState(entry.State) {
			return -1
		}
	}
	for i := range c.state.Entries {
		entry := &c.state.Entries[i]
		if !isPendingProgramState(entry.State) {
			continue
		}
		if entry.ScheduledAt > now {
			continue
		}
		return i
	}
	return -1
}

func (c *challengeProgrammingController) advance() {
	c.advanceMu.Lock()
	defer c.advanceMu.Unlock()

	for {
		c.mu.Lock()
		if c.launcher == nil || c.loadErr != nil {
			c.mu.Unlock()
			return
		}
		index := c.nextDueLocked(time.Now().Unix())
		if index < 0 {
			c.mu.Unlock()
			return
		}
		entry := c.state.Entries[index]
		challenge, ok := c.challengeLocked(entry.ChallengeID, entry.ChallengeVersion)
		if !ok {
			c.state.Entries[index].State = programStateBlocked
			c.state.Entries[index].Error = "challenge definition is missing"
			c.state.Entries[index].EndedAt = time.Now().Unix()
			_ = c.persistLocked()
			c.mu.Unlock()
			continue
		}
		c.state.Entries[index].State = programStateStarting
		c.state.Entries[index].StartedAt = time.Now().Unix()
		_ = c.persistLocked()
		c.mu.Unlock()

		runIDs, experimentID, err := c.launchChallenge(challenge)
		c.mu.Lock()
		current := -1
		for i := range c.state.Entries {
			if c.state.Entries[i].ID == entry.ID {
				current = i
				break
			}
		}
		if current < 0 {
			c.mu.Unlock()
			return
		}
		target := &c.state.Entries[current]
		if err != nil {
			target.State = programStateBlocked
			target.Error = err.Error()
			target.Result = "start_failed"
			target.EndedAt = time.Now().Unix()
			_ = c.persistLocked()
			c.mu.Unlock()
			continue
		}
		target.State = programStateLive
		target.RunIDs = append([]string(nil), runIDs...)
		target.ExperimentID = experimentID
		target.Error = ""
		for _, runID := range runIDs {
			c.state.RunLinks[runID] = challengeRunLink{
				EntryID: target.ID, ChallengeID: target.ChallengeID,
				ChallengeVersion: target.ChallengeVersion, ExperimentID: experimentID,
			}
		}
		_ = c.persistLocked()
		c.mu.Unlock()
		return
	}
}

func (c *challengeProgrammingController) launchChallenge(challenge challengeDefinition) ([]string, string, error) {
	if challenge.ExperimentRef != "" {
		return c.launchExperimentReference(challenge.ExperimentRef)
	}
	spec := challenge.Run
	if err := validateChallengeRun(&spec, challenge.Requirements); err != nil {
		return nil, "", err
	}
	c.wall.mu.Lock()
	for {
		spec.RunID = newRunID()
		if c.wall.tiles[spec.RunID] == nil {
			break
		}
	}
	c.wall.mu.Unlock()
	spec.Attempt = 0
	spec.Inference = nil
	spec.ExperimentID, spec.ExperimentArm, spec.ExperimentCase = "", "", ""
	spec.Endless = false
	body, _ := json.Marshal(spec)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/specs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c.launcher.ServeHTTP(recorder, request)
	if recorder.Code < 200 || recorder.Code >= 300 {
		return nil, "", fmt.Errorf("ordinary run enqueue failed: HTTP %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	return []string{spec.RunID}, "", nil
}

func (c *challengeProgrammingController) launchExperimentReference(reference string) ([]string, string, error) {
	value, ok := wallExperimentControllers.Load(c.wall)
	if !ok {
		return nil, "", errors.New("model experiment controller is unavailable")
	}
	controller := value.(*modelExperimentController)
	controller.mu.Lock()
	source, ok := controller.state.Experiments[reference]
	controller.mu.Unlock()
	if !ok {
		return nil, "", fmt.Errorf("experiment_ref %q was not found", reference)
	}
	request := source.Request
	request.Name = source.Name + " · queued"
	body, _ := json.Marshal(request)
	recorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/experiments", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/json")
	c.launcher.ServeHTTP(recorder, httpRequest)
	if recorder.Code < 200 || recorder.Code >= 300 {
		return nil, "", fmt.Errorf("experiment enqueue failed: HTTP %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.ID == "" {
		return nil, "", errors.New("experiment enqueue returned no experiment id")
	}
	controller.mu.Lock()
	created, ok := controller.state.Experiments[response.ID]
	controller.mu.Unlock()
	if !ok || len(created.RunIDs) == 0 {
		return nil, response.ID, errors.New("experiment enqueue returned no run ids")
	}
	return append([]string(nil), created.RunIDs...), response.ID, nil
}

func (c *challengeProgrammingController) finishRun(runID string) {
	c.advanceMu.Lock()

	c.mu.Lock()
	link, ok := c.state.RunLinks[runID]
	if !ok {
		c.mu.Unlock()
		c.advanceMu.Unlock()
		return
	}
	index := -1
	for i := range c.state.Entries {
		if c.state.Entries[i].ID == link.EntryID {
			index = i
			break
		}
	}
	if index < 0 || !isLiveProgramState(c.state.Entries[index].State) {
		c.mu.Unlock()
		c.advanceMu.Unlock()
		return
	}
	runIDs := append([]string(nil), c.state.Entries[index].RunIDs...)
	c.mu.Unlock()

	allTerminal := true
	result := "completed"
	var failures []string
	c.wall.mu.Lock()
	for _, id := range runIDs {
		tile := c.wall.tiles[id]
		if tile == nil || !tile.Finished {
			allTerminal = false
			continue
		}
		if tile.Reason != "" && tile.Reason != "done" && tile.Reason != "goal" && tile.Reason != "completed" {
			result = "failed"
			failures = append(failures, id+":"+tile.Reason)
		}
	}
	c.wall.mu.Unlock()
	if !allTerminal {
		c.advanceMu.Unlock()
		return
	}

	c.mu.Lock()
	for i := range c.state.Entries {
		if c.state.Entries[i].ID != link.EntryID {
			continue
		}
		entry := &c.state.Entries[i]
		if entry.Result == "cancelling" {
			entry.State, entry.Result = programStateCancelled, "cancelled"
		} else if result == "failed" {
			entry.State, entry.Result = programStateFailed, "failed"
			entry.Error = strings.Join(failures, ", ")
		} else {
			entry.State, entry.Result = programStateCompleted, "completed"
		}
		entry.EndedAt = time.Now().Unix()
		break
	}
	_ = c.persistLocked()
	c.mu.Unlock()
	c.advanceMu.Unlock()
	c.advance()
}

func (c *challengeProgrammingController) recoverLocked() {
	now := time.Now().Unix()
	for i := range c.state.Entries {
		entry := &c.state.Entries[i]
		if !isLiveProgramState(entry.State) {
			continue
		}
		if len(entry.RunIDs) == 0 {
			entry.State, entry.Result, entry.EndedAt = programStateBlocked, "restart_recovery", now
			entry.Error = "entry was starting during restart before run ids were recorded"
			continue
		}
		allTerminal := true
		c.wall.mu.Lock()
		for _, runID := range entry.RunIDs {
			tile := c.wall.tiles[runID]
			if tile == nil || !tile.Finished {
				allTerminal = false
				break
			}
		}
		c.wall.mu.Unlock()
		if allTerminal {
			entry.State, entry.Result, entry.EndedAt = programStateCompleted, "recovered_terminal", now
		} else {
			entry.State = programStateLive
		}
	}
}

func (c *challengeProgrammingController) runLink(runID string) (challengeRunLink, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	link, ok := c.state.RunLinks[runID]
	return link, ok
}

func (c *challengeProgrammingController) loadLocked() error {
	if cp := controlPlaneFor(c.wall); cp != nil {
		if _, err := cp.db.Exec(`CREATE TABLE IF NOT EXISTS challenge_programming_state (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			state_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
			return fmt.Errorf("create challenge programming state: %w", err)
		}
		var raw []byte
		err := cp.db.QueryRow(`SELECT state_json FROM challenge_programming_state WHERE id=1`).Scan(&raw)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load challenge programming state: %w", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &c.state); err != nil {
				return fmt.Errorf("decode challenge programming state: %w", err)
			}
		}
		return nil
	}
	path := c.localStatePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read challenge programming state: %w", err)
	}
	if err := json.Unmarshal(data, &c.state); err != nil {
		return fmt.Errorf("decode challenge programming state: %w", err)
	}
	return nil
}

func (c *challengeProgrammingController) persistLocked() error {
	raw, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	if cp := controlPlaneFor(c.wall); cp != nil {
		if _, err := cp.db.Exec(`CREATE TABLE IF NOT EXISTS challenge_programming_state (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			state_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
			return err
		}
		_, err = cp.db.Exec(`INSERT INTO challenge_programming_state(id,state_json,updated_at)
			VALUES(1,$1::jsonb,NOW())
			ON CONFLICT(id) DO UPDATE SET state_json=EXCLUDED.state_json,updated_at=NOW()`, string(raw))
		return err
	}
	path := c.localStatePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, raw, 0o644)
}

func (c *challengeProgrammingController) localStatePath() string {
	if c.wall.statePath != "" {
		return c.wall.statePath + ".programming.json"
	}
	if c.wall.dumpsDir != "" {
		return filepath.Join(c.wall.dumpsDir, challengeProgrammingStateFile)
	}
	return ""
}

func RunChallengeProgramming(w *Wall, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	controller := challengeProgrammingFor(w)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		controller.advance()
	}
}

func challengeProgrammingHTTPHandler(w *Wall, next http.Handler) http.Handler {
	controller := challengeProgrammingFor(w)
	controller.mu.Lock()
	controller.launcher = next
	controller.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/challenge-capabilities", func(res http.ResponseWriter, _ *http.Request) {
		writeJSON(res, http.StatusOK, map[string]any{"capability_version": challengeCapabilityVersion, "games": sortedCapabilities()})
	})
	mux.HandleFunc("GET /v1/challenges", func(res http.ResponseWriter, _ *http.Request) {
		writeJSON(res, http.StatusOK, map[string]any{"challenges": controller.listChallenges()})
	})
	mux.HandleFunc("POST /v1/challenges", func(res http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(res, req.Body, maxSmallControlBody)
		var challenge challengeDefinition
		if err := json.NewDecoder(req.Body).Decode(&challenge); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid challenge: " + err.Error()})
			return
		}
		saved, err := controller.saveChallenge(challenge)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusCreated, saved)
	})
	mux.HandleFunc("POST /v1/challenges/{id}/queue", func(res http.ResponseWriter, req *http.Request) {
		var body enqueueChallengeRequest
		if req.Body != nil {
			if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
				writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid queue request: " + err.Error()})
				return
			}
		}
		entry, err := controller.enqueueChallenge(req.PathValue("id"), body)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusCreated, entry)
	})
	mux.HandleFunc("POST /v1/programming/queue", func(res http.ResponseWriter, req *http.Request) {
		var body struct {
			Challenge   challengeDefinition `json:"challenge"`
			ScheduledAt int64               `json:"scheduled_at,omitempty"`
			PinNext     bool                `json:"pin_next,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid queue request: " + err.Error()})
			return
		}
		if strings.TrimSpace(body.Challenge.ID) == "" {
			body.Challenge.ID = "adhoc-" + strings.TrimPrefix(newRunID(), "run-")
		}
		if strings.TrimSpace(body.Challenge.Name) == "" {
			body.Challenge.Name = "One-off challenge"
		}
		body.Challenge.Version = 1
		body.Challenge.Ephemeral = true
		saved, err := controller.saveChallenge(body.Challenge)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		entry, err := controller.enqueueDefinition(saved, enqueueChallengeRequest{ScheduledAt: body.ScheduledAt, PinNext: body.PinNext})
		if err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusCreated, entry)
	})
	mux.HandleFunc("GET /v1/programming", func(res http.ResponseWriter, _ *http.Request) {
		writeJSON(res, http.StatusOK, controller.snapshot())
	})
	mux.HandleFunc("GET /v1/programming/runs/{id}", func(res http.ResponseWriter, req *http.Request) {
		link, ok := controller.runLink(req.PathValue("id"))
		if !ok {
			writeJSON(res, http.StatusNotFound, map[string]string{"error": "run is not linked to a challenge programming entry"})
			return
		}
		writeJSON(res, http.StatusOK, link)
	})
	mux.HandleFunc("POST /v1/programming/pause", func(res http.ResponseWriter, _ *http.Request) {
		if err := controller.setPaused(true); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, map[string]bool{"paused": true})
	})
	mux.HandleFunc("POST /v1/programming/resume", func(res http.ResponseWriter, _ *http.Request) {
		if err := controller.setPaused(false); err != nil {
			writeJSON(res, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, map[string]bool{"paused": false})
	})
	mux.HandleFunc("POST /v1/programming/reorder", func(res http.ResponseWriter, req *http.Request) {
		var body struct {
			EntryIDs []string `json:"entry_ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid reorder request: " + err.Error()})
			return
		}
		if err := controller.reorder(body.EntryIDs); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, controller.snapshot())
	})
	for _, action := range []string{"skip", "cancel", "pin-next", "retry"} {
		action := action
		mux.HandleFunc("POST /v1/programming/{id}/"+action, func(res http.ResponseWriter, req *http.Request) {
			entry, err := controller.mutateEntry(req.PathValue("id"), action)
			if err != nil {
				writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(res, http.StatusOK, entry)
		})
	}

	mux.Handle("/", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || !strings.HasPrefix(req.URL.Path, "/v1/runs/") || !strings.HasSuffix(req.URL.Path, "/finish") {
			next.ServeHTTP(res, req)
			return
		}
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, req)
		if recorder.Code >= 200 && recorder.Code < 300 {
			runID := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/v1/runs/"), "/finish")
			controller.finishRun(runID)
		}
		copyRecorder(res, recorder)
	}))
	controller.advance()
	return mux
}
