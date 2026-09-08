# PokePilot Copilot instructions

Read `AGENTS.md` and, before any gameplay/runtime change,
`docs/ARCHITECTURE.md`.

`docs/ARCHITECTURE.md` is binding. PokePilot is intended to support multiple
Pokémon games; Pokémon Red is the first game adapter, not the permanent shape
of generic runtime code.

Do not solve one failing run by adding Red-specific map IDs, NPCs, dialogue,
story gates, or RAM details to generic planning/recovery/routing layers. Use
semantic capabilities/transitions, typed structured outcomes, positive
postconditions, and the smallest deterministic regression that reproduces the
failure.

If a requested patch conflicts with the architecture, do not force it. State
the mismatch and refactor or extend the architecture first. Treat
`docs/ARCHITECTURE.md` as the single source of truth.
