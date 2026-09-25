package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	protocol "github.com/maestroi/pokepilot/renderstate"
)

const spectatorRenderStateLimit = 4 << 20

func spectatorRenderState(wallBase string) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	wallBase = strings.TrimRight(wallBase, "/")
	return func(res http.ResponseWriter, req *http.Request) {
		runID := strings.TrimSpace(req.URL.Query().Get("run"))
		if runID == "" || len(runID) > 256 {
			http.NotFound(res, req)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		values := url.Values{"run": []string{runID}}
		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/render-state?"+values.Encode(), nil)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeSpectatorUnavailable(res)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound {
				http.NotFound(res, req)
				return
			}
			writeSpectatorUnavailable(res)
			return
		}

		data, err := io.ReadAll(io.LimitReader(resp.Body, spectatorRenderStateLimit+1))
		if err != nil || len(data) == 0 || len(data) > spectatorRenderStateLimit {
			writeSpectatorUnavailable(res)
			return
		}
		if _, err := protocol.ReadJSON(bytes.NewReader(data)); err != nil {
			writeSpectatorUnavailable(res)
			return
		}

		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		res.Write(data) //nolint:errcheck // browser retries on the next tick
	}
}
