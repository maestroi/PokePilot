# PokePilot Operator Console Visual System

## Direction

The private operator console is a game-native workstation: one selected run owns
the working surface, related views stay synchronized, and other runs remain
available as compact sources. Its identity comes from real game frames, semantic
maps, party state, badges, encounters, and persisted event symbols—not retro
chrome, franchise colors, or decorative nostalgia.

## Physical Scene

An operator uses this console for long desktop sessions in a dim workstation or
homelab environment. The Game Boy image and semantic map need visual priority
without bright chrome competing with them.

## Color

Use a restrained dark strategy. Near-black and blue-black fields define the
shell and fitted working regions. Fine cool-gray rules establish hierarchy. The
game frame and semantic map keep their original color and provide the strongest
color in the interface. Cyan marks selection, live position, and primary
operator actions; green marks confirmed progress and healthy state; amber marks
checkpoints, blocked states, and recoverable attention; red is reserved for
failures or destructive actions.

Status must always include text, shape, or iconography in addition to color.
Avoid glow, gradients used as decoration, and large saturated background areas.

## Typography

Use a compact workhorse sans for interface copy and a tabular monospaced face
for run IDs, frames, timestamps, coordinates, hashes, and counters. Sentence
case is the default. Uppercase labels are reserved for short equipment-like
legends and must remain readable at normal size.

## Composition

- The selected run occupies most of the viewport.
- Game and Semantic Map dominate one paired visual unit; Game State is a compact
  third bay for the six party slots, badges, and money.
- The active objective and latest planner decision stay visible near the paired
  views.
- The transport timeline spans the working surface and owns chronological
  navigation.
- Active and recent runs appear in a compact source rail.
- Run Story is a continuous chronological reading surface below the transport.
- Finished replay replaces live media inside Game; it never creates a detached
  second media surface.
- Operations owns fleet health, workers, queue/leases, and recent terminal
  outcomes. Analytics alone owns campaign aggregates.

## Components

Panels should read as fitted equipment bays and aligned tables: square or
lightly rounded corners, precise one-pixel boundaries, compact headers, and
clear internal alignment. Avoid a page made from interchangeable floating
cards.

Controls use familiar browser affordances. Primary actions are filled and rare;
secondary actions are outlined or quiet text buttons. Copy controls appear only
where an exact machine value is useful. Ordinary copy remains selectable text.

Timeline markers use stable shapes for objective, decision, battle, checkpoint,
progress, recovery, and failure events. A selected marker connects visibly to
its Run Story entry and evidence.

## Motion

Motion communicates live state and continuity: a restrained live pulse, a
moving timeline playhead, and direct transitions when selecting runs or events.
No ambient animation. Honor reduced motion by replacing movement with immediate
state changes.

## Responsive Behavior

On narrow screens, the run rail becomes a drawer; Game, Semantic Map, then Game
State stack in that order; and the timeline pans horizontally without wrapping.
Run Story becomes the primary vertical surface. Debug evidence opens inline
rather than in a competing side panel.

## Copy

Lead with the operator's task and the actual state. Prefer “Start a new run from
here” over “Run from checkpoint,” “Generate replay” over “Render,” and
“Investigate with AI” over an unexplained handoff label. Explain unavailable
actions beside the control using the real backend reason.

## Prohibitions

- Do not recreate a generic analytics dashboard grid.
- Do not use retro pixel styling for normal interface text or controls.
- Do not display raw JSON, hashes, prompts, or long errors in the primary scan.
- Do not duplicate run facts across several panels.
- Do not imply semantic history exists for video frames where it was not
  persisted.
