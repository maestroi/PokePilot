# PokePilot Operator Console Visual System

## Direction

The private operator console uses the spatial logic of a live broadcast control
room: one selected run owns the working surface, related views stay synchronized,
other runs remain available as compact sources, and transport controls govern
review. The interface uses ordinary PokePilot language such as Game, Map, Run
Story, and Start from checkpoint.

## Physical Scene

An operator uses this console for long desktop sessions in a dim workstation or
homelab environment. The Game Boy image and semantic map need visual priority
without bright chrome competing with them.

## Color

Use a restrained dark strategy. Charcoal and blue-black fields define major
working regions. Cool gray rules and raised hardware-gray controls establish
structure. The game frame keeps its original color. Cyan marks selection and
live position, amber marks checkpoints and recoverable attention, green marks
confirmed progress, and red is reserved for failures or destructive actions.

Status must always include text, shape, or iconography in addition to color.
Avoid glow, gradients used as decoration, and large saturated background areas.

## Typography

Use a compact workhorse sans for interface copy and a tabular monospaced face
for run IDs, frames, timestamps, coordinates, hashes, and counters. Sentence
case is the default. Uppercase labels are reserved for short equipment-like
legends and must remain readable at normal size.

## Composition

- The selected run occupies most of the viewport.
- Game and Semantic Map form one paired visual unit.
- The active objective and latest planner decision stay visible near the paired
  views.
- The transport timeline spans the working surface and owns chronological
  navigation.
- Active and recent runs appear in a compact source rail.
- Run Story is a continuous chronological reading surface below the transport.
- Fleet, workers, run creation, and aggregate failures live in a secondary
  Operations surface.

## Components

Panels should read as fitted equipment bays: square or lightly rounded corners,
precise one-pixel boundaries, compact headers, and clear internal alignment.
Avoid a page made from interchangeable floating cards.

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

On narrow screens, the run rail becomes a drawer, Game and Map stack, and the
timeline pans horizontally without wrapping. Run Story becomes the primary
vertical surface. Debug evidence opens inline rather than in a competing side
panel.

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
