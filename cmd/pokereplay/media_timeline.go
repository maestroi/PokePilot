package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/maestroi/pokepilot/farm"
)

const maxMediaTimelineBytes = 8 << 20

var errMediaTimelineNotFound = errors.New("run has no media-timeline.json artifact")

// mediaTimeline resolves and validates the durable semantic companion to a
// replay. It deliberately uses the same artifact catalog/store seam as
// run.gbrun so renderers do not learn pokewall persistence internals.
func (s *replayServer) mediaTimeline(ctx context.Context, runID string) (farm.MediaTimeline, error) {
	return s.mediaTimelineAttempt(ctx, runID, 0)
}

func (s *replayServer) mediaTimelineAttempt(ctx context.Context, runID string, attempt int) (farm.MediaTimeline, error) {
	list, err := s.artifactsAttempt(ctx, runID, attempt)
	if err != nil {
		return farm.MediaTimeline{}, err
	}
	artifact, ok := findArtifact(list.Artifacts, farm.MediaTimelineArtifactName)
	if !ok {
		return farm.MediaTimeline{}, errMediaTimelineNotFound
	}
	data, err := s.readMediaTimelineArtifact(ctx, runID, artifact, attempt)
	if err != nil {
		return farm.MediaTimeline{}, err
	}
	if want := strings.ToLower(strings.TrimSpace(artifact.SHA256)); want != "" {
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			return farm.MediaTimeline{}, fmt.Errorf("media timeline sha256 mismatch: got %s want %s", got, want)
		}
	}
	timeline, err := farm.DecodeMediaTimeline(data)
	if err != nil {
		return farm.MediaTimeline{}, err
	}
	if timeline.Run.RunID != runID {
		return farm.MediaTimeline{}, fmt.Errorf("media timeline run_id %q does not match requested run %q", timeline.Run.RunID, runID)
	}
	return timeline, nil
}

func (s *replayServer) mediaTimelineOrEmpty(ctx context.Context, runID string, attempt int) (farm.MediaTimeline, error) {
	timeline, err := s.mediaTimelineAttempt(ctx, runID, attempt)
	if errors.Is(err, errMediaTimelineNotFound) {
		return farm.MediaTimeline{
			Run:             farm.MediaRunSummary{RunID: runID},
			Attempt:         attempt,
			FramesPerSecond: farm.GameBoyFramesPerSecond,
		}.Normalized(), nil
	}
	return timeline, err
}

func (s *replayServer) readMediaTimelineArtifact(ctx context.Context, runID string, artifact artifactRef, attempt int) ([]byte, error) {
	var reader io.ReadCloser
	if artifact.Store == "" {
		endpoint := s.wallBase + "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(artifact.Name) + "/content"
		if attempt > 0 {
			endpoint += "?attempt=" + fmt.Sprint(attempt)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		res, err := s.wallHTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("pokewall media timeline unavailable: %w", err)
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			defer res.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return nil, fmt.Errorf("pokewall media timeline returned %s: %s", res.Status, strings.TrimSpace(string(body)))
		}
		reader = res.Body
	} else {
		if artifact.Store != farm.ArtifactStoreS3 {
			return nil, fmt.Errorf("unsupported media timeline store %q", artifact.Store)
		}
		if s.store == nil {
			return nil, fmt.Errorf("S3 artifact storage is not configured")
		}
		if artifact.Bucket != "" && artifact.Bucket != s.store.Bucket() {
			return nil, fmt.Errorf("media timeline bucket %q does not match configured bucket %q", artifact.Bucket, s.store.Bucket())
		}
		obj, err := s.store.GetObject(ctx, artifact.ObjectKey, "")
		if err != nil {
			return nil, err
		}
		reader = obj.Body
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxMediaTimelineBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMediaTimelineBytes {
		return nil, fmt.Errorf("media timeline exceeds %d bytes", maxMediaTimelineBytes)
	}
	return data, nil
}
