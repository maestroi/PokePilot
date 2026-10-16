package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/maestroi/pokepilot/media/producer"
)

const maxProducerRequestBytes = 128 << 10

type producerRequest struct {
	// Historical rows must originate from durable run statistics and supply
	// explicit source identifiers. They are operator-provided, not model-made.
	History []producer.HistoricalRun `json:"history,omitempty"`
}

func (s *replayServer) producerInput(r *http.Request, runID string, history []producer.HistoricalRun) (producer.Input, error) {
	in := producer.Input{RunID: runID, History: history}
	latest, err := s.artifacts(r.Context(), runID)
	if err != nil {
		return in, err
	}
	attempts := latest.Attempt
	if attempts < 1 {
		attempts = 1
	}
	// This reads the same verified artifact generations as replay, but does
	// not require the full .gbrun media file to exist.
	if attempts > 64 {
		return in, errors.New("producer: too many attempts to assess in one request")
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		timeline, err := s.mediaTimelineAttempt(r.Context(), runID, attempt)
		if errors.Is(err, errMediaTimelineNotFound) || errors.Is(err, errRunNotFound) {
			in.Incomplete = true
			continue // historical runs may lack the optional semantic artifact
		}
		if err != nil {
			return in, err
		}
		if timeline.Attempt == 0 {
			timeline.Attempt = attempt
		}
		if timeline.Attempt != attempt {
			return in, fmt.Errorf("producer: timeline attempt %d does not match artifact generation %d", timeline.Attempt, attempt)
		}
		in.Timelines = append(in.Timelines, timeline)
	}
	return in, nil
}

func producerObjectKey(runID, hash string) string {
	sum := sha256.Sum256([]byte(runID))
	return path.Join("studio", hex.EncodeToString(sum[:]), "producer", "v1", hash+".json")
}

func configuredProducerModel() (producer.Model, error) {
	endpoint := strings.TrimSpace(os.Getenv("POKEPILOT_PRODUCER_CHAT_URL"))
	model := strings.TrimSpace(os.Getenv("POKEPILOT_PRODUCER_MODEL"))
	if endpoint == "" {
		if model != "" {
			return nil, errors.New("producer model configured without chat URL")
		}
		return nil, nil // explicit rules-v1 offline producer
	}
	if model == "" {
		return nil, errors.New("producer chat URL configured without model ID")
	}
	return producer.ChatModel{
		Endpoint: endpoint, ModelID: model,
		Deployment: strings.TrimSpace(os.Getenv("POKEPILOT_PRODUCER_DEPLOYMENT")),
		APIKey:     os.Getenv("POKEPILOT_PRODUCER_API_KEY"),
	}, nil
}

// handleProducerAssess is an explicit operator-triggered activity. It never
// changes the run or starts publishing/rendering anything. S3 objects are
// immutable by content hash and newer producer policies can coexist.
func (s *replayServer) handleProducerAssess(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing run id"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 storage is required for durable studio assessments"})
		return
	}
	var body producerRequest
	if r.ContentLength > maxProducerRequestBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "producer request too large"})
		return
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxProducerRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid producer request"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trailing or oversized producer request"})
		return
	}
	in, err := s.producerInput(r, runID, body.History)
	if err != nil {
		writeReplayError(w, err)
		return
	}
	model, err := configuredProducerModel()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	assessment, err := producer.Assess(r.Context(), in, model, producer.DefaultPolicy())
	if err != nil {
		// Malformed model output never becomes a publishable artifact.
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := producer.Validate(assessment, in); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	data, err := json.MarshalIndent(assessment, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode producer assessment"})
		return
	}
	key := producerObjectKey(runID, assessment.Hash)
	if _, err := s.store.PutObject(r.Context(), key, "application/json", data); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("persist producer assessment: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Assessment producer.Assessment `json:"assessment"`
		ObjectKey  string              `json:"object_key"`
	}{assessment, key})
}

func (s *replayServer) handleProducerAssessment(w http.ResponseWriter, r *http.Request) {
	runID, hash := r.PathValue("id"), r.PathValue("hash")
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" || strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid assessment identity"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio artifact storage disabled"})
		return
	}
	obj, err := s.store.GetObject(r.Context(), producerObjectKey(runID, hash), "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "assessment not found"})
		return
	}
	defer obj.Body.Close()
	data, err := io.ReadAll(io.LimitReader(obj.Body, maxProducerRequestBytes+1))
	if err != nil || len(data) > maxProducerRequestBytes {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stored assessment unreadable"})
		return
	}
	var assessment producer.Assessment
	if err := json.Unmarshal(data, &assessment); err != nil || assessment.Hash != hash || assessment.RunID != runID || producer.VerifyChecksum(assessment) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stored assessment identity mismatch"})
		return
	}
	writeJSON(w, http.StatusOK, assessment)
}
