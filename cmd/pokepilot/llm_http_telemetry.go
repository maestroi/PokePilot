package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// llmHTTPCallTelemetry is one /chat/completions request as observed at the
// HTTP transport boundary. Usage is OpenAI-compatible; Timings is the
// llama.cpp extension returned by its OpenAI-compatible server. Keeping this
// observer outside agent/llm.go means other OpenAI-compatible servers continue
// to work unchanged: they simply populate usage and leave the llama timings
// empty.
type llmHTTPCallTelemetry struct {
	Seq                uint64
	Endpoint           string
	ResponseModel      string
	PromptTokens       int
	CompletionTokens   int
	CachedPromptTokens int
	PrefillMS          float64
	PrefillTPS         float64
	DecodeMS           float64
	DecodeTPS          float64
	OverheadMS         float64
	TimingSource       string
	WallSeconds        float64
	TransportError     string
}

type llamaTimings struct {
	CacheN             int     `json:"cache_n"`
	PromptN            int     `json:"prompt_n"`
	PromptMS           float64 `json:"prompt_ms"`
	PromptPerSecond    float64 `json:"prompt_per_second"`
	PredictedN         int     `json:"predicted_n"`
	PredictedMS        float64 `json:"predicted_ms"`
	PredictedPerSecond float64 `json:"predicted_per_second"`
}

type telemetryEnvelope struct {
	Model string `json:"model"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
	Timings *llamaTimings `json:"timings,omitempty"`
}

var llmTelemetryState struct {
	sync.Mutex
	seq  uint64
	last llmHTTPCallTelemetry
}

// The planner creates its own http.Client when no custom client is supplied.
// Those clients use http.DefaultTransport, so wrapping it preserves each
// planner's existing per-request timeout while letting us observe the raw
// OpenAI-compatible response envelope before agent/llm.go consumes it.
func init() {
	http.DefaultTransport = &llmTelemetryTransport{base: http.DefaultTransport}
}

type llmTelemetryTransport struct {
	base http.RoundTripper
}

func (t *llmTelemetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	if !isLLMCompletionRequest(req) {
		return resp, err
	}
	endpoint := req.URL.Scheme + "://" + req.URL.Host
	if err != nil {
		recordLLMTelemetry(llmHTTPCallTelemetry{
			Endpoint:       endpoint,
			WallSeconds:    time.Since(start).Seconds(),
			TransportError: err.Error(),
		})
		return nil, err
	}
	resp.Body = &llmTelemetryBody{
		ReadCloser: resp.Body,
		endpoint:   endpoint,
		start:      start,
	}
	return resp, nil
}

func (t *llmTelemetryTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func isLLMCompletionRequest(req *http.Request) bool {
	return req != nil && req.URL != nil && strings.HasSuffix(req.URL.Path, "/chat/completions")
}

type llmTelemetryBody struct {
	io.ReadCloser
	endpoint string
	start    time.Time
	buf      bytes.Buffer
	readErr  error
	once     sync.Once
}

func (b *llmTelemetryBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.buf.Write(p[:n])
	}
	if err != nil && err != io.EOF {
		b.readErr = err
	}
	return n, err
}

func (b *llmTelemetryBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		t := parseLLMTelemetry(b.buf.Bytes(), b.endpoint, time.Since(b.start))
		if b.readErr != nil {
			t.TransportError = b.readErr.Error()
		} else if err != nil {
			t.TransportError = err.Error()
		}
		recordLLMTelemetry(t)
	})
	return err
}

func parseLLMTelemetry(data []byte, endpoint string, wall time.Duration) llmHTTPCallTelemetry {
	t := llmHTTPCallTelemetry{Endpoint: endpoint, WallSeconds: wall.Seconds()}
	var env telemetryEnvelope
	if json.Unmarshal(data, &env) != nil {
		return t
	}
	t.ResponseModel = env.Model
	if env.Usage != nil {
		t.PromptTokens = env.Usage.PromptTokens
		t.CompletionTokens = env.Usage.CompletionTokens
	}
	if env.Timings == nil {
		return t
	}
	t.TimingSource = "llama.cpp"
	t.CachedPromptTokens = env.Timings.CacheN
	t.PrefillMS = env.Timings.PromptMS
	t.PrefillTPS = env.Timings.PromptPerSecond
	t.DecodeMS = env.Timings.PredictedMS
	t.DecodeTPS = env.Timings.PredictedPerSecond
	if t.PromptTokens == 0 {
		t.PromptTokens = env.Timings.PromptN
	}
	if t.CompletionTokens == 0 {
		t.CompletionTokens = env.Timings.PredictedN
	}
	overhead := wall.Seconds()*1000 - t.PrefillMS - t.DecodeMS
	if overhead > 0 {
		t.OverheadMS = overhead
	}
	return t
}

func recordLLMTelemetry(t llmHTTPCallTelemetry) {
	llmTelemetryState.Lock()
	defer llmTelemetryState.Unlock()
	llmTelemetryState.seq++
	t.Seq = llmTelemetryState.seq
	llmTelemetryState.last = t
}

func currentLLMTelemetrySeq() uint64 {
	llmTelemetryState.Lock()
	defer llmTelemetryState.Unlock()
	return llmTelemetryState.seq
}

func latestLLMTelemetryAfter(seq uint64) (llmHTTPCallTelemetry, bool) {
	llmTelemetryState.Lock()
	defer llmTelemetryState.Unlock()
	if llmTelemetryState.last.Seq <= seq {
		return llmHTTPCallTelemetry{}, false
	}
	return llmTelemetryState.last, true
}
