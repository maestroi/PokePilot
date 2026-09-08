# PokePilot instructions for Gemini

Read `AGENTS.md` first. Before changing gameplay/runtime architecture, read
`docs/ARCHITECTURE.md` in full.

That architecture document is binding. PokePilot is a multi-game runtime whose
first adapter is Pokémon Red. Do not leak Red-specific maps, dialogue, story
gates, RAM addresses, or one-off mechanics into generic runtime policy simply
to make one run pass.

If the requested change does not fit the architecture, explain the mismatch and
reshape/refactor the boundary before implementing it. Prefer portable semantic
concepts, typed/structured outcomes, positive postconditions, and small
deterministic regressions.

`docs/ARCHITECTURE.md` is the single source of truth.
