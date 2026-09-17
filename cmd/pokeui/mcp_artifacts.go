package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

const artifactDownloadNote = "Operator REST on this host; MCP is only /mcp and cannot fetch bytes. GET content_url with no Authorization header. Run ids include the run- prefix."

func operatorPublicOrigin() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("POKEPILOT_RUN_BASE_URL")), "/")
}

func annotateOperatorArtifactDownloads(origin, runID string, payload map[string]any) {
	if payload == nil {
		return
	}
	if runID == "" {
		if nested, ok := payload["run"].(map[string]any); ok {
			runID, _ = nested["run_id"].(string)
		}
		if runID == "" {
			runID, _ = payload["run_id"].(string)
		}
	}
	runID = strings.TrimSpace(runID)
	download := map[string]any{
		"method":        http.MethodGet,
		"authorization": "none",
		"note":          artifactDownloadNote,
	}
	if origin != "" {
		payload["http_base"] = origin
		download["openapi"] = origin + "/openapi.json"
	} else {
		download["openapi"] = "/openapi.json"
	}
	payload["download"] = download
	if runID == "" {
		return
	}
	for _, art := range artifactMaps(payload) {
		name, _ := art["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		art["content_url"] = artifactContentURL(origin, runID, name)
	}
}

func artifactMaps(payload map[string]any) []map[string]any {
	switch arts := payload["artifacts"].(type) {
	case []any:
		out := make([]map[string]any, 0, len(arts))
		for _, item := range arts {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return arts
	default:
		return nil
	}
}

func artifactContentURL(origin, runID, name string) string {
	path := "/v1/runs/" + url.PathEscape(runID) + "/artifacts/" + url.PathEscape(name) + "/content"
	if origin == "" {
		return path
	}
	return strings.TrimRight(origin, "/") + path
}

func ensureRunPrefix(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "run-") {
		return id
	}
	return "run-" + id
}

func wallStatusNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "wall returned 404")
}
