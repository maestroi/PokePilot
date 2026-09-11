#!/usr/bin/env bash
# Pin pokefarm Swarm services to the digest currently tagged :latest.
# Safe to commit: image name and stack name only, no hosts, tokens, or ROM.
set -euo pipefail

IMAGE=${FARM_IMAGE_REPO:-ghcr.io/maestroi/pokepilot}
STACK=${FARM_STACK:-pokefarm}
# Replay is a device-bound sidecar on the iGPU worker, not a Swarm service.
# When FARM_REPLAY_HOST is set (manager unit), roll that container to :latest
# too. Without it this script used to print "pokefarm_replay not deployed; skip"
# forever while pokeui required newer replay APIs (DELETE /artifacts).
SERVICES=(wall ui spectator runner)

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
	# New service roles can land in the stack file before an operator has run
	# the next docker stack deploy. Skip those rather than making the timer fail;
	# once the service exists, it joins the normal digest rollout automatically.
	if ! docker service inspect "$svc" >/dev/null 2>&1; then
		echo "pokefarm-pull: $svc not deployed; skip"
		continue
	fi
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

if [ -n "${FARM_REPLAY_HOST:-}" ]; then
	echo "pokefarm-pull: updating replay sidecar on $FARM_REPLAY_HOST"
	ssh -o BatchMode=yes -o ConnectTimeout=15 "$FARM_REPLAY_HOST" \
		"FARM_IMAGE=${DIGEST_REF} /usr/local/sbin/pokefarm-replay-up" \
		|| echo "pokefarm-pull: replay sidecar update failed" >&2
fi

# litellm.yaml only names env vars (os.environ/POKEPILOT_LITELLM_*); the
# actual physical URLs/models live in the litellm service's own env, not in
# this file, so rolling its config here needs no secrets. A merged PR that
# changes routing/thinking behavior (e.g. #203) must reach the running
# gateway on its own, the way image changes already do above, or the fix
# only ever takes effect on the next manual `make farm-up`.
LITELLM_SVC="${STACK}_litellm"
LITELLM_YAML="$(dirname "$0")/litellm.yaml"
if docker service inspect "$LITELLM_SVC" >/dev/null 2>&1 && [ -f "$LITELLM_YAML" ]; then
	CUR_CONFIG=$(docker service inspect "$LITELLM_SVC" \
		--format '{{(index .Spec.TaskTemplate.ContainerSpec.Configs 0).ConfigName}}')
	TARGET=$(docker service inspect "$LITELLM_SVC" \
		--format '{{(index .Spec.TaskTemplate.ContainerSpec.Configs 0).File.Name}}')
	WANT_HASH=$(sha256sum "$LITELLM_YAML" | cut -c1-12)
	NEW_CONFIG="${STACK}_litellm_yaml_${WANT_HASH}"
	if [ "$CUR_CONFIG" = "$NEW_CONFIG" ]; then
		echo "pokefarm-pull: $LITELLM_SVC config already $NEW_CONFIG"
	else
		docker config create "$NEW_CONFIG" "$LITELLM_YAML" >/dev/null
		echo "pokefarm-pull: $LITELLM_SVC $CUR_CONFIG -> $NEW_CONFIG"
		docker service update --detach \
			--config-rm "$CUR_CONFIG" \
			--config-add "source=${NEW_CONFIG},target=${TARGET}" \
			"$LITELLM_SVC" >/dev/null
	fi
elif [ -f "$LITELLM_YAML" ]; then
	echo "pokefarm-pull: $LITELLM_SVC not deployed; skip"
fi
