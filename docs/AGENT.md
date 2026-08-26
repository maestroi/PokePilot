# The agent loop

`pokepilot -planner` chooses how objectives are picked. Two modes:

- `scripted` (default) — the deterministic flow: boot, take the `-starter`
  Pokemon, walk to `-goto`, keep serving. No model is ever called; a plain
  `make run` works with the inference server switched off.
- `llm` — the objective loop (`agent.Run`). Each round the model picks one
  of the offered objectives: `take a starter` plus one `go to <place>` per
  name `skill.Place` accepts. Every round is printed to stdout, so an
  unattended run leaves a log a human can read in the morning.

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
