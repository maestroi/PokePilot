# The agent loop

`pokepilot -planner` chooses how objectives are picked. Two modes:

- `scripted` (default) — the deterministic flow: boot, take the `-starter`
  Pokemon, walk to `-goto`, keep serving. No model is ever called; a plain
  `make run` works with the inference server switched off.
- `llm` — the objective loop (`agent.Run`). Each round the model picks one
  of the offered objectives: `take a starter` plus one `go to <place>` per
  name `skill.Place` accepts. Every round is printed to stdout, so an
  unattended run leaves a log a human can read in the morning.

Every run is bit-identical unless you ask for otherwise. Gen 1 has no seed:
its RNG is `hRandomAdd`/`hRandomSub` ($FFD3, $FFD4) reseeded from DIV, which
counts CPU cycles, and nothing in the loop reads a wall clock — so the same
ROM and the same inputs meet the same Pidgey on the same tile. `-seed N`
burns N-derived idle frames after boot, which shifts the cycle count and
reroutes every encounter that follows. Measured, walking to Viridian:

    -seed 0   first wild battle at (14,7)    frame 18956
    -seed 1   first wild battle at (12,22)   frame 17721
    -seed 2   first wild battle at (10,32)   frame 16020
    -seed 1   first wild battle at (12,22)   frame 17721   (identical, as intended)

A seed is for run diversity, not for a fair comparison: two policies under
different seeds have different luck, so a difference in outcome is not
evidence. Branch both arms from one save state for that.

Environment (llm mode only):

- `POKEPILOT_LLM_URL` — OpenAI-compatible base URL, default
  `http://192.168.50.81:8002/v1`
- `POKEPILOT_LLM_MODEL` — model name, default `qwen3.8-27b`

The run stops on budget exhaustion (rounds or frames), on a planner reply
that names no offered objective, or on a failed objective. The final line
says which; `error` and `stuck` stops exit non-zero.

Note: in llm mode the starter objective is always Squirtle
(`agent.Execute`); `-starter` only applies to scripted mode.

Worked example:

    POKEPILOT_LLM_URL=http://localhost:8002/v1 make run-llm
