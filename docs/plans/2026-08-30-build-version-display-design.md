# Build Version Display — Design

**Date:** 2026-08-30
**Status:** approved (maestro, 2026-08-30)

## Context

After a deploy, confirming that all ten runners plus wall and console rolled to
the new image means SSHing to the manager and reading image digests. The
operator console (pokemon.labstack.cc) shows worker presence — address, busy/idle,
last seen — but no build identity. A partially-rolled fleet (8 of 10 runners on
the new SHA) is invisible until someone checks by hand.

## Goal

One glance at the wall/console answers: *is every worker on the build I just
shipped?*

## Decisions

- **Identity is the git SHA, not the image digest.** It matches `git log` in one
  glance; CI already has it as `${{ github.sha }}`. The digest is what Swarm pins
  internally but is a 64-character blob nobody reads.
- **Stamp at image build time.** One image carries all three binaries, so one
  build-arg covers the whole fleet: `GIT_SHA` → `-ldflags "-X main.version=…"`.
  Local builds without the arg get `"dev"`.
- **Worker versions ride existing presence pings.** Runners already ping the wall
  on every lease attempt and carry addresses in heartbeats. No new endpoint for
  worker data; the version is just another field on messages that already flow.

## Data flow

1. **Build.** `.github/workflows/publish-farm.yml` passes
   `build-args: GIT_SHA=${{ github.sha }}`; `deploy/Dockerfile` declares
   `ARG GIT_SHA` and adds `-ldflags "-X main.version=${GIT_SHA}"` to all three
   `go build` lines. Each binary (`cmd/pokepilot`, `cmd/pokewall`,
   `cmd/pokeui`) declares `var version = "dev"` in package main.
2. **Runner → wall.** `farm.Client` gains an exported `Version` field, set once
   from the local `version` in `cmd/pokepilot/main.go`. The client stamps it on
   the wire itself: `Ping` puts it in `WorkerPing`, `Heartbeat` sets
   `hb.Version` before marshaling. Both paths matter — the wall upserts its
   worker record from *both* idle pings and run heartbeats, so a busy runner
   must not clear the version.
3. **Wall.** `workerInfo` and `workerRow` gain `Version` (JSON `version,omitempty`);
   `upsertWorkerLocked` takes it; the debug HTML workers table gains a `version`
   column. The wall's own build appears as `wall_version` in the dashboard JSON
   and in the debug page header. `NewWall` keeps its signature (21 test call
   sites); main sets an exported `Wall.Version` field.
4. **Console.** pokeui serves `GET /v1/version` → `{"version":"…"}` for its own
   build. The header shows console + wall SHAs; each worker chip shows its SHA;
   the Workers section opens with a one-line distribution — `10 × 96eadf9`, or
   `8 × 96eadf9, 2 × 2fce1f9` mid-rollout. The distribution line is the actual
   answer to "did everything update"; per-chip SHAs name the stragglers.

## Compatibility (rolling upgrade)

This feature is observed *during* mixed-version fleets, so it must degrade
cleanly:

- New wall + old runner: empty version → `omitempty` keeps JSON clean, chip
  shows `—`.
- Old wall + new runner: unknown JSON field ignored.
- Worker versions are ephemeral presence data — never persisted, nothing to
  migrate.

## Verification

- Unit: farm client sends the version on ping and heartbeat; wall dashboard JSON
  carries worker versions and `wall_version`; debug HTML has the column; pokeui
  `/v1/version` responds.
- End-to-end (the feature's own acceptance test): push → CI build →
  `./deploy/swarm.sh pull` → console shows all ten workers on the new SHA, with
  console and wall SHAs in the header matching `git rev-parse --short HEAD`.
