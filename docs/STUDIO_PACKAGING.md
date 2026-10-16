# Studio packaging: titles, descriptions, chapters and thumbnail concepts (#801)

Packaging is an optional, **review-only** step after the producer assessment (#797) and, optionally, the declarative episode editor (#799). It does not render thumbnails, publish media, rerun gameplay, or mutate farm/run state. The next stages are #802 (thumbnail renderer) and #803 (publication queue).

## API (replay service)

Create a versioned package:

```http
POST /v1/runs/{run-id}/studio/packages
Content-Type: application/json

{"assessment_hash":"<64-hex-sha>","edit_plan_hash":"<64-hex-sha>","style":"documentary"}
```

`edit_plan_hash` is optional. Omitting it generates verified titles/descriptions/Shorts ideas and thumbnail concepts, but **no output-relative chapters**, because those require a validated edit plan with known cuts and speeds. `style` is optional: `neutral`, `documentary` (default), or `energetic`. The run ID comes from the route; the historical producer assessment and all per-attempt timelines come from durable S3 artifacts, not from request-supplied facts.

Response: `{"package": { ... }, "object_key": "studio/<run-hash>/packaging/v1/<package-hash>.json"}`. The package has an immutable content checksum (`hash`), producer `assessment_hash`, optional `plan_hash`, `timeline_sha256`, model identity, prompt/template/schema versions, style, selected event+angle pairs, 3 distinct title suggestions, description, evidence-anchored chapters, 3 Shorts title/hook/caption proposals, and 3 structured thumbnail concepts. Each title/Short/thumbnail references a real recorded event (attempt, event_id, frame, timestamp). These are *candidates*, not automatically approved public claims.

Retrieve:

```http
GET /v1/runs/{run-id}/studio/packages/{package-hash}
```

The server verifies the package hash, source assessment, current authoritative run events, and optional edit plan before returning it. This allows historical re-packaging with new versions without altering the recording.

## Model configuration (optional)

By default `rules-v1` selects three different story angles from verified high-signal events. Optionally configure:

- `POKEPILOT_PACKAGING_CHAT_URL`: trusted HTTP(S) OpenAI-compatible chat-completions endpoint
- `POKEPILOT_PACKAGING_MODEL`: concrete model ID; required with chat URL
- `POKEPILOT_PACKAGING_DEPLOYMENT`: optional deployment label for artifact provenance
- `POKEPILOT_PACKAGING_API_KEY`: secret server-only provider token
- `POKEPILOT_PACKAGING_PUBLIC_BASE_URL`: optional HTTPS site root (e.g. `https://rompilot.app`) for a backlink to `/runs/{id}`

The model receives only bounded public facts: challenge goal (if safe), normalized outcome, style, semantic event IDs, labels, classes and timestamps. It may output **only two or three** allowed `moment`, `challenge`, and `recap` selections that cite exact available `attempt/event_id` pairs. Unknown fields, forged event IDs, and duplicate selections are rejected. The model cannot supply titles, fabricated records, free-form descriptions, external URLs, image prompts, media commands, or private traces. All public copy is assembled through deterministic allowlisted templates. This deliberately favors factual safety over unconstrained LLM clickbait.

## Editorial guarantees and limitations

- The producer's `do_not_publish` and `archive_only` classifications cannot yield a publishable package; no benchmark/record claims are generated.
- Descriptions include only trusted, sanitized goal, run outcome, planner field and observed event kinds. This release does **not** guess a game title/model/mode from unrelated fields; these can be added after durable structured metadata is available.
- If the public run URL is not configured, the backlink is omitted.
- Chapters are derived from event IDs in 1× retained edit footage, correctly offset through preceding cards, cuts and speedups. They cannot describe a discarded event.
- Shorts proposals cite actual selected producer events, but creating metadata does not imply a short video has already been rendered.
- Thumbnail concepts specify subject, composition, emphasis, and short factual text. Artwork generation, image storage and typography belong to #802.
- Template changes, prompt/model changes, style changes, edit changes or semantic timeline changes produce distinct content-addressed packages.
- Nothing is automatically sent to YouTube. Human review/approval and publication scheduling belong to #803.

## Verification

```bash
go test ./media/packaging ./cmd/pokereplay
```

Tests cover deterministic regeneration, content hashes, source-drift rejection, timed chapters, distinct candidates, malformed/forged model instructions, secrets in optional metadata, styles, URLs, and non-publishable run decisions.
