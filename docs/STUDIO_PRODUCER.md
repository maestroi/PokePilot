# Evidence-backed studio producer (#797)

The producer is an **explicit post-run analysis stage**, not a gameplay
controller, video renderer, or publisher. It consumes the durable
`farm.MediaTimeline` produced by #173, including per-attempt event IDs,
frames and timestamps. It does **not** read private gameplay prompts, hidden
model reasoning, ROM/RAM dumps, screenshots, or raw model transcripts.

## API

The internal replay service exposes:

- `POST /v1/runs/{id}/producer/assess` — collect available per-attempt
  `media-timeline.json` artifacts, score the run, validate selected beats,
  and persist a content-addressed JSON assessment in S3. The result includes
  `assessment` and `object_key`. This does not render or publish anything.
- `GET /v1/runs/{id}/producer/assessments/{hash}` — retrieve a stored
  version by its assessment SHA-256 identity. Older policy/model assessments
  remain accessible instead of being replaced.

The optional POST JSON body is:

```json
{
  "history": [
    {
      "run_id": "older-run-id",
      "source": "durable-farm-statistics/older-run-id/v1",
      "goal": "Beat the second gym",
      "duration_ms": 200000,
      "event_types": ["gym_battle", "badge_acquired", "run_finished"],
      "published": true
    }
  ]
}
```

Historical records should be assembled by a **trusted operator** from
durable run statistics. There is no automatic historical catalog lookup in
this first slice. Rows require a unique run ID and provenance source;
unsourced or conflicting rows fail validation. The producer compares durations
only for the same exact goal among successful completed runs and compares
event-type similarity only against explicitly marked published runs. Those
comparisons are descriptive, not world-record claims.

A run without recorded timeline events yields `do_not_publish` and no
invented narrative. Runs without a verified terminal status cannot be
promoted beyond `archive_only`. For full-episode eligibility, the
deterministic gate requires enough meaningful events and at least two
distinct event types. Strong similarity to a sourced published run downgrades
the maximum to `archive_only`. The creative producer may always choose a
**less publishable** outcome, including `do_not_publish`.

## Model configuration

By default the replay service runs a deterministic `rules-v1` offline
assessment (with exact event-backed beats). To opt into a separate editorial
LLM, configure:

| Variable | Meaning |
| --- | --- |
| `POKEPILOT_PRODUCER_CHAT_URL` | Full URL of an OpenAI-compatible chat-completions endpoint |
| `POKEPILOT_PRODUCER_MODEL` | Exact model identifier |
| `POKEPILOT_PRODUCER_DEPLOYMENT` | Optional stable deployment/revision ID, used in output identity |
| `POKEPILOT_PRODUCER_API_KEY` | Optional bearer token; never included in prompts or results |

Model input is bounded to safe run metadata and sampled semantic event
facts, plus sourced historical comparisons. Model output is required to be a
strict JSON object containing a primary story, secondary stories, beats with
exact references, and a proposed publication class. Unknown fields,
incorrect event IDs/attempts/frame/timestamp pairs, invalid classes,
oversized text, and malformed/trailing JSON all fail closed. No assessment
is persisted when validation fails.

Scores are deterministic observed-event heuristics (0–100), not verified
game difficulty ratings or a claim about audience response. Historical
importance and published-run similarity are omitted without sourced
comparisons. The producer persists policy/prompt/model identity, source
fingerprint, event references, scores, comparison provenance, publication
decision/reason, and assessment content hash.

## Boundaries and future work

The S3 object is an input for the planned declarative editor (#799) and
quality-gated publication queue (#803); nothing here queues an upload.
Assessor policy `v1` is intentionally conservative. The API is internal
to the replay service and can be proxied into Media Studio when an operator
UI for assessment review is added. A future historical-statistics adapter
should populate provenance server-side rather than accept operator-supplied
rows.

Use `go test ./media/producer ./cmd/pokereplay` to exercise the
contract and the replay-service integration.
