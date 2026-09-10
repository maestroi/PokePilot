package main

import (
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
)

// handleArtifactDelete removes the S3 objects owned by one finished run. The
// wall remains the artifact catalog, so this endpoint resolves its targets
// before the operator removes the wall history row. It never accepts arbitrary
// object keys from the browser.
func (s *replayServer) handleArtifactDelete(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	list, err := s.artifacts(r.Context(), runID)
	if err != nil {
		writeReplayError(w, err)
		return
	}

	prefixSet := make(map[string]struct{})
	keySet := make(map[string]struct{})
	for _, artifact := range list.Artifacts {
		if artifact.Store == "" {
			// Inline recordings can still have a derived MP4 in S3.
			if artifact.Replayable && s.store != nil {
				keySet[replayCacheKey(runID, artifact)] = struct{}{}
			}
			continue
		}
		if artifact.Store != "s3" {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "unsupported artifact store " + artifact.Store})
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured for the replay service"})
			return
		}
		if artifact.Bucket != "" && artifact.Bucket != s.store.Bucket() {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "artifact bucket does not match configured replay bucket"})
			return
		}
		key := strings.TrimPrefix(strings.TrimSpace(artifact.ObjectKey), "/")
		if key == "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "remote artifact has an empty object key"})
			return
		}

		// Recordings are stored below one hashed run directory. Purging that
		// directory also catches recordings from earlier attempts and every
		// replay MP4 derived beside them. If a legacy/non-standard key does not
		// match that shape, fall back to deleting only the referenced object.
		if artifact.Name == "run.gbrun" {
			if prefix, ok := recordingRunPrefix(key); ok {
				prefixSet[prefix] = struct{}{}
			} else {
				keySet[key] = struct{}{}
				keySet[replayCacheKey(runID, artifact)] = struct{}{}
			}
			continue
		}
		keySet[key] = struct{}{}
	}

	prefixes := sortedSet(prefixSet)
	keys := sortedSet(keySet)
	filteredKeys := keys[:0]
	for _, key := range keys {
		if !coveredByPrefix(key, prefixes) {
			filteredKeys = append(filteredKeys, key)
		}
	}
	keys = filteredKeys

	deleted := 0
	if len(prefixes) > 0 || len(keys) > 0 {
		if s.store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "S3 artifact storage is not configured for the replay service"})
			return
		}
		for _, prefix := range prefixes {
			n, err := s.store.DeletePrefix(r.Context(), prefix)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			deleted += n
		}
		for _, key := range keys {
			if err := s.store.DeleteObject(r.Context(), key); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			deleted++
		}
	}

	// Forget local replay status for objects that were just purged. A later
	// status request will consult S3 again instead of surfacing stale state.
	s.mu.Lock()
	for key := range s.jobs {
		if coveredByPrefix(key, prefixes) || containsKey(keys, key) {
			delete(s.jobs, key)
		}
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          runID,
		"status":          "purged",
		"deleted_objects": deleted,
	})
}

// recordingRunPrefix accepts only the canonical runner-owned key shape. This
// makes broad prefix deletion impossible for malformed or legacy metadata.
func recordingRunPrefix(key string) (string, bool) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" || path.Clean(key) != key || path.Base(key) != "run.gbrun" {
		return "", false
	}
	attemptDir := path.Dir(key)
	attemptName := path.Base(attemptDir)
	if !strings.HasPrefix(attemptName, "attempt-") {
		return "", false
	}
	attempt, err := strconv.Atoi(strings.TrimPrefix(attemptName, "attempt-"))
	if err != nil || attempt < 1 {
		return "", false
	}
	runDir := path.Dir(attemptDir)
	if path.Dir(runDir) != "runs" || path.Base(runDir) == "." || path.Base(runDir) == "" {
		return "", false
	}
	return runDir + "/", true
}

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func coveredByPrefix(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func containsKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}
