#!/usr/bin/env bash
# Roll pokefarm Swarm services to one exact published image digest.
#
# This script is intentionally image-owned rather than host-owned. The stable
# /usr/local/sbin/pokefarm-pull bootstrap extracts this file and litellm.yaml
# from the exact :latest digest it just pulled, then invokes it with
# FARM_IMAGE_DIGEST_REF set. That keeps rollout behavior in lockstep with the
# image and avoids a stale manager running months-old deployment logic.
set -euo pipefail

IMAGE=${FARM_IMAGE_REPO:-ghcr.io/maestroi/pokepilot}
STACK=${FARM_STACK:-pokefarm}
# Replay is deliberately absent from this list. Swarm cannot pass /dev/dri to a
# service, so the iGPU sidecar is a standalone container reconciled on its own
# worker by pokefarm-replay-pull.timer (see deploy/replay-sidecar.sh). The
# manager must not try to roll it over SSH: it has no trust into that worker, so
# the old FARM_REPLAY_HOST branch could never converge and silently left the
# sidecar on whatever image an operator had last run by hand.
SERVICES=(wall issues ui spectator runner linkbroker virtualtrader)

if ! docker service inspect "${STACK}_wall" >/dev/null 2>&1; then
	echo "pokefarm-pull: stack ${STACK} not deployed; skip"
	exit 0
fi

if [ -n "${FARM_IMAGE_DIGEST_REF:-}" ]; then
	DIGEST_REF=${FARM_IMAGE_DIGEST_REF}
else
	docker pull "${IMAGE}:latest"
	DIGEST_REF=$(docker image inspect "${IMAGE}:latest" --format '{{index .RepoDigests 0}}')
fi
if [ -z "${DIGEST_REF}" ] || [[ "${DIGEST_REF}" != *@* ]]; then
	echo "pokefarm-pull: ${IMAGE}:latest has no usable RepoDigest (${DIGEST_REF:-empty})" >&2
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

	# The service spec can already point at WANT while a failed/paused Swarm
	# rollout leaves an older task alive indefinitely. Looking only at .Spec made
	# the timer print "already current" forever while old runners kept leasing
	# work and reporting pre-fix failures. Inspect the actual Running tasks too.
	running=0
	stale_running=0
	while IFS='|' read -r current_state task_image; do
		case "$current_state" in
		Running\ *) ;;
		*) continue ;;
		esac
		running=$((running + 1))
		case "$task_image" in
		*@*) task_digest=${task_image##*@} ;;
		*) task_digest= ;;
		esac
		if [ "$task_digest" != "$WANT" ]; then
			stale_running=$((stale_running + 1))
		fi
	done < <(docker service ps --no-trunc --format '{{.CurrentState}}|{{.Image}}' "$svc" 2>/dev/null || true)

	if [ "$cur" = "$WANT" ] && [ "$running" -gt 0 ] && [ "$stale_running" -eq 0 ]; then
		echo "pokefarm-pull: $svc already $WANT ($running running task(s))"
		continue
	fi

	if [ "$cur" = "$WANT" ]; then
		update_state=$(docker service inspect "$svc" --format '{{if .UpdateStatus}}{{.UpdateStatus.State}}{{end}}' 2>/dev/null || true)
		if [ "$update_state" = "updating" ]; then
			echo "pokefarm-pull: $svc rollout still updating ($stale_running stale of $running running task(s)); defer force-roll"
			continue
		fi
		if [ "$running" -eq 0 ]; then
			echo "pokefarm-pull: $svc spec is $WANT but has no Running tasks; force-roll"
		else
			echo "pokefarm-pull: $svc spec is $WANT but $stale_running/$running Running task(s) are stale; force-roll"
		fi
		docker service update --force --detach --with-registry-auth --image "$DIGEST_REF" "$svc" >/dev/null
		updated=$((updated + 1))
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

# litellm.yaml only names env vars (os.environ/POKEPILOT_LITELLM_*); the
# actual physical URLs/models live in the litellm service's own env, not in
# this file, so rolling its config here needs no secrets. A merged PR that
# changes routing/thinking behavior must reach the running gateway on its own,
# the way image changes already do above, or the fix only ever takes effect on
# the next manual stack deploy.
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
		# The content-addressed config can already exist while the service sits
		# on a hand-made name (e.g. _v3); creating it again fails the whole
		# timer every run, so reuse it.
		if ! docker config inspect "$NEW_CONFIG" >/dev/null 2>&1; then
			docker config create "$NEW_CONFIG" "$LITELLM_YAML" >/dev/null
		fi
		echo "pokefarm-pull: $LITELLM_SVC $CUR_CONFIG -> $NEW_CONFIG"
		docker service update --detach \
			--config-rm "$CUR_CONFIG" \
			--config-add "source=${NEW_CONFIG},target=${TARGET}" \
			"$LITELLM_SVC" >/dev/null
	fi
elif [ -f "$LITELLM_YAML" ]; then
	echo "pokefarm-pull: $LITELLM_SVC not deployed; skip"
fi
