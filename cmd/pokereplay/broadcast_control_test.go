package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testBroadcastEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestBroadcastDestinationsStayServerSide(t *testing.T) {
	const twitchKey = "twitch-super-secret"
	const genericKey = "generic-super-secret"
	destinations := broadcastDestinationsFromEnv(testBroadcastEnv(map[string]string{
		"POKEPILOT_BROADCAST_TWITCH_STREAM_KEY":  twitchKey,
		"POKEPILOT_BROADCAST_GENERIC_ENDPOINT":   "rtmps://relay.example.test/live",
		"POKEPILOT_BROADCAST_GENERIC_STREAM_KEY": genericKey,
	}))

	if !destinations["twitch"].Configured || !destinations["generic"].Configured {
		t.Fatalf("configured destinations=%+v", destinations)
	}
	if destinations["youtube"].Configured {
		t.Fatal("YouTube should remain unconfigured without its server-side key")
	}

	server := &replayServer{broadcastDestinations: destinations}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/live/broadcast/config", nil)
	server.handleRTMPBroadcastConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{twitchKey, genericKey, "/live"} {
		if strings.Contains(body, secret) {
			t.Fatalf("public config leaked %q: %s", secret, body)
		}
	}

	var view broadcastControlView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.RestartBehavior != "manual_restart_required" {
		t.Fatalf("restart_behavior=%q", view.RestartBehavior)
	}
	if len(view.Destinations) != 3 {
		t.Fatalf("destinations=%+v", view.Destinations)
	}
}

func TestResolveBroadcastStartUsesConfiguredSecretAndAllowsSafeOverrides(t *testing.T) {
	const key = "server-only-key"
	server := &replayServer{
		broadcastDestinations: broadcastDestinationsFromEnv(testBroadcastEnv(map[string]string{
			"POKEPILOT_BROADCAST_TWITCH_STREAM_KEY": key,
		})),
	}
	cfg, err := server.resolveRTMPBroadcastStart(rtmpBroadcastStartRequest{
		Provider:         "twitch",
		Width:            1920,
		Height:           1080,
		FPS:              30,
		VideoBitrateKbps: 6000,
		Preset:           "fast",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StreamKey != key {
		t.Fatal("resolved broadcaster did not use the server-side credential")
	}
	if cfg.Width != 1920 || cfg.Height != 1080 || cfg.VideoBitrateKbps != 6000 || cfg.Preset != "fast" {
		t.Fatalf("safe overrides were not applied: %+v", cfg)
	}
}

func TestResolveBroadcastStartReportsMissingAndInvalidServerConfiguration(t *testing.T) {
	server := &replayServer{
		broadcastDestinations: broadcastDestinationsFromEnv(testBroadcastEnv(nil)),
	}
	if _, err := server.resolveRTMPBroadcastStart(rtmpBroadcastStartRequest{Provider: "twitch"}); err == nil || !strings.Contains(err.Error(), "server-side Twitch stream key") {
		t.Fatalf("missing Twitch credential err=%v", err)
	}

	server.broadcastDestinations = broadcastDestinationsFromEnv(testBroadcastEnv(map[string]string{
		"POKEPILOT_BROADCAST_GENERIC_ENDPOINT": "https://not-rtmp.example.test/live",
	}))
	destination := server.broadcastDestinations["generic"]
	if destination.Configured || destination.Reason != "server-side broadcast destination is invalid" {
		t.Fatalf("invalid generic destination=%+v", destination)
	}
}

func TestBroadcastStartAPIRejectsSecretsFromBrowser(t *testing.T) {
	server := newReplayServer("http://wall.invalid", "", "", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/runs/run-live/live/broadcast/start",
		strings.NewReader(`{"provider":"twitch","stream_key":"must-not-round-trip"}`),
	)
	req.SetPathValue("id", "run-live")
	server.handleRTMPBroadcastStart(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "must-not-round-trip") {
		t.Fatalf("error response leaked rejected secret: %s", rec.Body.String())
	}
}

func TestReplayRestartKeepsDestinationConfigButNotActiveBroadcasts(t *testing.T) {
	t.Setenv("POKEPILOT_BROADCAST_TWITCH_STREAM_KEY", "restart-secret")
	first := newReplayServer("http://wall.invalid", "", "", nil)
	if !first.broadcastDestinations["twitch"].Configured {
		t.Fatal("first server did not load configured Twitch destination")
	}
	first.rtmpSessions["run-live"] = &rtmpBroadcastSession{
		runID:  "run-live",
		status: rtmpBroadcastStatus{RunID: "run-live", State: "live"},
	}

	restarted := newReplayServer("http://wall.invalid", "", "", nil)
	if !restarted.broadcastDestinations["twitch"].Configured {
		t.Fatal("restarted server did not reload destination configuration")
	}
	if restarted.rtmpBroadcastIfPresent("run-live") != nil {
		t.Fatal("broadcast session unexpectedly survived process restart")
	}
	if got := restarted.broadcastControlSnapshot().RestartBehavior; got != "manual_restart_required" {
		t.Fatalf("restart behavior=%q", got)
	}
}
