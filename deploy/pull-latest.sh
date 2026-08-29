#!/usr/bin/env bash
# Pin pokefarm Swarm services to the digest currently tagged :latest.
# Safe to commit: image name and stack name only, no hosts, tokens, or ROM.
set -euo pipefail

IMAGE=${FARM_IMAGE_REPO:-ghcr.io/maestroi/pokepilot}
STACK=${FARM_STACK:-pokefarm}
SERVICES=(wall ui runner)

if ! docker service inspect "${STACK}_wall" >/dev/null 2>&1; then
	echo "pokefarm-pull: stack ${STACK} not deployed; skip"
	exit 0
fi

docker pull "${IMAGE}:latest"
DIGEST_REF=$(docker image inspect "${IMAGE}:latest" --format '{{index .RepoDigests 0}}')
if [ -z "${DIGEST_REF}" ]; then
	echo "pokefarm-pull: ${IMAGE}:latest has no RepoDigest after pull" >&2
	exit 1
fi
WANT=${DIGEST_REF##*@}

updated=0
for name in "${SERVICES[@]}"; do
	svc="${STACK}_${name}"
	img=$(docker service inspect "$svc" --format '{{.Spec.TaskTemplate.ContainerSpec.Image}}')
	case "$img" in
	*@*) cur=${img##*@} ;;
	*) cur= ;;
	esac
	if [ "$cur" = "$WANT" ]; then
		echo "pokefarm-pull: $svc already $WANT"
		continue
	fi
	echo "pokefarm-pull: $svc $img -> $DIGEST_REF"
	docker service update --detach --with-registry-auth --image "$DIGEST_REF" "$svc" >/dev/null
	updated=$((updated + 1))
done

if [ "$updated" -eq 0 ]; then
	echo "pokefarm-pull: already current ($WANT)"
else
	echo "pokefarm-pull: updated $updated service(s) to $WANT"
fi
