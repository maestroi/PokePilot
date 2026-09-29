package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	Paused  bool                     `json:"paused"`
	LiveNow *publicProgrammingEntry  `json:"live_now,omitempty"`
	UpNext  *publicProgrammingEntry  `json:"up_next,omitempty"`
	Future  []publicProgrammingEntry `json:"future"`
}

type wallProgrammingEntry struct {
	ID               string   `json:"id"`
	ChallengeID      string   `json:"challenge_id"`
	ChallengeVersion int      `json:"challenge_version"`
	ChallengeName    string   `json:"challenge_name"`
	State            string   `json:"state"`
	ScheduledAt      int64    `json:"scheduled_at,omitempty"`
	StartedAt        int64    `json:"started_at,omitempty"`
	RunIDs           []string `json:"run_ids,omitempty"`
}

func publicProgramming(in *wallProgrammingEntry) *publicProgrammingEntry {
	if in == nil {
		return nil
	}
	return &publicProgrammingEntry{
		ID: in.ID, ChallengeID: in.ChallengeID, ChallengeVersion: in.ChallengeVersion,
		ChallengeName: in.ChallengeName, State: in.State, ScheduledAt: in.ScheduledAt,
		StartedAt: in.StartedAt, RunIDs: append([]string(nil), in.RunIDs...),
	}
}

const spectatorVoteCookie = "rompilot_vote_id"

func spectatorVoteIdentity(res http.ResponseWriter, req *http.Request) (string, error) {
	if cookie, err := req.Cookie(spectatorVoteCookie); err == nil {
		value := strings.TrimSpace(cookie.Value)
		if len(value) == 32 {
			if _, err := hex.DecodeString(value); err == nil {
				return value, nil
			}
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	value := hex.EncodeToString(raw)
	secure := req.TLS != nil || strings.EqualFold(strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")), "https")
	http.SetCookie(res, &http.Cookie{
		Name: spectatorVoteCookie, Value: value, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int((365 * 24 * time.Hour) / time.Second),
	})
	return value, nil
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
			Paused  bool                   `json:"paused"`
			LiveNow *wallProgrammingEntry  `json:"live_now,omitempty"`
			UpNext  *wallProgrammingEntry  `json:"up_next,omitempty"`
			Queue   []wallProgrammingEntry `json:"queue"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&source); err != nil {
			writeUnreachable(res)
			return
		}
		out := publicProgrammingSnapshot{
			Paused: source.Paused, LiveNow: publicProgramming(source.LiveNow),
			UpNext: publicProgramming(source.UpNext), Future: []publicProgrammingEntry{},
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
	mux.HandleFunc("GET /v1/watch/vote", func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/votes/active", nil)
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
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		res.WriteHeader(http.StatusOK)
		_, _ = io.Copy(res, io.LimitReader(response.Body, 2<<20))
	})
	mux.HandleFunc("POST /v1/watch/vote/{id}/ballots", func(res http.ResponseWriter, req *http.Request) {
		var body struct {
			CandidateID string `json:"candidate_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(res, req.Body, 32<<10)).Decode(&body); err != nil {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(res).Encode(map[string]string{"error": "invalid ballot"})
			return
		}
		body.CandidateID = strings.TrimSpace(body.CandidateID)
		if body.CandidateID == "" {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(res).Encode(map[string]string{"error": "candidate_id is required"})
			return
		}
		voterID, err := spectatorVoteIdentity(res, req)
		if err != nil {
			writeUnreachable(res)
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"source": "spectator", "voter_id": voterID, "candidate_id": body.CandidateID,
		})
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()
		up, err := http.NewRequestWithContext(ctx, http.MethodPost, wallBase+"/v1/votes/"+req.PathValue("id")+"/ballots", bytes.NewReader(payload))
		if err != nil {
			writeUnreachable(res)
			return
		}
		up.Header.Set("Content-Type", "application/json")
		response, err := client.Do(up)
		if err != nil {
			writeUnreachable(res)
			return
		}
		defer response.Body.Close()
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		res.WriteHeader(response.StatusCode)
		_, _ = io.Copy(res, io.LimitReader(response.Body, 2<<20))
	})

	mux.Handle("/", next)
	return mux
}
