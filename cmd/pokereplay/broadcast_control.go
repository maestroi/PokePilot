package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type rtmpBroadcastStartRequest struct {
	Provider                string `json:"provider"`
	Width                   int    `json:"width,omitempty"`
	Height                  int    `json:"height,omitempty"`
	FPS                     int    `json:"fps,omitempty"`
	VideoBitrateKbps        int    `json:"video_bitrate_kbps,omitempty"`
	Codec                   string `json:"codec,omitempty"`
	Preset                  string `json:"preset,omitempty"`
	KeyframeIntervalSeconds int    `json:"keyframe_interval_seconds,omitempty"`
	Audio                   *bool  `json:"audio,omitempty"`
	AudioBitrateKbps        int    `json:"audio_bitrate_kbps,omitempty"`
	AudioSampleRate         int    `json:"audio_sample_rate,omitempty"`
}

type broadcastDestination struct {
	Provider   string
	Label      string
	Request    rtmpBroadcastRequest
	Configured bool
	Host       string
	Reason     string
}

type broadcastDestinationView struct {
	Provider   string `json:"provider"`
	Label      string `json:"label"`
	Configured bool   `json:"configured"`
	Host       string `json:"host,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type broadcastControlView struct {
	Destinations    []broadcastDestinationView `json:"destinations"`
	RestartBehavior string                     `json:"restart_behavior"`
}

func broadcastDestinationsFromEnv(getenv func(string) string) map[string]broadcastDestination {
	twitchEndpoint := strings.TrimSpace(getenv("POKEPILOT_BROADCAST_TWITCH_ENDPOINT"))
	if twitchEndpoint == "" {
		twitchEndpoint = "rtmps://live.twitch.tv/app"
	}
	youtubeEndpoint := strings.TrimSpace(getenv("POKEPILOT_BROADCAST_YOUTUBE_ENDPOINT"))
	if youtubeEndpoint == "" {
		youtubeEndpoint = "rtmps://a.rtmp.youtube.com/live2"
	}

	out := map[string]broadcastDestination{
		"twitch": {
			Provider: "twitch",
			Label:    "Twitch",
			Request: rtmpBroadcastRequest{
				Provider:  "twitch",
				Endpoint:  twitchEndpoint,
				StreamKey: strings.TrimSpace(getenv("POKEPILOT_BROADCAST_TWITCH_STREAM_KEY")),
			},
		},
		"youtube": {
			Provider: "youtube",
			Label:    "YouTube Live",
			Request: rtmpBroadcastRequest{
				Provider:  "youtube",
				Endpoint:  youtubeEndpoint,
				StreamKey: strings.TrimSpace(getenv("POKEPILOT_BROADCAST_YOUTUBE_STREAM_KEY")),
			},
		},
		"generic": {
			Provider: "generic",
			Label:    "Generic RTMP(S)",
			Request: rtmpBroadcastRequest{
				Provider:  "generic",
				Endpoint:  strings.TrimSpace(getenv("POKEPILOT_BROADCAST_GENERIC_ENDPOINT")),
				StreamKey: strings.TrimSpace(getenv("POKEPILOT_BROADCAST_GENERIC_STREAM_KEY")),
			},
		},
	}

	for provider, destination := range out {
		destination.Host = broadcastDestinationHost(destination.Request.Endpoint)
		switch provider {
		case "twitch":
			if destination.Request.StreamKey == "" {
				destination.Reason = "server-side Twitch stream key is not configured"
			}
		case "youtube":
			if destination.Request.StreamKey == "" {
				destination.Reason = "server-side YouTube Live stream key is not configured"
			}
		case "generic":
			if destination.Request.Endpoint == "" {
				destination.Reason = "server-side generic RTMP(S) endpoint is not configured"
			}
		}
		if destination.Reason == "" {
			if _, err := normalizeRTMPBroadcastRequest(destination.Request); err != nil {
				destination.Reason = "server-side broadcast destination is invalid"
			} else {
				destination.Configured = true
			}
		}
		out[provider] = destination
	}
	return out
}

func broadcastDestinationHost(endpoint string) string {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Host
}

func (s *replayServer) broadcastControlSnapshot() broadcastControlView {
	view := broadcastControlView{
		RestartBehavior: "manual_restart_required",
		Destinations:    make([]broadcastDestinationView, 0, 3),
	}
	for _, provider := range []string{"twitch", "youtube", "generic"} {
		destination := s.broadcastDestinations[provider]
		view.Destinations = append(view.Destinations, broadcastDestinationView{
			Provider:   destination.Provider,
			Label:      destination.Label,
			Configured: destination.Configured,
			Host:       destination.Host,
			Reason:     destination.Reason,
		})
	}
	return view
}

func (s *replayServer) resolveRTMPBroadcastStart(req rtmpBroadcastStartRequest) (rtmpBroadcastConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "youtube_live" {
		provider = "youtube"
	}
	if provider == "" {
		return rtmpBroadcastConfig{}, errors.New("provider is required")
	}
	destination, ok := s.broadcastDestinations[provider]
	if !ok {
		return rtmpBroadcastConfig{}, errors.New("provider must be twitch, youtube, or generic")
	}
	if !destination.Configured {
		if destination.Reason != "" {
			return rtmpBroadcastConfig{}, errors.New(destination.Reason)
		}
		return rtmpBroadcastConfig{}, errors.New("broadcast destination is not configured")
	}

	base := destination.Request
	base.Provider = provider
	base.Width = req.Width
	base.Height = req.Height
	base.FPS = req.FPS
	base.VideoBitrateKbps = req.VideoBitrateKbps
	base.Codec = req.Codec
	base.Preset = req.Preset
	base.KeyframeIntervalSeconds = req.KeyframeIntervalSeconds
	base.Audio = req.Audio
	base.AudioBitrateKbps = req.AudioBitrateKbps
	base.AudioSampleRate = req.AudioSampleRate
	return normalizeRTMPBroadcastRequest(base)
}

func (s *replayServer) handleRTMPBroadcastConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.broadcastControlSnapshot())
}
