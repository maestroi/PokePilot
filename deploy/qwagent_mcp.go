package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultMCPEndpoint = "https://admin.rompilot.app/mcp"

type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type mcpCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type mcpResponse struct {
	Error  *mcpRPCError `json:"error"`
	Result *mcpResult   `json:"result"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpResult struct {
	IsError           bool            `json:"isError"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	Content           []mcpContent    `json:"content"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func DefaultMCPEndpoint() string {
	return defaultMCPEndpoint
}

func CallMCPTool(client *http.Client, endpoint, token, name string, args map[string]any) (json.RawMessage, error) {
	client = withoutRedirects(client)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("MCP endpoint is empty")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("MCP token is empty")
	}
	body, err := json.Marshal(mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: mcpCallParams{
			Name:      name,
			Arguments: args,
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP HTTP %d: %s", res.StatusCode, jsonPreview(raw))
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil, fmt.Errorf("MCP response is not JSON (got %q)", jsonPreview(raw))
	}
	var rpc mcpResponse
	if err := json.Unmarshal(trimmed, &rpc); err != nil {
		return nil, fmt.Errorf("decode MCP response: %w", err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("MCP %s: %s", name, rpc.Error.Message)
	}
	if rpc.Result == nil {
		return nil, fmt.Errorf("MCP %s: empty result", name)
	}
	if rpc.Result.IsError {
		return nil, fmt.Errorf("MCP %s: %s", name, mcpText(rpc.Result))
	}
	if len(rpc.Result.StructuredContent) > 0 && string(rpc.Result.StructuredContent) != "null" {
		return rpc.Result.StructuredContent, nil
	}
	if text := mcpText(rpc.Result); looksLikeJSON(text) {
		return json.RawMessage(text), nil
	}
	return nil, fmt.Errorf("MCP %s: missing structured content", name)
}

func mcpText(result *mcpResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, item := range result.Content {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && (s[0] == '{' || s[0] == '[')
}

func withoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}
