package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type publicProgrammingEntry struct {
	ID               string   `json:"id"`
	ChallengeID      string   `json:"challenge_id"`
	ChallengeVersion int      `json:"challenge_version"`
	ChallengeName    string   `json:"challenge_name"`
	State            string   `json:"state"`
	ScheduledAt      int64    `json:"scheduled_at,omitempty"`
	StartedAt        int64    `json:"started_at,omitempty"`
	RunIDs           []string `json:"run_ids,omitempty"`
}

type publicProgrammingSnapshot struct {
	Paused  bool                    `json:"paused"`
	LiveNow *publicProgrammingEntry `json:"live_now,omitempty"`
	UpNext  *publicProgrammingEntry `json:"up_next,omitempty"`
	Future  []publicProgrammingEntry `json:"future"`
}

func spectatorProgrammingHTTPHandler(wallBase string, next http.Handler) http.Handler {
	client := &http.Client{Timeout: proxyTimeout}
	wallBase = strings.TrimRight(strings.TrimSpace(wallBase), "/")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/watch/programming", func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/programming", nil)
		if err != nil {
			writeUnreachable(res)
			return
		}
		response, err := client.Do(up)
		if err != nil {
			writeUnreachable(res)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			writeUnreachable(res)
			return
		}
		var source struct {
			Paused  bool `json:"paused"`
			LiveNow *struct {
				ID               string   `json:"id"`
				ChallengeID      string   `json:"challenge_id"`
				ChallengeVersion int      `json:"challenge_version"`
				ChallengeName    string   `json:"challenge_name"`
				State            string   `json:"state"`
				ScheduledAt      int64    `json:"scheduled_at,omitempty"`
				StartedAt        int64    `json:"started_at,omitempty"`
				RunIDs           []string `json:"run_ids,omitempty"`
			} `json:"live_now,omitempty"`
			UpNext *struct {
				ID               string   `json:"id"`
				ChallengeID      string   `json:"challenge_id"`
				ChallengeVersion int      `json:"challenge_version"`
				ChallengeName    string   `json:"challenge_name"`
				State            string   `json:"state"`
				ScheduledAt      int64    `json:"scheduled_at,omitempty"`
				StartedAt        int64    `json:"started_at,omitempty"`
				RunIDs           []string `json:"run_ids,omitempty"`
			} `json:"up_next,omitempty"`
			Queue []struct {
				ID               string   `json:"id"`
				ChallengeID      string   `json:"challenge_id"`
				ChallengeVersion int      `json:"challenge_version"`
				ChallengeName    string   `json:"challenge_name"`
				State            string   `json:"state"`
				ScheduledAt      int64    `json:"scheduled_at,omitempty"`
				StartedAt        int64    `json:"started_at,omitempty"`
				RunIDs           []string `json:"run_ids,omitempty"`
			} `json:"queue"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&source); err != nil {
			writeUnreachable(res)
			return
		}
		convert := func(in interface {
		}) {}
		_ = convert
		out := publicProgrammingSnapshot{Paused: source.Paused, Future: []publicProgrammingEntry{}}
		if source.LiveNow != nil {
			out.LiveNow = &publicProgrammingEntry{
				ID: source.LiveNow.ID, ChallengeID: source.LiveNow.ChallengeID, ChallengeVersion: source.LiveNow.ChallengeVersion,
				ChallengeName: source.LiveNow.ChallengeName, State: source.LiveNow.State, ScheduledAt: source.LiveNow.ScheduledAt,
				StartedAt: source.LiveNow.StartedAt, RunIDs: append([]string(nil), source.LiveNow.RunIDs...),
			}
		}
		if source.UpNext != nil {
			out.UpNext = &publicProgrammingEntry{
				ID: source.UpNext.ID, ChallengeID: source.UpNext.ChallengeID, ChallengeVersion: source.UpNext.ChallengeVersion,
				ChallengeName: source.UpNext.ChallengeName, State: source.UpNext.State, ScheduledAt: source.UpNext.ScheduledAt,
				StartedAt: source.UpNext.StartedAt, RunIDs: append([]string(nil), source.UpNext.RunIDs...),
			}
		}
		for _, entry := range source.Queue {
			if entry.State != "queued" && entry.State != "scheduled" && entry.State != "voting" {
				continue
			}
			out.Future = append(out.Future, publicProgrammingEntry{
				ID: entry.ID, ChallengeID: entry.ChallengeID, ChallengeVersion: entry.ChallengeVersion,
				ChallengeName: entry.ChallengeName, State: entry.State, ScheduledAt: entry.ScheduledAt,
			})
		}
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(res).Encode(out)
	})
	mux.Handle("/", next)
	return mux
}

var _ = time.Second
