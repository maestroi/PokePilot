# Calm Navy Operator Console Design

## Intent

Make the private PokéFarm operator console feel calmer, friendlier, and less fatiguing during long sessions while preserving its current information architecture, dimensions, density, and interactions.

## Visual direction

Use a calm navy workstation palette. The page canvas becomes a deep navy rather than near-black, while panels sit only slightly above it. Fine blue-gray separators replace bright box outlines. Text remains warm and readable without becoming stark white. Cyan, green, amber, and red are softened and reserved for selection, health, checkpoints, and failures respectively.

The Game Boy frame and semantic map remain the most visually prominent surfaces. Chrome around them recedes. The coral rule beneath the application header is removed because it reads as a permanent alert. Selected states use a restrained blue tint rather than a strongly colored slab.

## Scope

- Change shared color tokens and direct hard-coded surface colors in `cmd/pokeui/ui/console.css`.
- Soften borders, inputs, buttons, status chips, timeline chrome, and the run rail through those tokens.
- Keep the current logo, component layout, dimensions, spacing, typography sizes, content, and behavior unchanged.
- Retain readable contrast, keyboard focus, and semantic status distinctions.

## Verification

- Add a static UI regression that pins the calm navy tokens and ensures the coral header rule is absent.
- Run the PokeUI tests and the full ROM-free repository verification gate.
- Rebuild with `make farm-up` and confirm the live CSS matches the source served at `http://localhost:18080/`.
