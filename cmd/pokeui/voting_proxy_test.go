package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpectatorVoteProxyForcesSourceAndAnonymousCookie(t *testing.T) {
	var got map[string]string
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/votes/vote-123/ballots" {
			http.Error(res, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(res).Encode(map[string]any{"accepted": true})
	}))
	defer wall.Close()

	h := spectatorProgrammingHTTPHandler(wall.URL, http.NotFoundHandler())
	body := []byte(`{"candidate_id":"mewtwo","source":"youtube","voter_id":"spoofed"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/watch/vote/vote-123/ballots", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	if got["candidate_id"] != "mewtwo" {
		t.Fatalf("candidate = %q", got["candidate_id"])
	}
	if got["source"] != "spectator" {
		t.Fatalf("source = %q, want spectator", got["source"])
	}
	if got["voter_id"] == "" || got["voter_id"] == "spoofed" {
		t.Fatalf("voter_id = %q, want server-generated identity", got["voter_id"])
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != spectatorVoteCookie || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestSpectatorVoteProxyReusesAnonymousCookie(t *testing.T) {
	var voterIDs []string
	wall := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		voterIDs = append(voterIDs, body["voter_id"])
		res.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(res).Encode(map[string]any{"accepted": true})
	}))
	defer wall.Close()

	h := spectatorProgrammingHTTPHandler(wall.URL, http.NotFoundHandler())
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/v1/watch/vote/vote-123/ballots", bytes.NewBufferString(`{"candidate_id":"a"}`)))
	cookies := first.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("first vote did not set identity cookie")
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/watch/vote/vote-123/ballots", bytes.NewBufferString(`{"candidate_id":"a"}`))
	secondReq.AddCookie(cookies[0])
	second := httptest.NewRecorder()
	h.ServeHTTP(second, secondReq)

	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d: %s", second.Code, second.Body.String())
	}
	if len(voterIDs) != 2 || voterIDs[0] == "" || voterIDs[0] != voterIDs[1] {
		t.Fatalf("voter ids = %+v", voterIDs)
	}
}
