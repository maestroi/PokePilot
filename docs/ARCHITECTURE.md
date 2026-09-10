# PokePilot architecture principles

PokePilot is not a Pokémon Red bot with an LLM bolted on top. The project is a
long-running game-playing runtime whose first supported game happens to be
Pokémon Red.

The architectural target is that adding another Pokémon game — Crystal,
Emerald, FireRed, or later titles — should primarily mean adding a game adapter
and game-specific facts, not forking the planner/runtime and copying years of
Red-specific recovery logic.

These principles are **binding design constraints** for changes to the agent,
skills, world model, recovery, farm, and game-state layers. They are not
aspirational documentation.

If a requested fix does not fit these principles, do not force the fix into the
current design. Stop, describe the mismatch, and change or extend the
architecture first. A local patch that makes one run pass while making the next
game harder to support is not a successful fix.

## The core rule

> Generic code owns semantics and lifecycle. A game adapter owns game facts and
> game-specific mechanics.

The core may understand concepts such as:

- travel to a destination;
- battle an opponent;
- heal a party;
- acquire or use an item;
- satisfy a progression requirement;
- complete, block, require a choice, or fail an invariant;
- stabilize an objective boundary;
- verify a semantic postcondition.

The generic layers should not need to know that map `0x34` is a museum, that a
specific dialogue is a Pewter ticket gate, that a badge is stored at one Red RAM
address, or that an Emerald menu has a particular cursor layout. Those are game
facts and belong behind the game-specific boundary.

The current repository predates a clean physical adapter split, so not every
package is there yet. Treat this document as the direction of travel: new code
must improve the boundary or at minimum avoid making it worse.

## 1. Objectives are transactions

Every planner-selected objective follows this lifecycle:

```text
observe
  -> validate prerequisites
  -> normalize the start boundary
  -> execute the owned action
  -> normalize the finish boundary
  -> verify the semantic postcondition
  -> return a structured ObjectiveResult
```

A function returning `nil` is never enough evidence that the objective
completed. Success means the world now satisfies the objective's positive
postcondition.

An objective owns the state it leaves behind. It must not leak a menu, dialogue,
battle, partial transition, or unresolved choice and make the next objective
responsible for cleaning it up.

Generic boundary cleanup may perform only semantically reversible operations,
such as backing out of a known dismissable menu. It must never answer a gameplay
choice, make a story decision, spend a resource, or silently advance progression.
Those belong to the skill or game adapter that encountered them.

## 2. Small gameplay failures are normal; invariant failures are not

A full campaign is long enough that ordinary failures are guaranteed. Losing a
battle, failing to catch a Pokémon, lacking money, reaching a gated path, or
needing healing must not automatically kill the entire run.

Use structured outcomes and typed errors. Examples include:

- `completed` — the semantic postcondition is true;
- `blocked` — the world is stable but the requested action cannot currently
  complete;
- `choice_required` — a gameplay choice is open and needs an owner;
- `postcondition_failed` — execution claimed success but the world contradicts
  it;
- `postcondition_unavailable` — the world never became safely inspectable within
  a bounded settle;
- `stabilization_failed` — the objective did not return the game to a safe
  boundary;
- `ownership_failure` — work escaped the layer that was supposed to own it;
- `controller_uncertain` — a bounded controller can no longer drive safely;
- `unknown_failure` — unclassified failures default to terminal, never silently
  recoverable.

Do not classify behavior by parsing error prose. If policy needs to understand a
failure, introduce a typed sentinel/result or a structured field.

## 3. The planner reasons about state changes, not retries

A retry is justified by a relevant world change, not because an objective failed
once.

Examples of relevant change:

- the lead gained levels;
- inventory changed;
- money changed;
- a badge/event flag changed;
- a capability became available;
- the player reached a new stable region;
- a previously missing prerequisite was satisfied.

If the same objective, same relevant state, same build, and same failure repeat,
that is evidence of a defect or a missing planning concept. Do not burn the run
budget repeating it.

`ObjectiveResult` is the durable boundary for this information. Prefer adding
structured facts there (or in owned skill results) over adding log text.

## 4. World transitions are semantic, not only geometric

A map edge is not merely `A -> B`. A transition may:

- require a badge, item, field move, vehicle, money, or story flag;
- trigger dialogue or a battle;
- consume a resource;
- permanently change after use;
- be one-way;
- require an explicit choice;
- be unavailable until another objective changes the world.

Navigation answers:

> Given the capabilities and world state that are usable now, how can I reach
> this destination?

Progression answers:

> What must become true before this destination or transition is usable?

Do not teach pathfinding to solve progression by taking bizarre detours, entering
unrelated buildings, or special-casing named maps. Improve the transition model.

## 5. Capabilities are the portable planning vocabulary

The planner should reason from capabilities rather than game/version-specific
conditionals.

Examples:

```text
can_cut
can_surf
can_fly
can_heal_here
can_buy(item)
can_access(region)
can_move_boulders
can_use_bike
has_required_key_item
```

The exact source of a capability is adapter-owned. Red may derive one from a
badge plus an HM and learned move; another game may have a different rule. The
planner consumes the semantic capability, not the RAM implementation.

When a new game requires a concept that genuinely is not represented by the
current capability model, extend the semantic model deliberately. Do not hide
that concept in a one-game planner conditional.

## 6. Game-specific knowledge stays game-specific

The following are game-adapter concerns:

- RAM/HRAM addresses and save layout;
- ROM table formats;
- map ids, warp ids, coordinates, object formats;
- event/badge/story flag encodings;
- menu layouts and dialogue encodings;
- game-specific item/species/move ids;
- named story gates and scripted encounters;
- generation-specific battle and field-move rules;
- evidence required to prove a game-specific postcondition.

The following are core/runtime concerns:

- objective lifecycle;
- normalized outcomes;
- planner policy over structured results;
- generic transition/capability concepts;
- recovery ownership rules;
- failure fingerprinting and quarantine policy;
- deterministic replay contracts;
- campaign/milestone qualification orchestration.

A Red-specific fact in generic runtime code is a design smell. Sometimes a
migration requires temporary coupling; if so, mark it explicitly and create a
clear removal path rather than normalizing it as architecture.

## 7. Prefer shared-point fixes over path fixes

When a farm run says:

```text
go to Mt Moon -> failed at Museum prompt
```

the objective name is evidence, not necessarily the layer to patch.

Ask which invariant failed:

- Did transition state identity allow a fake cycle?
- Did a route edge omit a prerequisite?
- Did the owning skill leak a choice?
- Did generic cleanup make a gameplay decision?
- Did the planner retry without a state change?

Fix the shared abstraction that permitted the invalid state. Do not add
`if museum`, `if Brock`, `if map == 0x34`, or a dialogue substring to a generic
layer unless that value is part of an explicitly game-specific adapter.

## 8. Long-run resilience is a first-class feature

The target is completion of a whole campaign, not success of isolated verbs.
That means the runtime must assume that over many hours it will encounter:

- stochastic losses;
- temporary resource shortages;
- optional dialogue;
- dynamic NPC blockers;
- route gates;
- state transitions that were not previously exercised;
- controller uncertainty and genuine defects.

Ordinary game outcomes should re-plan from stable state. Invariant/controller
failures should stop or quarantine before the planner makes the state worse.
Unknown failures default to stopping, because silently treating a new defect as
recoverable creates endless loops.

Do not use save-state rollback as ordinary runtime recovery. It can erase real
progress and turns the project into save-scumming rather than a controller that
can finish the game it actually played. Save states are for reproducibility,
fixtures, and diagnostics.

## 9. Endless runs discover; deterministic replays prevent regressions

Endless farm runs are the exploration layer. They are not the regression test
suite.

Every actionable production failure should move through this funnel:

```text
endless run discovers failure
  -> capture build + ObjectiveResult + typed failure + checkpoint/state
  -> reproduce the smallest deterministic failing scenario
  -> fix the owning abstraction
  -> add the replay/scenario to the qualification corpus
  -> resume endless exploration for new failures
```

A bug that can be reproduced from a checkpoint in seconds must not require a
multi-hour fresh campaign to verify forever. Failed endless successors therefore
resume from the parent's latest major (post-badge) checkpoint so the live farm
can rediscover the same defect; a successful `done` still starts a new campaign.

Qualification should be layered:

1. pure ROM/state/unit tests;
2. short deterministic skill/objective replays;
3. milestone journeys from trusted checkpoints;
4. full fresh-save campaign qualification;
5. endless/fuzz-like farm exploration.

The higher layers find integration problems. The lower layers keep already-found
problems fixed cheaply.

## 10. Repeated failures are quarantined, not rediscovered forever

A failure should have a stable fingerprint built from structured information,
such as:

- game/version;
- build;
- checkpoint or relevant state identity;
- objective kind and arguments;
- normalized outcome;
- typed root cause;
- relevant transition/controller context.

The farm may retain every historical occurrence for evidence, but it should not
keep launching equivalent verification work for the same unresolved fingerprint.
Resolved/fixed fingerprints remain history and should not re-enter the actionable
queue unless they reproduce on a newer build or materially different state.

## 11. Deterministic evidence beats LLM inference

LLMs choose goals, interpret structured evidence, and help diagnose missing
abstractions. They should not reconstruct collision grids, guess RAM state,
memorize map geometry, or infer facts deterministic code can measure.

If code can answer a question exactly, give the model the answer. Existing
examples are the map probe, ROM parsers, state decoders, structured observations,
and `ObjectiveResult`.

When an LLM spends substantial reasoning on a game-state fact that can be
measured, that is a tooling gap to fix rather than a prompt-engineering victory.

## Target layering

The conceptual architecture is:

```text
Campaign planner
  "what should become true next?"
        |
        v
Objective runtime
  transactions, outcomes, recovery policy, postconditions
        |
        v
Semantic skills + world model
  travel, battle, acquire, heal, capabilities, transitions
        |
        v
Game adapter
  maps, events, RAM, ROM, menus, story rules, game-specific evidence
        |
        v
Emulator / game runtime
```

The physical Go packages do not need to be reorganized in one giant rewrite.
Migrate boundaries incrementally while keeping behavior working. A good change
makes this dependency direction clearer; a bad change makes generic code depend
on more one-game facts.

## Design gate for every change

Before implementing a gameplay/runtime fix, answer these questions:

1. **Who owns this behavior?** Core runtime, semantic skill/world model, or game
   adapter?
2. **Is this a semantic concept or a Red fact?** If another Pokémon game could
   implement it differently, keep the fact behind the adapter boundary.
3. **What positive postcondition proves success?** Never use absence of an error
   as the proof.
4. **What structured outcome should ordinary failure produce?** Do not encode
   policy in log prose.
5. **Can this failure leave the emulator in a stable state for replanning?** If
   not, the owning layer must stabilize it or report a terminal invariant
   failure.
6. **Does the proposed fix improve the shared abstraction, or only the exact path
   that exposed it?** Prefer the shared abstraction.
7. **How will this be replayed cheaply after it is found once?** Add the smallest
   deterministic regression scenario possible.
8. **Would this design still make sense if the next game were Crystal or
   Emerald?** The implementation can be game-specific; the core concept should
   remain coherent.

If the answer to these questions shows that the requested patch does not fit the
architecture, **do not implement the patch as requested**. Rethink the boundary,
introduce the missing semantic concept, or propose the prerequisite refactor.
That is expected engineering work, not a failure to follow the task.

## Practical rule for AI coding agents

When working on PokePilot:

- read `AGENTS.md` and this file before changing game/runtime architecture;
- treat these principles as repository requirements;
- do not bypass them to make one test or farm run green;
- do not add generic special cases for named Red maps, NPCs, dialogue, badges,
  or story events;
- prefer typed/structured results over strings;
- prefer a reusable invariant plus a deterministic regression over a local retry;
- when a task conflicts with these principles, say so and reshape the design
  before coding.

The project optimizes for a runtime that can finish many Pokémon games over many
hours. A one-off fix that narrows that future is technical debt, even when it
makes today's run advance one screen farther.
