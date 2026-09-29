#!/usr/bin/env bash
# Authoritative definition of the PokePilot replay sidecar.
#
# Why replay is not a Swarm service: on Docker 28 Swarm cannot hand the iGPU to
# a service. `devices:` passes `docker stack deploy`'s schema check but is
# dropped from the task -- the container starts with HostConfig.Devices=null and
# no /dev/dri -- `docker service create` has no --device flag at all, and
# `privileged: true` is dropped the same way. Bind-mounting /dev/dri as a volume
# makes the device nodes visible but opening them fails with EPERM, because only
# --device widens the device cgroup. All four measured on vm-swarm-worker-05.
#
# So replay is a standalone container on a dedicated render box
# (vm-pokefarm-render-01) that is NOT a Swarm node: as a worker it attracted
# farm tasks that starved the encoder. It publishes its HTTP port on the host,
# pokeui/spectator reach it by host address, and it reaches pokewall through
# the port the farm stack publishes (FARM_REPLAY_WALL).
#
# This file is the single source of truth for that container. It ships inside
# the farm image and pokefarm-replay-pull executes it from there, so the
# definition cannot drift from the image the way the old hand-run `docker run`
# on the host did.
#
# Reconciliation is idempotent: the container carries a label holding a
# fingerprint of this definition, so editing the definition (or rolling the
# image) recreates the container -- but only once /healthz reports no
# active_renders, so an image published mid-encode waits for the render to
# finish. An unchanged, running, healthy definition is a no-op; a stopped or
# unhealthy container is replaced immediately.
set -euo pipefail

IMAGE=${FARM_IMAGE:-}
CONTAINER=${FARM_REPLAY_CONTAINER:-pokefarm-replay}
PUBLISH=${FARM_REPLAY_PUBLISH:-8080:8080}
ENV_FILE=${FARM_REPLAY_ENV_FILE:-/opt/pokefarm/replay.env}
ROM=${FARM_REPLAY_ROM:-/opt/pokefarm/roms/pokemon_red.gb}
RENDER_DEVICE=${FARM_REPLAY_RENDER_DEVICE:-/dev/dri/renderD128}
CARD_DEVICE=${FARM_REPLAY_CARD_DEVICE:-/dev/dri/card1}
GROUP_ADD=${FARM_REPLAY_GROUP_ADD:-44 993}
LISTEN=${FARM_REPLAY_LISTEN:-:8080}
WALL=${FARM_REPLAY_WALL:-}
HEALTH_TIMEOUT=${FARM_REPLAY_HEALTH_TIMEOUT:-30}
SPEC_LABEL=pokefarm.replay.spec

die() {
	printf 'pokefarm-replay: %s\n' "$*" >&2
	exit 1
}

[ -n "$IMAGE" ] || die "FARM_IMAGE must name the farm image (tag or digest reference)"
[ -n "$WALL" ] || die "FARM_REPLAY_WALL must name pokewall's published URL (set it in /etc/default/pokefarm-replay)"
[ -f "$ENV_FILE" ] || die "missing $ENV_FILE (S3 tuple and encoder settings for the replay trust boundary)"
[ -f "$ROM" ] || die "missing ROM $ROM"
[ -e "$RENDER_DEVICE" ] || die "missing $RENDER_DEVICE: this sidecar must run on the iGPU node"
docker image inspect "$IMAGE" >/dev/null 2>&1 ||
	die "image $IMAGE is not present locally; pull it first"

spec=$(
	cat <<SPEC
image=$IMAGE
publish=$PUBLISH
env_file=$ENV_FILE
rom=$ROM
render=$RENDER_DEVICE
card=$CARD_DEVICE
group_add=$GROUP_ADD
listen=$LISTEN
wall=$WALL
SPEC
)
fingerprint=$(printf '%s' "$spec" | sha256sum | awk '{print substr($1, 1, 16)}')

want_image=$(docker image inspect "$IMAGE" --format '{{.Id}}')
if docker inspect "$CONTAINER" >/dev/null 2>&1; then
	have_image=$(docker inspect "$CONTAINER" --format '{{.Image}}')
	have_spec=$(docker inspect "$CONTAINER" --format "{{index .Config.Labels \"$SPEC_LABEL\"}}")
	health=
	if [ "$(docker inspect "$CONTAINER" --format '{{.State.Running}}')" = true ]; then
		health=$(docker exec "$CONTAINER" wget -qO- "http://127.0.0.1${LISTEN}/healthz" 2>/dev/null || true)
	fi
	if [ -n "$health" ]; then
		if [ "$have_image" = "$want_image" ] && [ "$have_spec" = "$fingerprint" ]; then
			printf 'pokefarm-replay: %s already current and healthy (%s, spec %s)\n' "$CONTAINER" "$IMAGE" "$fingerprint"
			exit 0
		fi
		# Never swap a healthy container mid-render: the next timer tick retries.
		# ponytail: an always-busy renderer defers updates indefinitely; add a max
		# deferral if the render queue ever stays non-empty for days.
		active=$(printf '%s' "$health" | sed -n 's/.*"active_renders":\([0-9][0-9]*\).*/\1/p')
		if [ "${active:-0}" -gt 0 ]; then
			printf 'pokefarm-replay: deferring update of %s: %s render(s) in flight\n' "$CONTAINER" "$active"
			exit 0
		fi
	fi
	printf 'pokefarm-replay: recreating %s (image %s -> %s, spec %s -> %s)\n' \
		"$CONTAINER" "$have_image" "$want_image" "$have_spec" "$fingerprint"
	docker rm -f "$CONTAINER" >/dev/null
else
	printf 'pokefarm-replay: creating %s (spec %s)\n' "$CONTAINER" "$fingerprint"
fi

args=(
	--detach
	--name "$CONTAINER"
	--restart unless-stopped
	--label "$SPEC_LABEL=$fingerprint"
	--publish "$PUBLISH"
	--env-file "$ENV_FILE"
	--device "$RENDER_DEVICE"
	--volume "$ROM:/rom/pokemon_red.gb:ro"
)
# card1 is the display node paired with renderD128 on this host. VAAPI encodes
# through the render node, so a host that names the pair differently only needs
# the render node to be present.
if [ -n "$CARD_DEVICE" ] && [ -e "$CARD_DEVICE" ]; then
	args+=(--device "$CARD_DEVICE")
fi
# The farm image runs as root, which already passes the video/render group
# check; the groups are kept so a future non-root USER still opens the node.
for gid in $GROUP_ADD; do
	args+=(--group-add "$gid")
done

docker run "${args[@]}" "$IMAGE" \
	pokereplay -http "$LISTEN" -wall "$WALL" -rom /rom/pokemon_red.gb

# Positive postcondition: the sidecar must answer its own health endpoint before
# this run counts as good. A container that is merely "started" but cannot reach
# wall or S3 would otherwise leave pokeui pointing at a dead replay URL.
deadline=$((SECONDS + HEALTH_TIMEOUT))
while :; do
	health=$(docker exec "$CONTAINER" wget -qO- "http://127.0.0.1${LISTEN}/healthz" 2>/dev/null || true)
	if [ -n "$health" ]; then
		printf 'pokefarm-replay: healthy %s\n' "$health"
		exit 0
	fi
	if [ "$SECONDS" -ge "$deadline" ]; then
		break
	fi
	sleep 1
done
printf 'pokefarm-replay: %s did not answer /healthz within %ss\n' "$CONTAINER" "$HEALTH_TIMEOUT" >&2
docker logs --tail 20 "$CONTAINER" >&2 || true
exit 1
