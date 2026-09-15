#!/usr/bin/env bash
# Stable host bootstrap for the PokePilot farm updater.
#
# This file is the only deployment logic installed on the Swarm manager. It
# deliberately does not know how to roll individual services. Instead it pulls
# :latest, extracts the authoritative rollout script from that exact image
# digest, and runs it. Future rollout fixes therefore arrive with the image and
# cannot be blocked by an old /usr/local/sbin/pokefarm-pull copy.
set -euo pipefail

IMAGE=${FARM_IMAGE_REPO:-ghcr.io/maestroi/pokepilot}
BUNDLE_PATH=/usr/local/share/pokepilot/deploy

docker pull "${IMAGE}:latest"
DIGEST_REF=$(docker image inspect "${IMAGE}:latest" --format '{{index .RepoDigests 0}}')
if [ -z "${DIGEST_REF}" ] || [[ "${DIGEST_REF}" != *@* ]]; then
	echo "pokefarm-pull: ${IMAGE}:latest has no usable RepoDigest (${DIGEST_REF:-empty})" >&2
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

# Supply an explicit command because the farm image intentionally has no
# default CMD. docker cp works against a stopped container, so nothing from the
# image is executed during extraction.
cid=$(docker create "$DIGEST_REF" /bin/true)
docker cp "${cid}:${BUNDLE_PATH}/." "$tmpdir/"
docker rm -f "$cid" >/dev/null
cid=

ROLLOUT="$tmpdir/rollout-latest.sh"
if [ ! -f "$ROLLOUT" ]; then
	echo "pokefarm-pull: ${DIGEST_REF} is missing ${BUNDLE_PATH}/rollout-latest.sh" >&2
	exit 1
fi
chmod +x "$ROLLOUT"

# Pin the rollout to the same immutable digest whose script we just extracted.
# This prevents a moving :latest tag from changing between bootstrap and roll.
FARM_IMAGE_DIGEST_REF="$DIGEST_REF" "$ROLLOUT"
