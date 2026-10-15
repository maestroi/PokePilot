# Documentation index

Start at the [repository overview](../README.md#layout) for the source layout.
The links below separate current guidance from project history.

## Contributing and architecture

- [Working rules](../AGENTS.md): repository workflow, probes, and PR requirements.
- [Development](DEVELOPMENT.md): local builds, ROM-free checks, and test selection.
- [Architecture](ARCHITECTURE.md): binding runtime and game-adapter boundaries.
- [Game profiles](GAME_PROFILES.md) and [Gen I boundaries](GEN1_BOUNDARIES.md): adapter contracts and shared engine ownership.
- [Qualification](QUALIFICATION.md): regression layers and the private replay corpus.
- [Technical design entry point](DESIGN.md): links to current technical contracts.
- [Product direction](../PRODUCT.md) and [operator visual system](../DESIGN.md): root documents for the product and UI.

## Gameplay and diagnosis

- [Agent loop](AGENT.md): objectives, planner behavior, and gameplay evidence.
- [Red](POKERED.md), [Yellow](POKEYELLOW.md), [Gold](POKEGOLD.md), and [Silver](POKESILVER.md): game/decomp references.
- [Boxxle](BOXXLE.md) and [Tetris](TETRIS.md): other game adapters.
- [Run inspector](RUN_INSPECTOR.md), [portable reproduction](PORTABLE_REPRO.md), and [RAM forensics](RAM_FORENSICS.md): failure investigation.
- [Road to the Elite Four](ROAD-TO-ELITE-FOUR.md): campaign progression beyond Cerulean City.
- [ROM debugging](ROM_DEBUG.md) and [reverse engineering](ROM_REVERSE_ENGINEERING.md): ROM investigation tools.

## Services, deployment, and media

- [Deployment](../deploy/README.md): farm containers and Swarm operations.
- [Domains](DOMAINS.md), [spectator](SPECTATOR.md), and [MCP](MCP.md): public/operator service boundaries.
- [Artifact storage](S3_ARTIFACT_STORAGE.md): durable farm artifacts.
- [Media engine](MEDIA_ENGINE.md), [render state](RENDER_STATE.md), and [render themes](RENDER_THEMES.md): media and rendering contracts.
- [Telegram operator](TELEGRAM_OPERATOR.md): operator integration.

## Notes and historical material

- [Engineering run notes](RUNNOTES.md) retain measurements and decisions.
- [Root handoff](../RUNNOTES.md) is a short pointer for the next task.
- `plans/` and `superpowers/` contain task designs and implementation plans;
  check the associated issue/PR for their implementation status.
- `archive/` retains historical context. It does not override current contracts
  or the binding architecture rules.

Add new durable guidance to the relevant section here. Keep task handoffs and
scratch measurements out of the repository root; use the current issue/PR for
handoffs and follow the scratch-file rules in [AGENTS.md](../AGENTS.md).
