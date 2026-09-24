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

On the farm the backend is chosen per run, the same way the strategist
deployment is: the PokeWall launch form has a **Fast decision engine** field
(Off / TypeSafe Jev / Local System-1) with objective-selection and
failure-recovery toggles and a minimum confidence. The choice travels on the
run as `decision_engine`:

```json
{"decision_engine": {"backend": "jev", "objectives": true, "failures": true, "min_confidence": 0.65}}
```

Every runner uses the same image. `deploy/farm.yml` gives every runner
`TYPESAFE_API_KEY` from the stack environment; nothing calls Jev unless the
run selected it. The key never enters a run spec, catalog row, clone or
archive. A run without `decision_engine` keeps the runner's
`POKEPILOT_DECISION_BACKEND` default (unset = off), so older runs are
unchanged; `"backend": "off"` disables typed decisions even when the runner
default enables them. A Jev run leased by a runner without the key fails at
start with `credentials are not configured` rather than silently running a
different experiment. Clones and endless successors keep the selection, and
the live view and archive show it.

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

Hosted Jev path against the same ROM-free fixtures:

```sh
TYPESAFE_API_KEY=... go run ./cmd/agent-eval -backend jev -model jev-latest -json
```

Each report includes pass score, wall-clock duration, prompt/completion token
usage when available, and per-fixture choices. That is the first small
behavior/runtime benchmark requested by #996; longer farm experiments can use
the same telemetry once `POKEPILOT_DECISION_OBJECTIVES=1` is enabled.
