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

# pokewatch (deploy/ops.yml) freezes deploys after repeated Swarm rollbacks by
# labeling its own service (never a farm service: a label-only update resets
# that service's UpdateStatus and would break the rollback hold below). Runs
# keep playing on the current image; the freeze lifts by itself at the
# timestamp. No ops stack means no freeze.
FREEZE_SERVICE=${FARM_FREEZE_SERVICE:-pokefarm-ops_watch}
frozen_until=$(docker service inspect "$FREEZE_SERVICE" --format '{{index .Spec.Labels "pokepilot.deploy-frozen-until"}}' 2>/dev/null || true)
if [[ "$frozen_until" =~ ^[0-9]+$ ]] && [ "$frozen_until" -gt "$(date +%s)" ]; then
	echo "pokefarm-pull: deploys frozen until $(date -u -d "@$frozen_until" '+%F %T UTC') after repeated rollbacks; skip"
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

# UNUSABLE_NODES is a "|host|host|" list of nodes the manager can no longer act
# on: Down nodes, and managers it cannot reach. Their tasks keep reporting
# Running at their old image forever, because Swarm has no agent left to stop
# them. Such a task is not evidence of a stuck rollout (see the Running-task
# scan below), so it must not be counted as one.
UNUSABLE_NODES="|"
if node_rows=$(docker node ls --format '{{.Hostname}}|{{.Status}}|{{.ManagerStatus}}' 2>/dev/null); then
	while IFS='|' read -r node_host node_status node_manager_status; do
		[ -n "$node_host" ] || continue
		if [ "$node_status" != "Ready" ] || [ "$node_manager_status" = "Unreachable" ]; then
			UNUSABLE_NODES="${UNUSABLE_NODES}${node_host}|"
		fi
	done <<< "$node_rows"
fi

# node_unusable reports whether host is one of those nodes.
node_unusable() {
	case "$UNUSABLE_NODES" in
	*"|$1|"*) return 0 ;;
	*) return 1 ;;
	esac
}

# Every update below asks Swarm to roll a service back when its new tasks fail
# inside the monitor window (an image that crash-loops at boot). PreviousSpec
# is then the rejected image. Re-rolling that digest every tick would crash the
# farm forever, so hold until a newer image is published: the next merge gets
# a fresh digest and is tried normally.
for name in "${SERVICES[@]}"; do
	rolled=$(docker service inspect "${STACK}_${name}" \
		--format '{{if .UpdateStatus}}{{.UpdateStatus.State}}{{end}}|{{if .PreviousSpec}}{{.PreviousSpec.TaskTemplate.ContainerSpec.Image}}{{end}}' 2>/dev/null || true)
	case "$rolled" in
	rollback_*"|"*"@$WANT")
		echo "pokefarm-pull: Swarm rolled ${STACK}_${name} back from $WANT; holding rollout until a newer image"
		exit 0
		;;
	esac
done

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
	# Runners drain on SIGTERM: finish the in-flight model round, flush the
	# checkpoint, and report Finish(reason=drained). Swarm's default 10s stop
	# grace SIGKILLs a busy runner mid-round, so every deploy surfaced as
	# "lost: no heartbeat" and spent resilient recovery budget. deploy/farm.yml
	# carries the same 6m, but the swarm stack file is host-local and drifted,
	# so the image-owned rollout enforces it. start-first with parallelism 0
	# brings up every replacement at once instead of serializing 10 drains.
	lifecycle=(--update-failure-action rollback --update-monitor 60s)
	if [ "$name" = runner ]; then
		lifecycle+=(--stop-grace-period 6m --update-order start-first --update-parallelism 0)
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
	#
	# Only tasks on nodes the manager can still act on count. A task stranded on
	# a Down or unreachable node stays "Running" at the old digest forever, so
	# counting it stale made every timer tick force-roll an otherwise healthy
	# fleet — a rollout that never converges, restarting runners (and killing
	# their in-flight runs) every two minutes indefinitely.
	running=0
	stale_running=0
	stranded_running=0
	while IFS='|' read -r current_state task_image task_node desired_state; do
		case "$current_state" in
		Running\ *) ;;
		*) continue ;;
		esac
		# A replaced runner keeps Running at the old digest for up to its 6m
		# drain while Swarm already wants it Shutdown (start-first). Counting
		# it stale force-rolled every fresh runner each tick, re-draining
		# busy runs every two minutes. It is leaving, not stuck.
		if [ -n "$desired_state" ] && [ "$desired_state" != Running ]; then
			continue
		fi
		if node_unusable "$task_node"; then
			stranded_running=$((stranded_running + 1))
			continue
		fi
		running=$((running + 1))
		case "$task_image" in
		*@*) task_digest=${task_image##*@} ;;
		*) task_digest= ;;
		esac
		if [ "$task_digest" != "$WANT" ]; then
			stale_running=$((stale_running + 1))
		fi
	done < <(docker service ps --no-trunc --format '{{.CurrentState}}|{{.Image}}|{{.Node}}|{{.DesiredState}}' "$svc" 2>/dev/null || true)

	if [ "$stranded_running" -gt 0 ]; then
		echo "pokefarm-pull: $svc has $stranded_running Running task(s) on node(s) Swarm cannot act on; not counting them as stale"
	fi

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
		docker service update --force --detach --with-registry-auth --image "$DIGEST_REF" ${lifecycle[@]+"${lifecycle[@]}"} "$svc" >/dev/null
		updated=$((updated + 1))
		continue
	fi

	echo "pokefarm-pull: $svc $img -> $DIGEST_REF"
	docker service update --detach --with-registry-auth --image "$DIGEST_REF" ${lifecycle[@]+"${lifecycle[@]}"} "$svc" >/dev/null
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

# --- reclaim untagged images ------------------------------------------------
# Every merge publishes a new digest and this timer pulls it within ~2 minutes,
# which leaves the previous digest untagged and nothing ever removed it. On
# 2026-10-01 the manager's 49G root filesystem reached 99M free holding 296
# untagged pokepilot images (33.75G reclaimed). The Swarm control plane and
# pokefarm_postgres share that filesystem, so filling it takes the whole farm
# down -- and it did so silently, because nothing measured free space. Reclaim
# on every tick rather than waiting for a human to notice.
#
# Only untagged tags of IMAGE are candidates, and the keep-set is every digest
# the stack can still be asked to run: each service's spec, its PreviousSpec
# (the rollback target the hold logic above depends on), and any live task.
# Dropping a rollback target would force `docker service rollback` to re-pull
# from the registry, which is the exact situation that logic exists to avoid.
prune_untagged_images() {
	local keep=$'\n' svc ref id digest referenced removed=0
	local -a candidates=()
	for svc in "${SERVICES[@]}" litellm; do
		svc="${STACK}_${svc}"
		docker service inspect "$svc" >/dev/null 2>&1 || continue
		while read -r ref; do
			case "$ref" in
			*@*) keep="${keep}${ref##*@}"$'\n' ;;
			esac
		done < <(docker service inspect "$svc" --format \
			'{{.Spec.TaskTemplate.ContainerSpec.Image}}
{{if .PreviousSpec}}{{.PreviousSpec.TaskTemplate.ContainerSpec.Image}}{{end}}' 2>/dev/null || true)
		while read -r ref; do
			case "$ref" in
			*@*) keep="${keep}${ref##*@}"$'\n' ;;
			esac
		done < <(docker service ps --no-trunc --format '{{.Image}}' "$svc" 2>/dev/null || true)
	done

	# Collect first: removing images while the listing command that produced
	# them is still running reads a half-mutated list.
	while read -r id; do
		[ -n "$id" ] && candidates+=("$id")
	done < <(docker images --no-trunc --filter "reference=${IMAGE}" \
		--format '{{.ID}} {{.Tag}}' 2>/dev/null | awk '$2 == "<none>" {print $1}')

	for id in ${candidates[@]+"${candidates[@]}"}; do
		referenced=0
		while read -r digest; do
			[ -n "$digest" ] || continue
			case "$keep" in
			*$'\n'"$digest"$'\n'*)
				referenced=1
				break
				;;
			esac
		done < <(docker image inspect "$id" --format '{{range .RepoDigests}}{{.}}
{{end}}' 2>/dev/null | sed 's/.*@//' || true)
		[ "$referenced" -eq 0 ] || continue
		docker rmi "$id" >/dev/null 2>&1 && removed=$((removed + 1))
	done
	if [ "$removed" -gt 0 ]; then
		echo "pokefarm-pull: pruned $removed untagged ${IMAGE} image(s)"
	fi
	return 0
}
prune_untagged_images
