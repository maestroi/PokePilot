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
	"strings"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/media/packaging"
	"github.com/maestroi/pokepilot/media/thumbnail"
)

const thumbnailManifestVersion = 1
const thumbnailMaxManifestBytes = 128 << 10

type thumbnailRenderRequest struct {
	Variants     int  `json:"variants"`
	GeneratedArt bool `json:"generated_art,omitempty"`
}
type thumbnailCandidate struct {
	ID            string         `json:"id"`
	ConceptID     string         `json:"concept_id"`
	Variant       int            `json:"variant"`
	Spec          thumbnail.Spec `json:"spec"`
	ObjectKey     string         `json:"object_key"`
	SHA256        string         `json:"sha256"`
	Bytes         int            `json:"bytes"`
	AssetKey      string         `json:"asset_key,omitempty"`
	ArtworkStatus string         `json:"artwork_status"`
}
type thumbnailManifest struct {
	Version     int                  `json:"version"`
	RunID       string               `json:"run_id"`
	PackageHash string               `json:"package_hash"`
	Template    string               `json:"template"`
	Candidates  []thumbnailCandidate `json:"candidates"`
	Hash        string               `json:"hash"`
}
type thumbnailSelection struct {
	Version      int    `json:"version"`
	ManifestHash string `json:"manifest_hash"`
	CandidateID  string `json:"candidate_id"`
	PackageHash  string `json:"package_hash"`
}
type thumbnailListReply struct {
	Manifest   thumbnailManifest `json:"manifest"`
	SelectedID string            `json:"selected_id,omitempty"`
}

func thumbnailRoot(runID, packageHash string) string {
	return path.Join(path.Dir(packagingObjectKey(runID, packageHash)), "thumbnails", packageHash)
}
func thumbnailManifestKey(runID, packageHash string) string {
	return path.Join(thumbnailRoot(runID, packageHash), "manifest.json")
}
func thumbnailSelectionKey(runID, packageHash string) string {
	return path.Join(thumbnailRoot(runID, packageHash), "selection.json")
}
func sealThumbnailManifest(m thumbnailManifest) thumbnailManifest {
	m.Hash = ""
	b, _ := json.Marshal(m)
	m.Hash = thumbnail.Digest(b)
	return m
}
func validateThumbnailManifest(m thumbnailManifest, pkg packaging.Package) error {
	if m.Version != thumbnailManifestVersion || m.RunID != pkg.RunID || m.PackageHash != pkg.Hash || m.Template != thumbnail.TemplateVersion {
		return errors.New("thumbnail: invalid manifest identity")
	}
	hash := m.Hash
	if hash != sealThumbnailManifest(m).Hash {
		return errors.New("thumbnail: manifest checksum mismatch")
	}
	if len(m.Candidates) == 0 || len(m.Candidates) > len(pkg.Thumbnails)*thumbnail.MaxVariants {
		return errors.New("thumbnail: invalid candidate count")
	}
	seen := make(map[string]bool)
	for _, c := range m.Candidates {
		if c.ID != c.Spec.Hash || !validEditHash(c.ID) || seen[c.ID] || c.Spec.ConceptID != c.ConceptID || c.Variant != c.Spec.Variant {
			return errors.New("thumbnail: candidate identity mismatch")
		}
		seen[c.ID] = true
		if err := thumbnail.Validate(c.Spec, pkg); err != nil {
			return err
		}
		if !validEditHash(c.SHA256) || c.Bytes < 1 || c.Bytes > thumbnail.MaxImageBytes ||
			c.ObjectKey != path.Join(thumbnailRoot(pkg.RunID, pkg.Hash), "images", c.ID+".jpg") {
			return errors.New("thumbnail: invalid candidate image metadata")
		}
		if c.Spec.AssetSHA256 == "" && c.AssetKey != "" ||
			c.Spec.AssetSHA256 != "" && c.AssetKey != path.Join(thumbnailRoot(pkg.RunID, pkg.Hash), "artwork", c.Spec.AssetSHA256+".bin") {
			return errors.New("thumbnail: invalid artwork source reference")
		}
	}
	return nil
}
func (s *replayServer) loadVerifiedThumbnailPackage(ctx context.Context, runID, hash string) (packaging.Package, error) {
	var pkg packaging.Package
	if !validEditHash(hash) || strings.TrimSpace(runID) == "" || s.store == nil {
		return pkg, errors.New("thumbnail: invalid package or disabled storage")
	}
	obj, err := s.store.GetObject(ctx, packagingObjectKey(runID, hash), "")
	if err != nil {
		return pkg, err
	}
	defer obj.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(obj.Body, thumbnailMaxManifestBytes+1))
	if err != nil || len(raw) > thumbnailMaxManifestBytes {
		return pkg, errors.New("thumbnail: stored package too large")
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return pkg, errors.New("thumbnail: malformed package")
	}
	if pkg.RunID != runID || pkg.Hash != hash || packaging.VerifyChecksum(pkg) != nil {
		return pkg, errors.New("thumbnail: package checksum invalid")
	}
	a, timelines, plan, err := s.packagingInputs(ctx, runID, pkg.AssessmentHash, pkg.PlanHash)
	if err != nil {
		return pkg, err
	}
	if err := packaging.Validate(pkg, packaging.Request{
		Assessment: a, Timelines: timelines, Plan: plan, Style: pkg.Style,
		RunBaseURL: strings.TrimSpace(os.Getenv("POKEPILOT_PACKAGING_PUBLIC_BASE_URL")),
	}); err != nil {
		return pkg, fmt.Errorf("thumbnail: packaging evidence changed: %w", err)
	}
	return pkg, nil
}
func (s *replayServer) getThumbnailManifest(ctx context.Context, runID string, pkg packaging.Package) (thumbnailManifest, error) {
	var m thumbnailManifest
	obj, err := s.store.GetObject(ctx, thumbnailManifestKey(runID, pkg.Hash), "")
	if err != nil {
		return m, err
	}
	defer obj.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(obj.Body, thumbnailMaxManifestBytes+1))
	if err != nil || len(raw) > thumbnailMaxManifestBytes {
		return m, errors.New("thumbnail: stored manifest too large")
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, errors.New("thumbnail: malformed manifest")
	}
	if err := validateThumbnailManifest(m, pkg); err != nil {
		return m, err
	}
	return m, nil
}
func (s *replayServer) getThumbnailSelection(ctx context.Context, runID string, m thumbnailManifest) (string, error) {
	obj, err := s.store.GetObject(ctx, thumbnailSelectionKey(runID, m.PackageHash), "")
	if artifactstore.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer obj.Body.Close()
	data, err := io.ReadAll(io.LimitReader(obj.Body, 4097))
	if err != nil || len(data) > 4096 {
		return "", errors.New("thumbnail: selection too large")
	}
	var selected thumbnailSelection
	if err := json.Unmarshal(data, &selected); err != nil {
		return "", err
	}
	if selected.Version != 1 || selected.ManifestHash != m.Hash || selected.PackageHash != m.PackageHash {
		return "", nil
	}
	for _, c := range m.Candidates {
		if c.ID == selected.CandidateID {
			return selected.CandidateID, nil
		}
	}
	return "", nil
}
func configuredThumbnailProvider() (thumbnail.ArtworkProvider, error) {
	url := strings.TrimSpace(os.Getenv("POKEPILOT_THUMBNAIL_IMAGE_URL"))
	model := strings.TrimSpace(os.Getenv("POKEPILOT_THUMBNAIL_IMAGE_MODEL"))
	if url == "" && model == "" {
		return nil, nil
	}
	if url == "" || model == "" {
		return nil, errors.New("thumbnail: image URL and model must both be set")
	}
	return thumbnail.ChatImageProvider{
		Endpoint: url, ModelID: model,
		Deployment: strings.TrimSpace(os.Getenv("POKEPILOT_THUMBNAIL_IMAGE_DEPLOYMENT")),
		APIKey:     os.Getenv("POKEPILOT_THUMBNAIL_IMAGE_API_KEY"),
	}, nil
}
func (s *replayServer) handleThumbnailRender(w http.ResponseWriter, r *http.Request) {
	runID, pkgHash := r.PathValue("id"), r.PathValue("hash")
	if r.ContentLength > 4096 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "thumbnail request too large"})
		return
	}
	var args thumbnailRenderRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 4097))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid thumbnail render request"})
		return
	}
	if args.Variants == 0 {
		args.Variants = 2
	}
	if args.Variants < 1 || args.Variants > thumbnail.MaxVariants {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "variants must be 1–3"})
		return
	}
	pkg, err := s.loadVerifiedThumbnailPackage(r.Context(), runID, pkgHash)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "verified source packaging unavailable"})
		return
	}
	provider, err := configuredThumbnailProvider()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !args.GeneratedArt {
		provider = nil
	}
	manifest := thumbnailManifest{
		Version: thumbnailManifestVersion, RunID: runID, PackageHash: pkg.Hash,
		Template: thumbnail.TemplateVersion,
	}
	// Optional image generation occurs once per concept; deterministic themed
	// variants can reuse the exact immutable asset instead of repeated billing.
	for _, concept := range pkg.Thumbnails {
		var asset []byte
		var assetKey string
		providerID, artStatus := "none", "not_requested"
		if args.GeneratedArt && provider == nil {
			artStatus = "unconfigured"
		}
		if provider != nil {
			generated, genErr := provider.Generate(r.Context(), concept, 1)
			if genErr != nil {
				artStatus = "fallback"
			} else {
				asset = generated
				providerID = provider.Identity()
				assetKey = path.Join(thumbnailRoot(runID, pkg.Hash), "artwork", thumbnail.Digest(asset)+".bin")
				if _, err = s.store.PutObject(r.Context(), assetKey, "application/octet-stream", asset); err != nil {
					// A failed provider/asset step never poisons generated text
					// or claims a partial artwork reference.
					artStatus = "fallback"
					asset = nil
					providerID = "none"
					assetKey = ""
				} else {
					artStatus = "generated"
				}
			}
		}
		for v := 1; v <= args.Variants; v++ {
			assetSHA := ""
			if len(asset) > 0 {
				assetSHA = thumbnail.Digest(asset)
			}
			spec, err := thumbnail.NewSpec(pkg, concept.ID, v, providerID, assetSHA)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid concept specification"})
				return
			}
			rendered, err := thumbnail.Render(pkg, spec, asset)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "thumbnail composition failed"})
				return
			}
			imageKey := path.Join(thumbnailRoot(runID, pkg.Hash), "images", spec.Hash+".jpg")
			if _, err = s.store.PutObject(r.Context(), imageKey, "image/jpeg", rendered.JPEG); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "thumbnail image storage failed"})
				return
			}
			manifest.Candidates = append(manifest.Candidates, thumbnailCandidate{
				ID: spec.Hash, ConceptID: concept.ID, Variant: v, Spec: spec, ObjectKey: imageKey,
				SHA256: rendered.SHA256, Bytes: rendered.Size, AssetKey: assetKey,
				ArtworkStatus: artStatus,
			})
		}
	}
	manifest = sealThumbnailManifest(manifest)
	if err := validateThumbnailManifest(manifest, pkg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid generated thumbnail manifest"})
		return
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode thumbnail manifest"})
		return
	}
	if _, err = s.store.PutObject(r.Context(), thumbnailManifestKey(runID, pkgHash), "application/json", raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "thumbnail manifest storage failed"})
		return
	}
	writeJSON(w, http.StatusOK, thumbnailListReply{Manifest: manifest})
}
func (s *replayServer) handleThumbnailList(w http.ResponseWriter, r *http.Request) {
	runID, pkgHash := r.PathValue("id"), r.PathValue("hash")
	pkg, err := s.loadVerifiedThumbnailPackage(r.Context(), runID, pkgHash)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "verified package unavailable"})
		return
	}
	m, err := s.getThumbnailManifest(r.Context(), runID, pkg)
	if artifactstore.IsNotFound(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thumbnails not generated yet"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "thumbnail manifest invalid"})
		return
	}
	selection, err := s.getThumbnailSelection(r.Context(), runID, m)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "thumbnail selection unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, thumbnailListReply{Manifest: m, SelectedID: selection})
}
func (s *replayServer) handleThumbnailImage(w http.ResponseWriter, r *http.Request) {
	runID, pkgHash, candidateID := r.PathValue("id"), r.PathValue("hash"), r.PathValue("candidate")
	if !validEditHash(candidateID) {
		http.Error(w, "invalid thumbnail id", http.StatusBadRequest)
		return
	}
	pkg, err := s.loadVerifiedThumbnailPackage(r.Context(), runID, pkgHash)
	if err != nil {
		http.Error(w, "invalid packaging", http.StatusUnprocessableEntity)
		return
	}
	m, err := s.getThumbnailManifest(r.Context(), runID, pkg)
	if err != nil {
		http.Error(w, "thumbnail manifest unavailable", http.StatusNotFound)
		return
	}
	for _, c := range m.Candidates {
		if c.ID != candidateID {
			continue
		}
		obj, err := s.store.GetObject(r.Context(), c.ObjectKey, "")
		if err != nil {
			http.Error(w, "thumbnail image unavailable", http.StatusNotFound)
			return
		}
		defer obj.Body.Close()
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", fmt.Sprint(c.Bytes))
		_, _ = io.CopyN(w, obj.Body, int64(c.Bytes))
		return
	}
	http.Error(w, "unknown thumbnail candidate", http.StatusNotFound)
}
func (s *replayServer) handleThumbnailSelect(w http.ResponseWriter, r *http.Request) {
	runID, pkgHash, candidateID := r.PathValue("id"), r.PathValue("hash"), r.PathValue("candidate")
	if !validEditHash(candidateID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate ID"})
		return
	}
	pkg, err := s.loadVerifiedThumbnailPackage(r.Context(), runID, pkgHash)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "verified package unavailable"})
		return
	}
	m, err := s.getThumbnailManifest(r.Context(), runID, pkg)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thumbnail manifest unavailable"})
		return
	}
	var found *thumbnailCandidate
	for i := range m.Candidates {
		if m.Candidates[i].ID == candidateID {
			found = &m.Candidates[i]
			break
		}
	}
	if found == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate not in current manifest"})
		return
	}
	if _, err := s.store.HeadObject(r.Context(), found.ObjectKey); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "candidate image unavailable"})
		return
	}
	selected := thumbnailSelection{Version: 1, ManifestHash: m.Hash, CandidateID: candidateID, PackageHash: pkgHash}
	raw, _ := json.Marshal(selected)
	if _, err := s.store.PutObject(r.Context(), thumbnailSelectionKey(runID, pkgHash), "application/json", raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "cannot persist selection"})
		return
	}
	writeJSON(w, http.StatusOK, selected)
}
