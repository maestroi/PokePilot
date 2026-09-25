#!/usr/bin/env bash
# Stable worker-side bootstrap for the PokePilot replay sidecar.
#
# Mirrors deploy/pull-latest.sh on the manager: it knows only how to pull
# :latest, extract the authoritative sidecar definition from that exact image
# digest, and run it. The container definition therefore travels with the image,
# and a stale host copy of this bootstrap cannot pin the farm to old sidecar
# behavior.
#
# The manager cannot roll this container itself: Swarm cannot pass /dev/dri to a
# service, and the manager has no SSH trust into the iGPU worker. So the worker
# owns its own reconciliation timer (pokefarm-replay-pull.timer).
set -euo pipefail

IMAGE=${FARM_IMAGE_REPO:-ghcr.io/maestroi/pokepilot}
BUNDLE_PATH=/usr/local/share/pokepilot/deploy
SIDECAR=replay-sidecar.sh

docker pull "${IMAGE}:latest"
DIGEST_REF=$(docker image inspect "${IMAGE}:latest" --format '{{index .RepoDigests 0}}')
if [ -z "${DIGEST_REF}" ] || [[ "${DIGEST_REF}" != *@* ]]; then
	printf 'pokefarm-replay-pull: %s:latest has no usable RepoDigest (%s)\n' \
		"$IMAGE" "${DIGEST_REF:-empty}" >&2
	exit 1
fi

tmpdir=$(mktemp -d)
cid=
cleanup() {
	if [ -n "$cid" ]; then
		docker rm -f "$cid" >/dev/null 2>&1 || true
	fi
	rm -rf "$tmpdir"
}
trap cleanup EXIT

# The farm image intentionally has no default CMD; supply one so nothing from
# the image executes during extraction. docker cp works against a stopped
# container.
cid=$(docker create "$DIGEST_REF" /bin/true)
docker cp "${cid}:${BUNDLE_PATH}/${SIDECAR}" "$tmpdir/${SIDECAR}"
docker rm -f "$cid" >/dev/null
cid=

chmod +x "$tmpdir/${SIDECAR}"

# Pin the sidecar to the same immutable digest whose definition we extracted, so
# a moving :latest tag cannot change between extraction and reconcile.
FARM_IMAGE="$DIGEST_REF" "$tmpdir/${SIDECAR}"
