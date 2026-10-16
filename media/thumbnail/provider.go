package thumbnail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/media/packaging"
)

// ArtworkProvider is an optional sandboxed creative step. The deterministic
// compositor never lets it draw text, branding or factual overlays.
type ArtworkProvider interface {
	Identity() string
	Generate(context.Context, packaging.ThumbnailConcept, int) ([]byte, error)
}

// ChatImageProvider accepts the base64 JSON response shape of an
// OpenAI-compatible images/generations endpoint. Its URL is server config;
// callers cannot supply an arbitrary upstream URL through the Studio API.
type ChatImageProvider struct {
	Endpoint   string
	ModelID    string
	Deployment string
	APIKey     string
	Client     *http.Client
}

func (m ChatImageProvider) Identity() string {
	if m.Deployment != "" {
		return "images:" + m.ModelID + ":" + m.Deployment
	}
	return "images:" + m.ModelID
}
func (m ChatImageProvider) Generate(ctx context.Context, concept packaging.ThumbnailConcept, variant int) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(m.Endpoint))
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("thumbnail: image endpoint must be trusted HTTP(S) URL without credentials or query")
	}
	if strings.TrimSpace(m.ModelID) == "" {
		return nil, errors.New("thumbnail: image model missing")
	}
	if variant < 1 || variant > MaxVariants || concept.Evidence.EventID == "" {
		return nil, errors.New("thumbnail: invalid artwork request")
	}
	// Producer validated this public event class and its exact evidence.
	// No private planner traces, prompts, usernames, free-form video text or
	// secrets are passed to an image provider.
	prompt := fmt.Sprintf("Decorative abstract retro handheld adventure illustration. Event theme: %s. Palette variant %d. Widescreen cinematic lighting. Right-focused composition, left half quiet/dark. No words, no lettering, no logos, no watermarks, no game characters, no copyrighted visual assets.", concept.Subject, variant)
	raw, err := json.Marshal(map[string]any{"model": m.ModelID, "prompt": prompt, "size": "1536x1024", "response_format": "b64_json", "n": 1})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("thumbnail: image provider unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("thumbnail: image provider returned HTTP %d", res.StatusCode)
	}
	const maxResponse = 9 << 20
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(body) > maxResponse {
		return nil, errors.New("thumbnail: image provider response unreadable or too large")
	}
	var response struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || len(response.Data) != 1 || response.Data[0].B64JSON == "" {
		return nil, errors.New("thumbnail: image provider returned no base64 artwork")
	}
	data, err := base64.StdEncoding.DecodeString(response.Data[0].B64JSON)
	if err != nil {
		return nil, errors.New("thumbnail: image provider base64 malformed")
	}
	if _, err := DecodeArtwork(data); err != nil {
		return nil, err
	}
	return data, nil
}
