// Command mcp-client is a protocol-level smoke client for PokePilot's remote
// MCP endpoint. It lists tools only; it never starts or cancels a run.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func main() {
	endpoint := strings.TrimSpace(os.Getenv("POKEPILOT_MCP_URL"))
	if endpoint == "" {
		endpoint = "https://pokemon.labstack.cc/mcp"
	}
	token := strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN"))
	if token == "" {
		log.Fatal("POKEPILOT_MCP_TOKEN is required")
	}

	httpClient := &http.Client{Transport: bearerTransport{
		token: token,
		base:  http.DefaultTransport,
	}}
	client := mcp.NewClient(&mcp.Implementation{Name: "pokepilot-example-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		log.Fatalf("connect %s: %v", endpoint, err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		log.Fatalf("list tools: %v", err)
	}
	fmt.Printf("connected to %s\n", endpoint)
	for _, tool := range tools.Tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
	}
}
