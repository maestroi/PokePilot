package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
)

type durableArtifact struct {
	meta   farm.Artifact
	inline []byte
}

type durableFinish struct {
	report   farm.FinishReport
	artifacts []durableArtifact
	uploaded []string
	store    *artifactstore.S3
}

func (d *durableFinish) cleanupUploads() {
	if d == nil || d.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, key := range d.uploaded {
		_ = d.store.DeleteObject(ctx, key)
	}
}

func durabilizeFinishReport(report farm.FinishReport, attempt int) (*durableFinish, error) {
	store, configured, err := artifactstore.S3FromEnv()
	if err != nil {
		return nil, fmt.Errorf("finish artifact S3: %w", err)
	}
	result := &durableFinish{report: report, store: store}
	result.report.Artifacts = append([]farm.Artifact(nil), report.Artifacts...)

	// FinishReport historically carried these two binary payloads directly.
	// Promote them into ordinary artifacts so the durable representation can
	// keep their hashes/object keys without placing bytes in PostgreSQL.
	if len(report.SaveState) > 0 {
		sum := sha256.Sum256(report.SaveState)
		result.report.Artifacts = append(result.report.Artifacts, farm.Artifact{
			Name: "final.state", MediaType: "application/octet-stream",
			SHA256: hex.EncodeToString(sum[:]), Data: append([]byte(nil), report.SaveState...),
		})
	}
	if len(report.FramePNG) > 0 {
		sum := sha256.Sum256(report.FramePNG)
		result.report.Artifacts = append(result.report.Artifacts, farm.Artifact{
			Name: "final-frame.png", MediaType: "image/png",
			SHA256: hex.EncodeToString(sum[:]), Data: append([]byte(nil), report.FramePNG...),
		})
	}
	result.report.SaveState = nil
	result.report.FramePNG = nil

	if err := farm.ValidateFinishArtifacts(farm.FinishReport{Artifacts: result.report.Artifacts}); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for i, art := range result.report.Artifacts {
		if _, duplicate := seen[art.Name]; duplicate {
			result.cleanupUploads()
			return nil, fmt.Errorf("duplicate durable finish artifact %q", art.Name)
		}
		seen[art.Name] = struct{}{}
		meta := art
		var inline []byte
		switch {
		case art.Store == farm.ArtifactStoreS3:
			meta.Data = nil
		case isSmallStructuredArtifact(art):
			inline = append([]byte(nil), art.Data...)
			meta.Data = nil
		default:
			if !configured || store == nil {
				result.cleanupUploads()
				return nil, fmt.Errorf("S3 is required for finish artifact %s", art.Name)
			}
			key := finishObjectKey(report.RunID, attempt, art.Name)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			obj, uploadErr := store.PutObject(ctx, key, art.MediaType, art.Data)
			cancel()
			if uploadErr != nil {
				result.cleanupUploads()
				return nil, fmt.Errorf("upload finish artifact %s: %w", art.Name, uploadErr)
			}
			result.uploaded = append(result.uploaded, key)
			meta = farm.Artifact{
				Name: art.Name, MediaType: art.MediaType, SHA256: obj.SHA256,
				Store: farm.ArtifactStoreS3, Bucket: obj.Bucket, ObjectKey: obj.Key, Size: obj.Size,
			}
		}
		result.report.Artifacts[i] = meta
		result.artifacts = append(result.artifacts, durableArtifact{meta: meta, inline: inline})
	}
	return result, nil
}

func finishObjectKey(runID string, attempt int, name string) string {
	prefix := safeBase(runID)
	if len(prefix) > 64 {
		prefix = prefix[:64]
	}
	sum := sha256.Sum256([]byte(runID))
	return fmt.Sprintf("runs/%s-%s/attempt-%d/finish/%s", prefix, hex.EncodeToString(sum[:6]), attempt, name)
}
