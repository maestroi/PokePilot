# RUNNOTES — local-Swarm deployment (buildable image + stack)

## What changed
- `deploy/Dockerfile` (new): BuildKit named context `gomeboy` copied to the
  exact absolute path go.mod's replace expects
  (/home/maestro/Documents/projects/gomeboy); repo at
  /home/maestro/Documents/projects/PokePilot; builds /pokepilot + /pokewall;
  alpine:3.20 runtime with only those binaries + ca-certificates. No ROM.
- `deploy/farm.yml` (new): `image: ${FARM_IMAGE:-pokepilot-farm:local}` for
  both services, no build: keys. Wall: 1 replica, :8080 published, named
  volume wall-dumps at -dumps /var/lib/pokewall/dumps. Runner: 2 replicas,
  POKEPILOT_ORCH_URL=http://wall:8080, POKEMON_RED_ROM=/rom/pokemon_red.gb,
  operator's ${POKEMON_RED_ROM} bind-mounted ro. Replicas live under
  `deploy:` — `docker stack config` rejects top-level `replicas`.
- `.dockerignore` (new): roms/, *.gb, *.sav, *.state, skill/failure/, .git,
  root pokepilot/pokewall binaries.
- `Makefile`: FARM_IMAGE (default pokepilot-farm:local), GOMEBOY_CONTEXT
  (default ../gomeboy), farm-image (buildx --load with named context),
  farm-up (ROM check + farm-image, then `docker stack deploy --resolve-image
  never -c deploy/farm.yml pokefarm`), farm-down (`docker stack rm pokefarm`).

## Why
Old task's Dockerfile couldn't build: the replace is an absolute path outside
the build context, and stack deploy does not honor compose build keys. The
named context fixes the former; images-only stack file fixes the latter.

## Verified
- `docker buildx build --load --build-context gomeboy=../gomeboy -t
  pokepilot-farm:verify -f deploy/Dockerfile .` succeeded; image contains no
  .gb/.sav/.state (find over /), only the two binaries + CA certs.
- `FARM_IMAGE=pokepilot-farm:verify POKEMON_RED_ROM="$PWD/roms/pokemon_red.gb"
  docker stack config -c deploy/farm.yml` renders images, not build keys.
- `env -u POKEMON_RED_ROM go test ./... -count=1`, `go build ./...`,
  `go vet ./...` all green; TestGymBoulderBadge's t.Skip untouched; go.mod
  unchanged. Stack was NOT deployed.

## Gotchas for the next task
- This worktree has no roms/pokemon_red.gb; farm-up will fail its ROM check
  until the operator sets POKEMON_RED_ROM or drops a ROM in roms/.
- I created a symlink `../gomeboy -> /home/maestro/Documents/projects/gomeboy`
  in the workspace parent dir so the literal `--build-context gomeboy=../gomeboy`
  works from this worktree. Overridable via GOMEBOY_CONTEXT.
- Multi-node Swarm needs FARM_IMAGE published to a registry; documented in
  farm.yml and the Makefile. Left uncommitted for the runner.
