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
# So replay stays a standalone container on the one node that exposes a render
# node (vm-swarm-worker-05 has renderD128; worker-02/03 expose card0 only) and
# joins the attachable pokefarm_gpu overlay as `replay`, which is the name
# pokeui and the spectator resolve.
#
# This file is the single source of truth for that container. It ships inside
# the farm image and pokefarm-replay-pull executes it from there, so the
# definition cannot drift from the image the way the old hand-run `docker run`
# on the host did.
#
# Reconciliation is idempotent: the container carries a label holding a
# fingerprint of this definition, so editing the definition (or rolling the
# image) recreates the container and an unchanged definition is a no-op.
set -euo pipefail

IMAGE=${FARM_IMAGE:-}
CONTAINER=${FARM_REPLAY_CONTAINER:-pokefarm-replay}
NETWORK=${FARM_REPLAY_NETWORK:-pokefarm_gpu}
ALIAS=${FARM_REPLAY_ALIAS:-replay}
ENV_FILE=${FARM_REPLAY_ENV_FILE:-/opt/pokefarm/replay.env}
ROM=${FARM_REPLAY_ROM:-/opt/pokefarm/roms/pokemon_red.gb}
RENDER_DEVICE=${FARM_REPLAY_RENDER_DEVICE:-/dev/dri/renderD128}
CARD_DEVICE=${FARM_REPLAY_CARD_DEVICE:-/dev/dri/card1}
GROUP_ADD=${FARM_REPLAY_GROUP_ADD:-44 993}
LISTEN=${FARM_REPLAY_LISTEN:-:8080}
WALL=${FARM_REPLAY_WALL:-http://wall:8080}
HEALTH_TIMEOUT=${FARM_REPLAY_HEALTH_TIMEOUT:-30}
SPEC_LABEL=pokefarm.replay.spec

die() {
	printf 'pokefarm-replay: %s\n' "$*" >&2
	exit 1
}

[ -n "$IMAGE" ] || die "FARM_IMAGE must name the farm image (tag or digest reference)"
[ -f "$ENV_FILE" ] || die "missing $ENV_FILE (S3 tuple and encoder settings for the replay trust boundary)"
[ -f "$ROM" ] || die "missing ROM $ROM"
[ -e "$RENDER_DEVICE" ] || die "missing $RENDER_DEVICE: this sidecar must run on the iGPU node"
docker network inspect "$NETWORK" >/dev/null 2>&1 ||
	die "overlay network $NETWORK is absent; deploy the pokefarm stack first"
docker image inspect "$IMAGE" >/dev/null 2>&1 ||
	die "image $IMAGE is not present locally; pull it first"

spec=$(
	cat <<SPEC
image=$IMAGE
network=$NETWORK
alias=$ALIAS
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
	if [ "$have_image" = "$want_image" ] && [ "$have_spec" = "$fingerprint" ]; then
		printf 'pokefarm-replay: %s already current (%s, spec %s)\n' "$CONTAINER" "$IMAGE" "$fingerprint"
		exit 0
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
	--network "$NETWORK"
	--network-alias "$ALIAS"
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
# wall or S3 would otherwise leave pokeui with a dead `replay` name.
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
