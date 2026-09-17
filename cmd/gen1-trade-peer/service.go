package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/game"
	gen1trade "github.com/maestroi/pokepilot/gen1/trade"
	"github.com/maestroi/pokepilot/red/data"
	redtrade "github.com/maestroi/pokepilot/red/trade"
)

var virtualTraderPolicies = []string{"scripted", "tradeback", "version-assisted", "pokedex"}

type sessionRequest = gen1trade.SessionRequest
type sessionStatus = gen1trade.SessionStatus

type managedSession struct {
	status sessionStatus
	cancel context.CancelFunc
}

type tradeService struct {
	broker      string
	romData     []byte
	peerID      string
	trainerName string
	timeout     time.Duration
	ttl         time.Duration
	events      io.Writer

	mu       sync.Mutex
	sessions map[string]*managedSession
	eventMu  sync.Mutex
}

type tradeServiceConfig struct {
	Broker      string
	ROMData     []byte
	PeerID      string
	TrainerName string
	Timeout     time.Duration
	TTL         time.Duration
	Events      io.Writer
}

func newTradeService(cfg tradeServiceConfig) *tradeService {
	if cfg.PeerID == "" {
		cfg.PeerID = "virtual-trader"
	}
	if cfg.TrainerName == "" {
		cfg.TrainerName = "POKEPILOT"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 15 * time.Minute
	}
	return &tradeService{
		broker: cfg.Broker, romData: append([]byte(nil), cfg.ROMData...), peerID: cfg.PeerID,
		trainerName: cfg.TrainerName, timeout: cfg.Timeout, ttl: cfg.TTL, events: cfg.Events,
		sessions: map[string]*managedSession{},
	}
}

func (s *tradeService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "available": true})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/capabilities":
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"kind":      "gen1_virtual_trader",
			"policies":  append([]string(nil), virtualTraderPolicies...),
		})
	case r.URL.Path == "/v1/sessions" && r.Method == http.MethodPost:
		s.handleCreate(w, r)
	case r.URL.Path == "/v1/sessions" && r.Method == http.MethodGet:
		s.handleList(w)
	case strings.HasPrefix(r.URL.Path, "/v1/sessions/"):
		s.handleSession(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *tradeService) handleCreate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req sessionRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request: " + err.Error()})
		return
	}
	status, err := s.start(req)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, errSessionActive) {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *tradeService) handleList(w http.ResponseWriter) {
	s.mu.Lock()
	out := make([]sessionStatus, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, session.status)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	writeJSON(w, http.StatusOK, out)
}

func (s *tradeService) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	session, ok := s.sessions[id]
	if ok && r.Method == http.MethodDelete {
		session.cancel()
	}
	var status sessionStatus
	if ok {
		status = session.status
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown session"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, status)
	case http.MethodDelete:
		writeJSON(w, http.StatusAccepted, map[string]string{"session": id, "status": "cancelling"})
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

var errSessionActive = errors.New("virtual trader session already active")

func (s *tradeService) start(req sessionRequest) (sessionStatus, error) {
	req.Session = strings.TrimSpace(req.Session)
	if req.Session == "" {
		return sessionStatus{}, errors.New("session is required")
	}
	if strings.ContainsAny(req.Session, " \t\r\n/") {
		return sessionStatus{}, errors.New("session must be a single path-safe token")
	}
	req.Policy = strings.ToLower(strings.TrimSpace(req.Policy))
	if req.Policy == "" {
		req.Policy = "tradeback"
	}
	if err := validatePolicy(req.Policy); err != nil {
		return sessionStatus{}, err
	}
	req.Species = game.CanonicalID(req.Species)
	if req.Species == "" {
		req.Species = "pidgey"
	}
	if req.Level == 0 {
		req.Level = 20
	}
	if req.Level < 1 || req.Level > 100 {
		return sessionStatus{}, fmt.Errorf("level %d outside 1..100", req.Level)
	}
	if req.Game == "" {
		req.Game = "pokemon-red"
	}

	machine, err := s.machine(req)
	if err != nil {
		return sessionStatus{}, err
	}
	status := sessionStatus{
		Session: req.Session, RunID: strings.TrimSpace(req.RunID), Game: strings.TrimSpace(req.Game),
		Policy: req.Policy, Species: req.Species, Level: req.Level, Status: "starting", StartedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	if current, exists := s.sessions[req.Session]; exists && (current.status.Status == "starting" || current.status.Status == "running") {
		s.mu.Unlock()
		return sessionStatus{}, fmt.Errorf("%w: %s", errSessionActive, req.Session)
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.ttl)
	managed := &managedSession{status: status, cancel: cancel}
	s.sessions[req.Session] = managed
	s.mu.Unlock()

	go s.run(ctx, managed, req, machine)
	return status, nil
}

func (s *tradeService) machine(req sessionRequest) (*gen1trade.Machine, error) {
	mon, err := redtrade.SyntheticMon(s.romData, game.SpeciesID(req.Species), uint8(req.Level), 0x504b)
	if err != nil {
		return nil, fmt.Errorf("build offered Pokemon: %w", err)
	}
	return gen1trade.NewMachine(gen1trade.MachineConfig{
		Trainer:   gen1trade.NewTrainer(s.trainerName, mon),
		Tradeback: req.Policy == "tradeback",
		OnEvent: func(event gen1trade.Event) {
			if s.events == nil {
				return
			}
			name, _ := data.SpeciesName(event.Species)
			s.eventMu.Lock()
			defer s.eventMu.Unlock()
			_ = json.NewEncoder(s.events).Encode(map[string]any{
				"type": "trade_event", "session": req.Session, "run_id": req.RunID,
				"event": event.Kind, "remote_slot": event.RemoteSlot, "local_slot": event.LocalSlot,
				"species": name, "species_id": fmt.Sprintf("0x%02x", event.Species), "policy": req.Policy,
			})
		},
	})
}

func (s *tradeService) run(ctx context.Context, managed *managedSession, req sessionRequest, machine *gen1trade.Machine) {
	metadata := map[string]string{
		"game": req.Game, "trade_policy": req.Policy, "requested_species": req.Species,
		"status": "ready_to_trade", "run_id": req.RunID,
	}
	err := gen1trade.RunBroker(ctx, gen1trade.BrokerConfig{
		Address: s.broker, Session: req.Session, PeerID: s.peerID + "-" + req.Session,
		Metadata: metadata, Timeout: s.timeout,
		OnReady: func() { s.setStatus(req.Session, "running", "") },
	}, machine)
	managed.cancel()
	switch {
	case err == nil:
		s.setStatus(req.Session, "done", "")
	case errors.Is(err, context.Canceled):
		s.setStatus(req.Session, "cancelled", "")
	case errors.Is(err, context.DeadlineExceeded):
		s.setStatus(req.Session, "expired", "session TTL exceeded")
	default:
		s.setStatus(req.Session, "error", err.Error())
	}
}

func (s *tradeService) setStatus(id, status, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.sessions[id]; session != nil {
		session.status.Status = status
		session.status.Error = detail
	}
}

func (s *tradeService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		session.cancel()
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
