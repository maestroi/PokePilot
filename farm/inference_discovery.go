package farm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProbeOpenAIEndpoint verifies an OpenAI-compatible endpoint and, when
// DiscoverModel is enabled, resolves the deployment to the model the endpoint
// actually advertises. Dedicated llama.cpp/vLLM endpoints typically expose a
// single model, which makes model swaps configuration-free for the farm.
//
// Managed switchable hosts use ControlURL and keep their declared deployment
// identity; the wall talks to that control plane instead of probing a model
// which may not be loaded yet.
func ProbeOpenAIEndpoint(ctx context.Context, client *http.Client, d ModelDeployment) (ModelDeployment, error) {
	if strings.TrimSpace(d.Endpoint) == "" {
		return d, fmt.Errorf("deployment %q has no endpoint", d.ID)
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	url := strings.TrimRight(d.Endpoint, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return d, fmt.Errorf("build model discovery request: %w", err)
	}
	if envName := strings.TrimSpace(d.EndpointTokenEnv); envName != "" {
		token := strings.TrimSpace(os.Getenv(envName))
		if token == "" {
			return d, fmt.Errorf("endpoint token environment variable %s is empty", envName)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return d, fmt.Errorf("probe %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return d, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return d, fmt.Errorf("probe %s returned HTTP %s", url, resp.Status)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return d, fmt.Errorf("decode %s: %w", url, err)
	}
	ids := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return d, fmt.Errorf("endpoint %s advertised no models", url)
	}

	configured := strings.TrimSpace(d.APIModel)
	if !d.DiscoverModel {
		for _, id := range ids {
			if id == configured {
				return d, nil
			}
		}
		return d, fmt.Errorf("configured api_model %q is not advertised by endpoint (models: %s)", configured, strings.Join(ids, ", "))
	}

	selected := ""
	if configured != "" {
		for _, id := range ids {
			if id == configured {
				selected = id
				break
			}
		}
	}
	if selected == "" && len(ids) == 1 {
		selected = ids[0]
	}
	if selected == "" && strings.TrimSpace(d.ModelID) != "" {
		for _, id := range ids {
			if id == strings.TrimSpace(d.ModelID) {
				selected = id
				break
			}
		}
	}
	if selected == "" {
		return d, fmt.Errorf("endpoint advertises multiple models (%s); configure api_model to select one", strings.Join(ids, ", "))
	}

	out := d
	configuredAPI := strings.TrimSpace(d.APIModel)
	configuredModel := strings.TrimSpace(d.ModelID)
	out.APIModel = selected

	// API aliases and logical model identities can legitimately differ (for
	// example a stable cloud alias or pokepilot-* alias). If the advertised id
	// matches either configured identity, keep the declared ModelID/revision.
	// Only a genuinely new advertised identity invalidates artifact metadata.
	identityChanged := configuredModel == "" || (selected != configuredAPI && selected != configuredModel)
	if configuredModel == "" || identityChanged {
		out.ModelID = selected
	}
	if identityChanged {
		out.Revision = ""
		out.Artifact = ""
		out.Quantization = ""
	}
	return out, nil
}
