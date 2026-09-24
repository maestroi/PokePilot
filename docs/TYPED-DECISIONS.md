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

Enable the hosted TypeSafe Jev backend with:

```sh
export POKEPILOT_DECISION_BACKEND=jev
export TYPESAFE_API_KEY=...
# Optional overrides:
# export POKEPILOT_DECISION_MODEL=jev-latest
# export POKEPILOT_DECISION_URL=https://api.typesafe.ai/v1
```

The Jev adapter calls the System One `choice` API directly over HTTP; no Jev SDK
is required for ordinary builds or runs. `POKEPILOT_DECISION_TOKEN` overrides
`TYPESAFE_API_KEY` when set. Credentials live only in process environment /
runtime engine state and are never written to benchmark settings, run specs, or
typed-decision telemetry.

### Per-run selection in PokeWall

On the farm the decision engine is chosen per run from the **same model
registry as the strategist**. Register it once in PokeWall's deployments panel
(Protocol: *TypeSafe choice API* for Jev, or any OpenAI-compatible deployment
used through typed choices), then pick it in the launch form's **Fast decision
engine** field, next to a **Mode** (Shadow / Active), battle,
objective-selection and failure-recovery toggles, and a minimum confidence.
The choice travels on the run as `decision_engine`:

```json
{"decision_engine": {"deployment": "typesafe-jev", "mode": "shadow", "battles": true, "objectives": true, "failures": true, "min_confidence": 0.65}}
```

A registry row for Jev looks like:

```json
{"id": "typesafe-jev", "label": "TypeSafe Jev", "protocol": "typesafe-choice",
 "endpoint": "https://api.typesafe.ai/v1", "model_id": "jev-latest", "api_model": "jev-latest",
 "compute": "TypeSafe cloud", "token_env": "TYPESAFE_API_KEY", "enabled": true}
```

At enqueue the wall resolves `deployment`, sets `backend` from the protocol
and copies the deployment's secret-free identity into
`decision_engine.inference` (any identity a client sends is discarded). The
runner builds the engine from that identity, exactly as it does for the
strategist's `inference`, so no runner needs `POKEPILOT_DECISION_URL` or
`POKEPILOT_DECISION_MODEL`. The one thing that stays in runner environment is
the key, read from the variable the row's `token_env` names; this is the same
rule the strategist follows, and it keeps the key out of run specs, the
registry, clones and archives. A choice-only deployment cannot be selected
as the strategist or an experiment arm (400).

A selection without `deployment` (API clients, older runs) still accepts
`"backend": "jev" | "system-one"` and uses the runner's own decision endpoint
settings below.

Modes:

- `shadow` asks the backend at every enabled decision point and records its
  answer, confidence and whether it agreed with what actually executed
  (`shadow`, `executed`, `agreed` on each decision record; run-level
  `decision_mode`, `decision_agreements`, `decision_disagreements`). The
  strategist still picks objectives and deterministic recovery policy still
  handles failures; a shadow `pause`/`impossible` never stops the run.
- `active` lets accepted answers steer objective selection and tighten
  failure recovery, as before. A selection without `mode` is active, so
  older specs are unchanged.
- `off` is the same as backend `off`.
- `battles` is shadow-only (the wall answers 400 for active battles). The
  run carries it to the runner as `DecisionSettings.Battles`; the live battle
  consumer that records per-turn shadow choices is #1456.

Every runner uses the same image. `deploy/farm.yml` gives every runner
`TYPESAFE_API_KEY` from the stack environment; nothing calls Jev unless the
run selected it. A run without `decision_engine` keeps the runner's
`POKEPILOT_DECISION_BACKEND` default (unset = off), so older runs are
unchanged; `"backend": "off"` disables typed decisions even when the runner
default enables them. A Jev run leased by a runner without the key fails at
start with `credentials are not configured` (naming the missing variable)
rather than silently running a different experiment. Clones and endless
successors keep the selection and its identity, and the live view and
archive show it.

Runner defaults, used only by runs without a registered deployment:

- `POKEPILOT_DECISION_FAILURES` defaults to on when a decision backend is
  enabled. It classifies only failures that deterministic runtime policy has
  already declared recoverable.
- `POKEPILOT_DECISION_OBJECTIVES` defaults to off. Set it to `1` to let the
  typed backend choose from the already-valid objective menu before falling
  back to the existing planner.
- `POKEPILOT_DECISION_MODE` is the runner default mode (`active` when unset;
  `shadow` or `off`). `POKEPILOT_DECISION_BATTLES=1` applies only in shadow.
- `POKEPILOT_DECISION_MIN_CONFIDENCE` defaults to `0.65`. A lower-confidence
  answer is recorded and falls back to the existing deterministic/generative
  path.
- `POKEPILOT_DECISION_TIMEOUT` tunes either decision backend.
  `POKEPILOT_DECISION_MAX_TOKENS` applies only to the OpenAI-compatible
  backend.
- `POKEPILOT_DECISION_TOKEN` is the optional bearer token. The
  OpenAI-compatible backend falls back to `llm_token`; Jev falls back to the
  official `TYPESAFE_API_KEY` environment variable.

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

## Battle turns (#1455)

`game.BattleDecisionState` is the portable contract for one battle turn at
the main battle menu: active/opponent species, level, HP, status and types;
the active mon's moves with PP, type, power, accuracy and effectiveness
against the current opponent; legal switches; caller-permitted items; and
whether RUN is legal. It names everything by semantic id and has no Red,
emulator or skill dependency.

The action set is derived, never authored by a backend:

`move:<slot>` / `switch:<party-slot>` / `item:<item>:<party-slot>` / `run`

The Gen I adapter (`skill.BuildBattleDecisionState`) decides legality:
move actions mirror `BattleState.Usable` (PP and Disable), switches exclude
the active and fainted members, items are listed only when permitted by the
caller, held in the bag and would have an effect on the target, and RUN is
legal only in wild battles. Safari Zone and the Old Man demo are reported as
`ErrBattleNotActionable`. `BattleDecisionRequest` declares exactly that set;
`ResolveBattleDecision` re-checks the reply against it before anything could
execute.

This PR only defines the contract. Execution stays with `skill.Battle`,
`SwitchActive` and `UseBattleMedicine`, and ordinary runs are unchanged:
`agent.BattleMoveDecider` adapts a `DecisionEngine` to the existing
`skill.MovePolicy` seam (move-only, falls back to the deterministic policy on
any error or low confidence) but nothing installs it yet. Live consumers
belong to #1456 (shadow mode).

The battle suite is its own evaluation mode, separate from the planner suite
and from live runs:

```sh
go run ./cmd/agent-eval -suite battle -list
go run ./cmd/agent-eval -suite battle -backend decision -url http://localhost:8001/v1 -model qwen3.5-4b -json
TYPESAFE_API_KEY=... go run ./cmd/agent-eval -suite battle -backend jev -model jev-latest -json
```

## Telemetry

Typed decisions are telemetry, not a dataset: a run keeps a fixed-size
summary and streams individual calls without storing them.

- **Per-run summary** (`decision_summary` on the run's stats, saved with the
  run row). Per decision kind: calls, fallbacks, errors, shadow agreements and
  disagreements, a 10-bucket confidence histogram with agreement per bucket
  (does the engine's confidence mean anything?), a latency histogram with
  p50/p95, tokens, and bounded tallies of the engine's and the executed
  choices. It stays a few KB whether the run makes a hundred decisions or a
  hundred thousand.
- **Live feed** (`decision_records`). The last 32 calls ride each
  heartbeat: engine choice (by label), confidence, what actually ran,
  agreement and latency. Older calls scroll out
  (`decision_records_dropped`) and the feed is stripped from the stored run
  row, so heartbeats stay a constant size and nothing per-call is persisted.
  `decision_exchanges` is no longer written.
- The operator live view shows both (Fast decisions panel); the archive
  shows the stored summary.

Heartbeat-level counters (backend/model identity, cumulative latency and
tokens, the latest choice and probabilities) remain for dashboards. A
sampled training log with full inputs and outcomes is tracked separately in
#1826.

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

Hosted Jev path against the same ROM-free fixtures:

```sh
TYPESAFE_API_KEY=... go run ./cmd/agent-eval -backend jev -model jev-latest -json
```

Each report includes pass score, wall-clock duration, prompt/completion token
usage when available, and per-fixture choices. That is the first small
behavior/runtime benchmark requested by #996; longer farm experiments can use
the same telemetry once `POKEPILOT_DECISION_OBJECTIVES=1` is enabled.
