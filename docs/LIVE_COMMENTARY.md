# Live Studio commentary (#800)

This is an **opt-in presentation sidecar**, separate from the gameplay planner,
controller, and offline producer. It consumes only the run's public-safe state
and bounded semantic activity from pokewall's read API. It never sends emulator
frames, private planner prompts, hidden model reasoning, raw logs, or issue
debug payloads to an editorial provider.

## Operator workflow

In the private operator UI, open **Media → Broadcasts**, choose an active
run, expand **Live commentary**, and choose the style, cadence, verbosity,
and optional speech voice. Enable commentary **before** starting the RTMP
broadcast if you want speech mixed into its audio track. Stop commentary
independently of the broadcast at any time. Voice/style/cadence changes take
effect on the next commentary session; stop and start it to change settings.
The sidecar restarts with commentary off.

API routes (proxied only on the authenticated/private operator service):

- `GET /v1/runs/{id}/live/commentary/status`: current mode, producer identity,
  safe recent emitted text, speech state and error summary. No secrets or
  private model traces.
- `POST /v1/runs/{id}/live/commentary/start`: enable for a **running** run.
- `POST /v1/runs/{id}/live/commentary/stop`: disable without affecting video
  or gameplay.

Example request:

```json
{
  "enabled": true,
  "style": "documentary",
  "verbosity": "sparse",
  "cadence_seconds": 120,
  "tts": false,
  "voice": ""
}
```

Style must be `documentary`, `sports`, or `calm`. Cadence is 30–3600
seconds; sparse mode doubles the cooldown. There is a five-second minimum
between high-priority milestones. Routine navigation, unknown activity, and
checkpoint spam are always silent. No commentary at all is also valid.

## Providers

The default `rules:live-v1` commentator uses sanitized milestone summaries.
The optional editorial model uses an independent OpenAI-compatible endpoint:

| Variable | Meaning |
| --- | --- |
| `POKEPILOT_LIVE_COMMENTARY_CHAT_URL` | Chat completions URL (full absolute HTTP(S) URL) |
| `POKEPILOT_LIVE_COMMENTARY_MODEL` | Live editorial model ID, separate from gameplay and offline producer |
| `POKEPILOT_LIVE_COMMENTARY_DEPLOYMENT` | Optional deployment/revision identifier |
| `POKEPILOT_LIVE_COMMENTARY_API_KEY` | Optional server-side bearer secret |

The optional speech provider is a compatible `/audio/speech` endpoint with
`response_format: "pcm"`; output must be **24 kHz mono signed 16-bit little
endian PCM** (OpenAI speech-compatible). It needs:

| Variable | Meaning |
| --- | --- |
| `POKEPILOT_LIVE_TTS_URL` | Full HTTP(S) speech endpoint |
| `POKEPILOT_LIVE_TTS_MODEL` | TTS model identifier |
| `POKEPILOT_LIVE_TTS_API_KEY` | Optional server-side bearer secret |
| `POKEPILOT_LIVE_TTS_VOICE` | Server-side fallback voice (default `alloy`) |

TTS is an independent HTTP call with a 12-second deadline and a 12-second
PCM output cap. Provider errors, malformed responses, encoder faults, and
full queues are swallowed by the presentation sidecar; gameplay is unaffected.
On TTS failure, the stream continues with silence and the safely generated
text remains inspectable. The UI never sends credentials.

When commentary is already running and TTS is enabled, **start the broadcast
with its audio track enabled**. RTMP's normal silent audio input is then
replaced by a continuously padded PCM source on FFmpeg fd 3; sparse speech is
mixed into the outgoing AAC track. The PCM queue holds two utterances at most
and can drop speech under load. Starting commentary after a broadcast starts
does not replace the running encoder's inputs—restart the broadcast for audio.
A generic RTMP destination defaults to video-only, so enable audio when
starting it. Existing no-commentary broadcasts work unchanged.

## Safety, persistence, tests

The policy selects the highest-priority unseen semantic event each poll and
requires valid event IDs, matching attempt and frames. It emits only approved
actions (`comment`, `recap`, `milestone`, `comparison`, or silence), a
short line of text (up to 220 characters), a source event reference, timestamp,
and policy/model identity. Unknown model fields, trailing JSON, untrusted
actions, and provider errors fail to silence. Comparisons are suppressed in
the absence of sourced historical context.

The most recent 128 safe emissions are kept in process memory; the status
route returns the newest 20. If artifact S3 is configured, each emission is
also saved as immutable JSON at
`studio/<sha256(run-id)>/live-commentary/v1/<sha256(event identity)>.json`.
S3 write errors do not block streams or runs. The `emitted_at` time is the
real live presentation time, distinct from the source frame's replay-relative
`timestamp_ms`. No raw model response or hidden reasoning is persisted.

Tests: `go test ./media/commentator ./cmd/pokereplay ./cmd/pokeui`.
