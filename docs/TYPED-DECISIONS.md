# Typed decision backend experiment

Issue #996 adds a backend-neutral typed decision seam alongside the existing
generative planner. The default remains unchanged: with no decision backend
configured, PokePilot uses the existing LLM planner and deterministic recovery
policy exactly as before.

## Runtime configuration

Enable the local OpenAI-compatible backend with:

```sh
export POKEPILOT_DECISION_BACKEND=system-one
export POKEPILOT_DECISION_URL=http://localhost:8000/v1
export POKEPILOT_DECISION_MODEL=qwen3.5-4b
```

The decision endpoint is independent of `POKEPILOT_LLM_URL` and
`POKEPILOT_LLM_MODEL`. If the decision-specific URL/model are omitted they fall
back to those LLM variables only as a convenience, so one process can point the
strategist and the typed decision engine at different deployments.

Feature switches:

- `POKEPILOT_DECISION_FAILURES` defaults to on when a decision backend is
  enabled. It classifies only failures that deterministic runtime policy has
  already declared recoverable.
- `POKEPILOT_DECISION_OBJECTIVES` defaults to off. Set it to `1` to let the
  typed backend choose from the already-valid objective menu before falling
  back to the existing planner.
- `POKEPILOT_DECISION_MIN_CONFIDENCE` defaults to `0.65`. A lower-confidence
  answer is recorded and falls back to the existing deterministic/generative
  path.
- `POKEPILOT_DECISION_TIMEOUT` and `POKEPILOT_DECISION_MAX_TOKENS` tune the
  local request independently.
- `POKEPILOT_DECISION_TOKEN` is the optional bearer token. It falls back to
  `llm_token` when omitted.

The OpenAI-compatible implementation requests strict JSON containing one
declared choice plus a complete probability distribution. PokePilot validates
the choice set and distribution again client-side, so a server that ignores the
schema cannot escape the declared choices. The probabilities are explicit
model-reported decision confidence, not token-logprob measurements.

## Safety boundary

Typed decisions do not replace deterministic mechanics. Navigation, controller
safety, objective legality, postconditions, quarantine, and bounded recovery
remain deterministic.

For failure recovery the engine chooses from:

`retry / recover / replan / pause / impossible / unknown`

The runtime invokes this only after `actionFor` has classified the failure as
recoverable. Terminal ownership, controller, stabilization, and unknown
failures therefore cannot be made retryable by the model. `pause` and
`impossible` may conservatively stop a run; the other choices continue through
the existing recovery policy rather than executing model-authored recovery.

For high-level objective selection the engine receives only the objectives that
`Offer` and run policy already made valid. Invalid or low-confidence output
falls back to the existing planner.

## Telemetry

Heartbeats persist typed-decision telemetry separately from the existing LLM
counters: backend/model identity, latency, prompt/completion tokens when
reported, request/response byte counts, selected choice, normalized
probabilities/confidence, rejection/fallback counts, and a bounded per-call
record list.

## ROM-free comparison

`cmd/agent-eval` now runs either backend over the same checked-in decision
fixtures, making model comparisons independent of emulator RNG.

Existing generative 4B/9B paths:

```sh
go run ./cmd/agent-eval -backend llm -url http://localhost:8001/v1 -model qwen3.5-4b -json
go run ./cmd/agent-eval -backend llm -url http://localhost:8002/v1 -model qwen3.5-9b -json
```

Typed decision path against the same endpoint/model:

```sh
go run ./cmd/agent-eval -backend decision -url http://localhost:8001/v1 -model qwen3.5-4b -json
```

Each report includes pass score, wall-clock duration, prompt/completion token
usage when available, and per-fixture choices. That is the first small
behavior/runtime benchmark requested by #996; longer farm experiments can use
the same telemetry once `POKEPILOT_DECISION_OBJECTIVES=1` is enabled.
