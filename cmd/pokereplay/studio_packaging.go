package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/editplan"
	"github.com/maestroi/pokepilot/media/packaging"
	"github.com/maestroi/pokepilot/media/producer"
)

const maxPackagingRequestBytes = 16 << 10
const maxStoredPackagingBytes = 128 << 10

type packagingCreateRequest struct {
	AssessmentHash string          `json:"assessment_hash"`
	EditPlanHash   string          `json:"edit_plan_hash,omitempty"`
	Style          packaging.Style `json:"style,omitempty"`
}
type packagingCreateReply struct {
	Package   packaging.Package `json:"package"`
	ObjectKey string            `json:"object_key"`
}

// Content-addressed metadata never modifies the producer or source recordings.
func packagingObjectKey(runID, hash string) string {
	root := path.Dir(path.Dir(path.Dir(producerObjectKey(runID, hash))))
	return path.Join(root, "packaging", "v1", hash+".json")
}
func configuredPackagingModel() (packaging.Model, error) {
	endpoint := strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_CHAT_URL"))
	model := strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_MODEL"))
	if endpoint == "" {
		if model != "" {
			return nil, errors.New("packaging model configured without chat URL")
		}
		return nil, nil
	}
	if model == "" {
		return nil, errors.New("packaging chat URL configured without model")
	}
	return packaging.ChatModel{
		Endpoint: endpoint, ModelID: model,
		Deployment: strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_DEPLOYMENT")),
		APIKey:     os.Getenv("POKEPILOT_PACKAGING_API_KEY"),
	}, nil
}
func (s *replayServer) packagingInputs(ctx context.Context, runID, assessmentHash, planHash string) (producer.Assessment, []farm.MediaTimeline, *editplan.Plan, error) {
	var none producer.Assessment
	if !validEditHash(assessmentHash) {
		return none, nil, nil, errors.New("packaging: invalid assessment hash")
	}
	a, err := s.loadEditAssessment(ctx, runID, assessmentHash)
	if err != nil {
		return none, nil, nil, err
	}
	// Read durable timeline generations directly. Packaging can be regenerated
	// for an older recording without rendering or emulating gameplay again.
	// No history rows are needed to validate event-level provenance here.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/", nil)
	if err != nil {
		return none, nil, nil, err
	}
	in, err := s.producerInput(req, runID, nil)
	if err != nil {
		return none, nil, nil, err
	}
	if in.Incomplete || len(in.Timelines) == 0 {
		return none, nil, nil, errors.New("packaging: complete semantic timelines are required")
	}
	var edit *editplan.Plan
	if planHash != "" {
		if !validEditHash(planHash) {
			return none, nil, nil, errors.New("packaging: invalid edit plan hash")
		}
		loaded, err := s.loadEditPlan(ctx, runID, planHash)
		if err != nil {
			return none, nil, nil, err
		}
		edit = &loaded
	}
	return a, in.Timelines, edit, nil
}

func (s *replayServer) handlePackagingCreate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing run ID"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio artifact storage disabled"})
		return
	}
	if r.ContentLength > maxPackagingRequestBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "packaging request oversized"})
		return
	}
	var body packagingCreateRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxPackagingRequestBytes+1))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid packaging request"})
		return
	}
	if dec.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packaging request has trailing or oversized JSON"})
		return
	}
	a, timelines, plan, err := s.packagingInputs(r.Context(), runID, body.AssessmentHash, body.EditPlanHash)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	model, err := configuredPackagingModel()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	in := packaging.Request{
		Assessment: a, Timelines: timelines, Plan: plan,
		Style: body.Style, RunBaseURL: strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_PUBLIC_BASE_URL")),
		Model: model,
	}
	pkg, err := packaging.Generate(r.Context(), in)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := packaging.Validate(pkg, in); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": fmt.Sprintf("invalid generated packaging: %v", err)})
		return
	}
	raw, err := json.Marshal(pkg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode packaging"})
		return
	}
	objectKey := packagingObjectKey(runID, pkg.Hash)
	if _, err := s.store.PutObject(r.Context(), objectKey, "application/json", raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "persist packaging failed"})
		return
	}
	writeJSON(w, http.StatusOK, packagingCreateReply{Package: pkg, ObjectKey: objectKey})
}

func (s *replayServer) handlePackagingGet(w http.ResponseWriter, r *http.Request) {
	runID, hash := r.PathValue("id"), r.PathValue("hash")
	if !validEditHash(hash) || strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid packaging identity"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio artifact storage disabled"})
		return
	}
	obj, err := s.store.GetObject(r.Context(), packagingObjectKey(runID, hash), "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "packaging not found"})
		return
	}
	defer obj.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(obj.Body, maxStoredPackagingBytes+1))
	if err != nil || len(raw) > maxStoredPackagingBytes {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stored packaging unreadable"})
		return
	}
	var pkg packaging.Package
	if err := json.Unmarshal(raw, &pkg); err != nil || pkg.RunID != runID || pkg.Hash != hash || packaging.VerifyChecksum(pkg) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stored packaging identity mismatch"})
		return
	}
	a, timelines, plan, err := s.packagingInputs(r.Context(), runID, pkg.AssessmentHash, pkg.PlanHash)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "packaging source is no longer verifiable"})
		return
	}
	in := packaging.Request{
		Assessment: a, Timelines: timelines, Plan: plan,
		Style: pkg.Style, RunBaseURL: strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_PUBLIC_BASE_URL")),
	}
	if err := packaging.Validate(pkg, in); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "stored packaging no longer matches semantic evidence"})
		return
	}
	writeJSON(w, http.StatusOK, pkg)
}

// studioPackageSummary says which stored packages exist for a run and whether
// thumbnails were rendered (and one selected) for each, so the operator UI can
// show what already exists without asking for a hash.
type studioPackageSummary struct {
	Hash              string `json:"hash"`
	CreatedAtUnixMS   int64  `json:"created_at_unix_ms,omitempty"`
	Thumbnails        bool   `json:"thumbnails"`
	ThumbnailSelected bool   `json:"thumbnail_selected"`
}

// studioPackagesFromKeys reads the layout below packagingObjectKey's directory:
// <hash>.json is a package; thumbnails/<hash>/{manifest,selection}.json are its
// thumbnail review state. Newest package first.
func studioPackagesFromKeys(prefix string, objects []artifactstore.ListedObject) []studioPackageSummary {
	byHash := map[string]*studioPackageSummary{}
	get := func(hash string) *studioPackageSummary {
		if byHash[hash] == nil {
			byHash[hash] = &studioPackageSummary{Hash: hash}
		}
		return byHash[hash]
	}
	for _, obj := range objects {
		rest := strings.TrimPrefix(obj.Key, prefix)
		parts := strings.Split(rest, "/")
		switch {
		case len(parts) == 1 && strings.HasSuffix(rest, ".json") && validEditHash(strings.TrimSuffix(rest, ".json")):
			get(strings.TrimSuffix(rest, ".json")).CreatedAtUnixMS = obj.LastModified.UnixMilli()
		case len(parts) == 3 && parts[0] == "thumbnails" && validEditHash(parts[1]) && parts[2] == "manifest.json":
			get(parts[1]).Thumbnails = true
		case len(parts) == 3 && parts[0] == "thumbnails" && validEditHash(parts[1]) && parts[2] == "selection.json":
			get(parts[1]).ThumbnailSelected = true
		}
	}
	out := make([]studioPackageSummary, 0, len(byHash))
	for _, item := range byHash {
		if item.CreatedAtUnixMS != 0 { // thumbnails without their package are orphans
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAtUnixMS == out[j].CreatedAtUnixMS {
			return out[i].Hash < out[j].Hash
		}
		return out[i].CreatedAtUnixMS > out[j].CreatedAtUnixMS
	})
	return out
}

func (s *replayServer) handlePackagingList(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if strings.TrimSpace(runID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "studio artifact storage disabled"})
		return
	}
	prefix := path.Dir(packagingObjectKey(runID, "x")) + "/"
	objects, err := s.store.List(r.Context(), prefix)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "studio packages unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": studioPackagesFromKeys(prefix, objects)})
}
