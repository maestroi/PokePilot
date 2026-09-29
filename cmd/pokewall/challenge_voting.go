package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	challengeVotingStateFile = "challenge-voting.json"

	voteStatusOpen      = "open"
	voteStatusClosed    = "closed"
	voteStatusCancelled = "cancelled"

	voteTieFirst    = "first"
	voteTieOperator = "operator"
)

var voteSourcePattern = regexp.MustCompile("^[a-z][a-z0-9_-]{0,31}$")

var (
	errVoteDuplicate = errors.New("voter already submitted a ballot")
	errVoteRateLimit = errors.New("vote source limit reached")
)

type challengeVoteCandidateRef struct {
	ChallengeID string `json:"challenge_id"`
	Version     int    `json:"version,omitempty"`
}

type challengeVoteCandidate struct {
	ChallengeID      string `json:"challenge_id"`
	ChallengeVersion int    `json:"challenge_version"`
	ChallengeName    string `json:"challenge_name"`
	Votes            int    `json:"votes"`
}

type challengeVoteBallot struct {
	CandidateID string `json:"candidate_id"`
	Source      string `json:"source"`
}

type challengeVoteSession struct {
	ID           string                         `json:"id"`
	Status       string                         `json:"status"`
	OpenedAt     int64                          `json:"opened_at"`
	ClosesAt     int64                          `json:"closes_at,omitempty"`
	ClosedAt     int64                          `json:"closed_at,omitempty"`
	TiePolicy    string                         `json:"tie_policy"`
	Candidates   []challengeVoteCandidate       `json:"candidates"`
	WinnerID     string                         `json:"winner_id,omitempty"`
	QueueEntryID string                         `json:"queue_entry_id,omitempty"`
	SourceTotals map[string]int                 `json:"source_totals,omitempty"`
	SourceLimits map[string]int                 `json:"source_limits,omitempty"`
	Ballots      map[string]challengeVoteBallot `json:"ballots,omitempty"`
}

type challengeVoteSnapshot struct {
	ID           string                   `json:"id"`
	Status       string                   `json:"status"`
	OpenedAt     int64                    `json:"opened_at"`
	ClosesAt     int64                    `json:"closes_at,omitempty"`
	ClosedAt     int64                    `json:"closed_at,omitempty"`
	TiePolicy    string                   `json:"tie_policy"`
	Candidates   []challengeVoteCandidate `json:"candidates"`
	WinnerID     string                   `json:"winner_id,omitempty"`
	QueueEntryID string                   `json:"queue_entry_id,omitempty"`
	SourceTotals map[string]int           `json:"source_totals,omitempty"`
}

type challengeVotingState struct {
	Sessions []challengeVoteSession `json:"sessions"`
}

type challengeVotingController struct {
	wall *Wall

	mu      sync.Mutex
	closeMu sync.Mutex
	loaded  bool
	loadErr error
	state   challengeVotingState
}

type createChallengeVoteRequest struct {
	Candidates   []challengeVoteCandidateRef `json:"candidates"`
	ClosesAt     int64                       `json:"closes_at,omitempty"`
	TiePolicy    string                      `json:"tie_policy,omitempty"`
	SourceLimits map[string]int              `json:"source_limits,omitempty"`
}

type castChallengeVoteRequest struct {
	Source      string `json:"source"`
	VoterID     string `json:"voter_id"`
	CandidateID string `json:"candidate_id"`
}

type closeChallengeVoteRequest struct {
	WinnerID string `json:"winner_id,omitempty"`
}

var challengeVotingControllers sync.Map // *Wall -> *challengeVotingController

func challengeVotingFor(w *Wall) *challengeVotingController {
	if existing, ok := challengeVotingControllers.Load(w); ok {
		c := existing.(*challengeVotingController)
		c.ensureLoaded()
		return c
	}
	c := &challengeVotingController{wall: w}
	actual, _ := challengeVotingControllers.LoadOrStore(w, c)
	out := actual.(*challengeVotingController)
	out.ensureLoaded()
	return out
}

func (c *challengeVotingController) ensureLoaded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return
	}
	c.loaded = true
	c.loadErr = c.loadLocked()
	for i := range c.state.Sessions {
		session := &c.state.Sessions[i]
		if session.SourceTotals == nil {
			session.SourceTotals = map[string]int{}
		}
		if session.SourceLimits == nil {
			session.SourceLimits = map[string]int{}
		}
		if session.Ballots == nil {
			session.Ballots = map[string]challengeVoteBallot{}
		}
	}
}

func voteSnapshot(in challengeVoteSession) challengeVoteSnapshot {
	return challengeVoteSnapshot{
		ID: in.ID, Status: in.Status, OpenedAt: in.OpenedAt, ClosesAt: in.ClosesAt,
		ClosedAt: in.ClosedAt, TiePolicy: in.TiePolicy, WinnerID: in.WinnerID,
		QueueEntryID: in.QueueEntryID,
		Candidates:   append([]challengeVoteCandidate(nil), in.Candidates...),
		SourceTotals: cloneStringIntMap(in.SourceTotals),
	}
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (c *challengeVotingController) list() ([]challengeVoteSnapshot, *challengeVoteSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]challengeVoteSnapshot, 0, len(c.state.Sessions))
	var active *challengeVoteSnapshot
	for _, session := range c.state.Sessions {
		snapshot := voteSnapshot(session)
		out = append(out, snapshot)
		if session.Status == voteStatusOpen {
			copy := snapshot
			active = &copy
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt > out[j].OpenedAt })
	return out, active
}

func (c *challengeVotingController) active() *challengeVoteSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.state.Sessions) - 1; i >= 0; i-- {
		if c.state.Sessions[i].Status == voteStatusOpen {
			out := voteSnapshot(c.state.Sessions[i])
			return &out
		}
	}
	return nil
}

func normalizeVoteSourceLimits(in map[string]int) (map[string]int, error) {
	if len(in) == 0 {
		return map[string]int{}, nil
	}
	out := make(map[string]int, len(in))
	for source, limit := range in {
		source = strings.ToLower(strings.TrimSpace(source))
		if !voteSourcePattern.MatchString(source) {
			return nil, fmt.Errorf("invalid vote source %q", source)
		}
		if limit < 1 {
			return nil, fmt.Errorf("source limit for %s must be positive", source)
		}
		out[source] = limit
	}
	return out, nil
}

func (c *challengeVotingController) create(request createChallengeVoteRequest) (challengeVoteSnapshot, error) {
	c.mu.Lock()
	if c.loadErr != nil {
		err := c.loadErr
		c.mu.Unlock()
		return challengeVoteSnapshot{}, err
	}
	for _, session := range c.state.Sessions {
		if session.Status == voteStatusOpen {
			c.mu.Unlock()
			return challengeVoteSnapshot{}, errors.New("an audience vote is already open")
		}
	}
	c.mu.Unlock()

	if len(request.Candidates) < 2 || len(request.Candidates) > 8 {
		return challengeVoteSnapshot{}, errors.New("vote requires between 2 and 8 candidates")
	}
	tiePolicy := strings.ToLower(strings.TrimSpace(request.TiePolicy))
	if tiePolicy == "" {
		tiePolicy = voteTieFirst
	}
	if tiePolicy != voteTieFirst && tiePolicy != voteTieOperator {
		return challengeVoteSnapshot{}, errors.New("tie_policy must be first or operator")
	}
	now := time.Now().Unix()
	if request.ClosesAt != 0 && request.ClosesAt <= now {
		return challengeVoteSnapshot{}, errors.New("closes_at must be in the future")
	}
	sourceLimits, err := normalizeVoteSourceLimits(request.SourceLimits)
	if err != nil {
		return challengeVoteSnapshot{}, err
	}

	programming := challengeProgrammingFor(c.wall)
	candidates := make([]challengeVoteCandidate, 0, len(request.Candidates))
	seen := map[string]bool{}
	for _, ref := range request.Candidates {
		id := strings.ToLower(strings.TrimSpace(ref.ChallengeID))
		if id == "" {
			return challengeVoteSnapshot{}, errors.New("candidate challenge_id is required")
		}
		if seen[id] {
			return challengeVoteSnapshot{}, fmt.Errorf("candidate challenge %s is repeated", id)
		}
		programming.mu.Lock()
		challenge, ok := programming.challengeLocked(id, ref.Version)
		programming.mu.Unlock()
		if !ok {
			return challengeVoteSnapshot{}, fmt.Errorf("candidate challenge %s version %d was not found", id, ref.Version)
		}
		if err := programming.validateChallengeReady(challenge); err != nil {
			return challengeVoteSnapshot{}, fmt.Errorf("candidate challenge %s is not ready: %w", id, err)
		}
		seen[id] = true
		candidates = append(candidates, challengeVoteCandidate{
			ChallengeID: challenge.ID, ChallengeVersion: challenge.Version, ChallengeName: challenge.Name,
		})
	}

	session := challengeVoteSession{
		ID: "vote-" + strings.TrimPrefix(newRunID(), "run-"), Status: voteStatusOpen,
		OpenedAt: now, ClosesAt: request.ClosesAt, TiePolicy: tiePolicy,
		Candidates: candidates, SourceTotals: map[string]int{}, SourceLimits: sourceLimits,
		Ballots: map[string]challengeVoteBallot{},
	}
	c.mu.Lock()
	c.state.Sessions = append(c.state.Sessions, session)
	if err := c.persistLocked(); err != nil {
		c.state.Sessions = c.state.Sessions[:len(c.state.Sessions)-1]
		c.mu.Unlock()
		return challengeVoteSnapshot{}, err
	}
	c.mu.Unlock()
	return voteSnapshot(session), nil
}

func voteIdentityDigest(sessionID, source, voterID string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + source + "\x00" + voterID))
	return hex.EncodeToString(sum[:])
}

func (c *challengeVotingController) cast(id string, request castChallengeVoteRequest) (challengeVoteSnapshot, bool, error) {
	source := strings.ToLower(strings.TrimSpace(request.Source))
	voterID := strings.TrimSpace(request.VoterID)
	candidateID := strings.ToLower(strings.TrimSpace(request.CandidateID))
	if !voteSourcePattern.MatchString(source) {
		return challengeVoteSnapshot{}, false, fmt.Errorf("invalid vote source %q", source)
	}
	if voterID == "" || len(voterID) > 256 {
		return challengeVoteSnapshot{}, false, errors.New("voter_id must be between 1 and 256 characters")
	}
	if candidateID == "" {
		return challengeVoteSnapshot{}, false, errors.New("candidate_id is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.sessionIndexLocked(id)
	if index < 0 {
		return challengeVoteSnapshot{}, false, errors.New("vote session not found")
	}
	session := &c.state.Sessions[index]
	if session.Status != voteStatusOpen {
		return challengeVoteSnapshot{}, false, errors.New("vote session is not open")
	}
	if session.ClosesAt != 0 && session.ClosesAt <= time.Now().Unix() {
		return challengeVoteSnapshot{}, false, errors.New("vote session has reached its closing time")
	}
	candidateIndex := -1
	for i := range session.Candidates {
		if session.Candidates[i].ChallengeID == candidateID {
			candidateIndex = i
			break
		}
	}
	if candidateIndex < 0 {
		return challengeVoteSnapshot{}, false, errors.New("candidate is not offered by this vote")
	}
	digest := voteIdentityDigest(session.ID, source, voterID)
	if previous, ok := session.Ballots[digest]; ok {
		if previous.CandidateID == candidateID {
			return voteSnapshot(*session), false, nil
		}
		return challengeVoteSnapshot{}, false, errVoteDuplicate
	}
	if limit := session.SourceLimits[source]; limit > 0 && session.SourceTotals[source] >= limit {
		return challengeVoteSnapshot{}, false, errVoteRateLimit
	}

	session.Ballots[digest] = challengeVoteBallot{CandidateID: candidateID, Source: source}
	session.Candidates[candidateIndex].Votes++
	session.SourceTotals[source]++
	if err := c.persistLocked(); err != nil {
		delete(session.Ballots, digest)
		session.Candidates[candidateIndex].Votes--
		session.SourceTotals[source]--
		return challengeVoteSnapshot{}, false, err
	}
	return voteSnapshot(*session), true, nil
}

func (c *challengeVotingController) sessionIndexLocked(id string) int {
	id = strings.TrimSpace(id)
	for i := range c.state.Sessions {
		if c.state.Sessions[i].ID == id {
			return i
		}
	}
	return -1
}

func resolveVoteWinner(session challengeVoteSession, override string) (challengeVoteCandidate, error) {
	override = strings.ToLower(strings.TrimSpace(override))
	if override != "" {
		for _, candidate := range session.Candidates {
			if candidate.ChallengeID == override {
				return candidate, nil
			}
		}
		return challengeVoteCandidate{}, errors.New("winner_id is not an offered candidate")
	}
	if len(session.Candidates) == 0 {
		return challengeVoteCandidate{}, errors.New("vote has no candidates")
	}
	high := session.Candidates[0].Votes
	winners := []challengeVoteCandidate{session.Candidates[0]}
	for _, candidate := range session.Candidates[1:] {
		if candidate.Votes > high {
			high = candidate.Votes
			winners = []challengeVoteCandidate{candidate}
		} else if candidate.Votes == high {
			winners = append(winners, candidate)
		}
	}
	if len(winners) > 1 && session.TiePolicy == voteTieOperator {
		return challengeVoteCandidate{}, errors.New("vote is tied and requires winner_id")
	}
	// Candidate order is operator-authored and persisted, so choosing the first
	// tied candidate is stable across retries and restarts.
	return winners[0], nil
}

func (c *challengeVotingController) close(id, override string) (challengeVoteSnapshot, error) {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	c.mu.Lock()
	index := c.sessionIndexLocked(id)
	if index < 0 {
		c.mu.Unlock()
		return challengeVoteSnapshot{}, errors.New("vote session not found")
	}
	session := c.state.Sessions[index]
	if session.Status == voteStatusClosed {
		out := voteSnapshot(session)
		c.mu.Unlock()
		return out, nil
	}
	if session.Status != voteStatusOpen {
		c.mu.Unlock()
		return challengeVoteSnapshot{}, errors.New("vote session is not open")
	}
	winner, err := resolveVoteWinner(session, override)
	c.mu.Unlock()
	if err != nil {
		return challengeVoteSnapshot{}, err
	}

	entry, err := challengeProgrammingFor(c.wall).enqueueVoteWinner(session.ID, winner.ChallengeID, winner.ChallengeVersion)
	if err != nil {
		return challengeVoteSnapshot{}, fmt.Errorf("queue winning challenge: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	index = c.sessionIndexLocked(id)
	if index < 0 {
		return challengeVoteSnapshot{}, errors.New("vote session disappeared during close")
	}
	target := &c.state.Sessions[index]
	if target.Status == voteStatusClosed {
		return voteSnapshot(*target), nil
	}
	target.Status = voteStatusClosed
	target.ClosedAt = time.Now().Unix()
	target.WinnerID = winner.ChallengeID
	target.QueueEntryID = entry.ID
	if err := c.persistLocked(); err != nil {
		return challengeVoteSnapshot{}, err
	}
	return voteSnapshot(*target), nil
}

func (c *challengeVotingController) cancel(id string) (challengeVoteSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := c.sessionIndexLocked(id)
	if index < 0 {
		return challengeVoteSnapshot{}, errors.New("vote session not found")
	}
	session := &c.state.Sessions[index]
	if session.Status != voteStatusOpen {
		return challengeVoteSnapshot{}, errors.New("vote session is not open")
	}
	session.Status = voteStatusCancelled
	session.ClosedAt = time.Now().Unix()
	if err := c.persistLocked(); err != nil {
		return challengeVoteSnapshot{}, err
	}
	return voteSnapshot(*session), nil
}

func (c *challengeProgrammingController) enqueueVoteWinner(voteID, challengeID string, version int) (challengeQueueEntry, error) {
	challengeID = strings.ToLower(strings.TrimSpace(challengeID))
	voteID = strings.TrimSpace(voteID)
	if voteID == "" {
		return challengeQueueEntry{}, errors.New("vote id is required")
	}
	entryID := "slot-" + voteID

	c.mu.Lock()
	for _, existing := range c.state.Entries {
		if existing.ID == entryID {
			c.mu.Unlock()
			return existing, nil
		}
	}
	challenge, ok := c.challengeLocked(challengeID, version)
	c.mu.Unlock()
	if !ok {
		return challengeQueueEntry{}, fmt.Errorf("challenge %s version %d not found", challengeID, version)
	}
	if err := c.validateChallengeReady(challenge); err != nil {
		return challengeQueueEntry{}, err
	}

	now := time.Now().Unix()
	entry := challengeQueueEntry{
		ID: entryID, ChallengeID: challenge.ID, ChallengeVersion: challenge.Version,
		ChallengeName: challenge.Name, State: programStateQueued, CreatedAt: now, Pinned: true,
	}
	c.mu.Lock()
	for _, existing := range c.state.Entries {
		if existing.ID == entryID {
			c.mu.Unlock()
			return existing, nil
		}
	}
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
	if err := c.persistLocked(); err != nil {
		c.state.Entries = append(c.state.Entries[:insert], c.state.Entries[insert+1:]...)
		c.mu.Unlock()
		return challengeQueueEntry{}, err
	}
	c.mu.Unlock()
	c.advance()
	return entry, nil
}

func (c *challengeVotingController) closeDue() {
	now := time.Now().Unix()
	var due []string
	c.mu.Lock()
	for _, session := range c.state.Sessions {
		if session.Status == voteStatusOpen && session.ClosesAt > 0 && session.ClosesAt <= now {
			due = append(due, session.ID)
		}
	}
	c.mu.Unlock()
	for _, id := range due {
		_, _ = c.close(id, "")
	}
}

func (c *challengeVotingController) loadLocked() error {
	if cp := controlPlaneFor(c.wall); cp != nil {
		query := "CREATE TABLE IF NOT EXISTS challenge_voting_state (" +
			"id SMALLINT PRIMARY KEY CHECK (id = 1)," +
			"state_json JSONB NOT NULL," +
			"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())"
		if _, err := cp.db.Exec(query); err != nil {
			return fmt.Errorf("create challenge voting state: %w", err)
		}
		var raw []byte
		err := cp.db.QueryRow("SELECT state_json FROM challenge_voting_state WHERE id=1").Scan(&raw)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("load challenge voting state: %w", err)
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &c.state); err != nil {
				return fmt.Errorf("decode challenge voting state: %w", err)
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
		return fmt.Errorf("read challenge voting state: %w", err)
	}
	if err := json.Unmarshal(data, &c.state); err != nil {
		return fmt.Errorf("decode challenge voting state: %w", err)
	}
	return nil
}

func (c *challengeVotingController) persistLocked() error {
	raw, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	if cp := controlPlaneFor(c.wall); cp != nil {
		query := "CREATE TABLE IF NOT EXISTS challenge_voting_state (" +
			"id SMALLINT PRIMARY KEY CHECK (id = 1)," +
			"state_json JSONB NOT NULL," +
			"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())"
		if _, err := cp.db.Exec(query); err != nil {
			return err
		}
		_, err = cp.db.Exec("INSERT INTO challenge_voting_state(id,state_json,updated_at) "+
			"VALUES(1,$1::jsonb,NOW()) ON CONFLICT(id) DO UPDATE SET state_json=EXCLUDED.state_json,updated_at=NOW()", string(raw))
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

func (c *challengeVotingController) localStatePath() string {
	if c.wall.statePath != "" {
		return c.wall.statePath + ".voting.json"
	}
	if c.wall.dumpsDir != "" {
		return filepath.Join(c.wall.dumpsDir, challengeVotingStateFile)
	}
	return ""
}

func RunChallengeVoting(w *Wall, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	controller := challengeVotingFor(w)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		controller.closeDue()
	}
}

func challengeVotingHTTPHandler(w *Wall, next http.Handler) http.Handler {
	controller := challengeVotingFor(w)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/votes", func(res http.ResponseWriter, _ *http.Request) {
		sessions, active := controller.list()
		writeJSON(res, http.StatusOK, map[string]any{"sessions": sessions, "active": active})
	})
	mux.HandleFunc("GET /v1/votes/active", func(res http.ResponseWriter, _ *http.Request) {
		writeJSON(res, http.StatusOK, map[string]any{"active": controller.active()})
	})
	mux.HandleFunc("POST /v1/votes", func(res http.ResponseWriter, req *http.Request) {
		var body createChallengeVoteRequest
		if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid vote request: " + err.Error()})
			return
		}
		session, err := controller.create(body)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusCreated, session)
	})
	mux.HandleFunc("POST /v1/votes/{id}/ballots", func(res http.ResponseWriter, req *http.Request) {
		var body castChallengeVoteRequest
		if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid ballot: " + err.Error()})
			return
		}
		session, accepted, err := controller.cast(req.PathValue("id"), body)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errVoteDuplicate) {
				status = http.StatusConflict
			} else if errors.Is(err, errVoteRateLimit) {
				status = http.StatusTooManyRequests
			}
			writeJSON(res, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, map[string]any{"accepted": accepted, "vote": session})
	})
	mux.HandleFunc("POST /v1/votes/{id}/close", func(res http.ResponseWriter, req *http.Request) {
		var body closeChallengeVoteRequest
		if req.Body != nil && req.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, maxSmallControlBody)).Decode(&body); err != nil {
				writeJSON(res, http.StatusBadRequest, map[string]string{"error": "invalid close request: " + err.Error()})
				return
			}
		}
		session, err := controller.close(req.PathValue("id"), body.WinnerID)
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, session)
	})
	mux.HandleFunc("POST /v1/votes/{id}/cancel", func(res http.ResponseWriter, req *http.Request) {
		session, err := controller.cancel(req.PathValue("id"))
		if err != nil {
			writeJSON(res, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(res, http.StatusOK, session)
	})
	mux.Handle("/", next)
	return mux
}
