# PokePilot instructions for Claude

Read `AGENTS.md` first. Before changing gameplay/runtime architecture, also read
`docs/ARCHITECTURE.md` in full.

`docs/ARCHITECTURE.md` is a binding design contract. PokePilot is a multi-game
runtime whose first adapter is Pokémon Red; it is not acceptable to make one
Red run pass by leaking Red-specific maps, dialogue, story gates, or RAM facts
into generic runtime policy.

If a requested change does not fit the architecture, do not force the patch.
Explain the mismatch and reshape/refactor the boundary first. Prefer shared
semantic invariants, typed/structured outcomes, positive postconditions, and
small deterministic regressions over path-specific fixes or full-run-only
verification.

Do not duplicate the architecture rules here. `docs/ARCHITECTURE.md` is the
single source of truth.
