# Studio declarative episode editor (#799)

A producer assessment is **evidence**, not an FFmpeg program. The episode editor
turns accepted story beats into a versioned, immutable edit plan, validates
every time range against the recorded semantic timeline, then uses the existing
composited replay video as input to a deterministic FFmpeg encoder/concatenator.

## Workflow (internal replay service)

1. Obtain a completed run's producer assessment (#797):
   `POST /v1/runs/{run-id}/producer/assess` — save its `assessment.hash`.
2. Render the run's **full broadcast replay** once:
   `POST /v1/runs/{run-id}/replay/render?mode=broadcast`. Wait for
   `GET /v1/runs/{run-id}/replay/status?mode=broadcast` to report `ready`.
3. Create an editable draft from the assessment:
   ```http
   POST /v1/runs/{run-id}/studio/edits
   Content-Type: application/json

   {"assessment_hash":"<64-hex-hash>","profile":"episode"}
   ```
   Or choose `"profile":"short"` where that assessment supports it.
   The response contains the full `plan` and its immutable `hash`.
4. Optionally submit a revised **typed** list of `segments` using the same
   endpoint with `assessment_hash`, `profile`, and `segments` supplied.
   Source identity/timings are always populated by the server.
5. Review and render:
   `GET /v1/runs/{run-id}/studio/edits/{hash}`
   `POST /v1/runs/{run-id}/studio/edits/{hash}/render`
   `GET /v1/runs/{run-id}/studio/edits/{hash}/status`
   `GET /v1/runs/{run-id}/studio/edits/{hash}/video`

Render is asynchronous, uses the replay worker's resource admission/lease
machinery, and stores the derived MP4 + manifest in S3 under a key including
the edit-plan hash, source hashes, timeline hashes, and render/encoder versions.
The source `.gbrun`, source MP4, full replay cache, and run status are intact.

If a full composited MP4 is not ready, render returns `409 source_missing`;
the editor **does not replay live gameplay**. Plan creation itself does not
require a rendered MP4.

## Typed plan

`Plan.Version`, `PolicyVersion`, `RenderVersion`, `AssessmentHash`,
`Sources`, `SourceHash`, `DurationMS`, and `Hash` make changes auditable
and reproducible. Segments are ordered, and all source timestamps are
**attempt-local milliseconds**.

| Action | Fields | Result |
| --- | --- | --- |
| `keep` | `attempt`, `start_ms`, `end_ms`, `speed` | Preserve selected source footage, at 1×/2×/4×/8×/16× |
| `cut` | `attempt`, `start_ms`, `end_ms` | Deliberately skip redundant footage |
| `card` | `duration_ms`, optional `overlay` and `caption` | Full-frame branded title card |
| `pad` | `duration_ms` | Neutral black padding |

A kept segment can also contain validated `event_ids`, `overlay`,
`caption`, `narration_cue`, `chapter`, and `transition` (`cut` or
`fade`). Selected producer beats must each appear at least once in
real-time kept source footage. Source ordering and non-overlap are enforced
across attempts. Invalid durations, event IDs, speed values, source
fingerprints, and hash mismatches reject the plan **before encoding**.

`narration_cue` is persisted editorial timing/wording metadata for #800; it
does **not** currently synthesize speech or add an audio track. Overlays and
captions become textfile-backed FFmpeg `drawtext` layers, never unchecked
filter expressions from the model.

The default policy keeps source windows around events including whole
spanning battles at 1× speed. The remainder is accelerated to 8× when short
enough, otherwise cut. A vertical short uses a 1080×1920 frame with the
existing landscape replay letterboxed, rather than inventing game-native
portrait layouts. More advanced layouts and TTS can replace these versioned
components later.

## Trust boundary and reproducibility

- Only typed actions are accepted from editor/producer clients; no arbitrary
  subprocess command, media URL, or filtergraph fields are accepted.
- Operator requests are size-bounded and unknown JSON fields are rejected.
- Assessment references are verified against an immutable producer artifact,
  and timeline/event IDs against the authoritative durable media timeline.
- Captions are literal text files in the worker's scratch directory, not
  interpolated FFmpeg expressions.
- The compiled encoder uses a fixed geometry, rate, codec and deterministic
  ordered concatenation. Renderer and encoder versions affect cache identity.
- Rendering can be retried/reclaimed by another replay worker after a lease
  expires; assessment or plan changes produce new immutable artifacts.
- A `do_not_publish` or `archive_only` producer decision cannot generate
  a public-format plan. `short_only` cannot be promoted to a full episode.

Run `go test ./media/editplan ./media/segment ./cmd/pokereplay`
plus the repository's ROM-free CI suite before merge.
