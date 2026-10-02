#!/usr/bin/env bash
# Unattended farm watchdog: checks the farm, fixer, and deploy health and
# messages a Telegram chat when a check turns bad, every 12h while it stays
# bad, and when it recovers. Sends one digest per day. State lives in
# $POKEPILOT_WATCH_STATE; a check that never changes never re-notifies.
#
# Required in ~/.config/pokepilot/env: TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID,
# POKEPILOT_WALL_URL (http://<swarm-node>:18080). Optional:
# POKEPILOT_SWARM_MANAGER (ssh target) enables the rollback and manager-disk
# checks. Optional: POKEPILOT_WATCH_MIN_FREE_GB (default 20) is the free space
# below which a filesystem pages.
set -uo pipefail

ENV_FILE=${POKEPILOT_ENV:-$HOME/.config/pokepilot/env}
if [ -f "$ENV_FILE" ]; then
	set -a
	# shellcheck disable=SC1090
	. "$ENV_FILE"
	set +a
fi

# systemd starts this in $HOME, where gh cannot infer the repository.
export GH_REPO=${GH_REPO:-${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}}
STATE=${POKEPILOT_WATCH_STATE:-$HOME/.local/share/pokepilot/farm-watch}
TRIAGE_STATE=${POKEPILOT_TRIAGE_STATE:-$HOME/.local/share/pokepilot/qwagent-triage}
# Swarm fixer (deploy/fixer.yml): set the service name; point TRIAGE_STATE at
# one replica's state dir and the ledger at the shared one.
FIXER_SERVICE=${POKEPILOT_FIXER_SERVICE:-}
ledger=${POKEPILOT_TRIAGE_LEDGER:-$TRIAGE_STATE/ledger.tsv}
PAID_CAP=${POKEPILOT_PAID_DAILY_CAP:-20}
STALL_SECONDS=${POKEPILOT_WATCH_STALL_SECONDS:-1800}
REMIND_SECONDS=43200
DIGEST_HOUR=${POKEPILOT_WATCH_DIGEST_HOUR:-9}
mkdir -p "$STATE"
now=$(date +%s)

notify() {
	if [ -z "${TELEGRAM_BOT_TOKEN:-}" ] || [ -z "${TELEGRAM_CHAT_ID:-}" ]; then
		echo "farm-watch: (no Telegram configured) $1" >&2
		return 0
	fi
	curl -fsS -m 20 -o /dev/null "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
		--data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" \
		--data-urlencode "text=$1" ||
		echo "farm-watch: telegram send failed" >&2
}

# report NAME GRACE [MESSAGE]: an empty MESSAGE means the check is OK. A check
# must be bad on GRACE consecutive ticks before it notifies, so one flaky
# probe during a deploy does not page.
report() {
	local name=$1 grace=$2 msg=${3:-} f="$STATE/$1" count=0 notified=0
	[ -f "$f" ] && read -r count notified <"$f"
	if [ -z "$msg" ]; then
		[ "$notified" -gt 0 ] && notify "✅ PokePilot recovered: $name"
		rm -f "$f"
		return
	fi
	count=$((count + 1))
	if [ "$count" -ge "$grace" ] && { [ "$notified" -eq 0 ] || [ $((now - notified)) -ge "$REMIND_SECONDS" ]; }; then
		[ "$notified" -eq 0 ] && notify "🔴 PokePilot: $msg" || notify "🔴 PokePilot still: $msg"
		notified=$now
	fi
	echo "$count $notified" >"$f"
}

# --- farm: wall reachable and some run's frame counter moving -------------
dash="$STATE/dashboard.json"
if [ -z "${POKEPILOT_WALL_URL:-}" ]; then
	report wall 1 "POKEPILOT_WALL_URL is not set; farm checks are off"
elif curl -fsS -m 30 -o "$dash" "$POKEPILOT_WALL_URL/v1/dashboard"; then
	report wall 2 ""
	read -r running frames < <(python3 -c '
import json, sys
runs = json.load(open(sys.argv[1]))["runs"]
live = [r for r in runs if r.get("status") in ("running", "leased")]
print(len(live), sum(int(r.get("frame") or 0) for r in live))
' "$dash")
	last_frames="" last_change=$now
	[ -f "$STATE/frames" ] && read -r last_change last_frames <"$STATE/frames"
	if [ "$frames" != "$last_frames" ]; then
		echo "$now $frames" >"$STATE/frames"
		last_change=$now
	fi
	if [ "$running" -eq 0 ]; then
		report progress 2 "no runs are running"
	elif [ $((now - last_change)) -ge "$STALL_SECONDS" ]; then
		report progress 1 "$running runs, no frame progress for $(((now - last_change) / 60))m"
	else
		report progress 1 ""
	fi
else
	report wall 2 "wall $POKEPILOT_WALL_URL unreachable"
fi

# --- disk: a full filesystem stops the farm, and silently ------------------
# The Swarm manager's root filesystem carries the control plane and
# pokefarm_postgres. On 2026-10-01 it reached 99M free while holding 296
# untagged image layers, and nothing noticed, because no check measured free
# space; the farm would have died with no page. Absolute free space is the
# metric that generalizes: a percentage threshold means nothing across a 49G VM
# and a 1.8T workstation.
MIN_FREE_GB=${POKEPILOT_WATCH_MIN_FREE_GB:-20}

# free_gb prints whole gigabytes available on a mount, or nothing if df failed.
free_gb() {
	df -Pk "$1" 2>/dev/null | awk 'NR == 2 {printf "%d", $4 / 1048576}'
}

if [ -n "${POKEPILOT_SWARM_MANAGER:-}" ]; then
	# ssh can drop a connection mid-deploy, so this one keeps a grace tick.
	manager_free=$(timeout 60 ssh -o BatchMode=yes -o ConnectTimeout=10 "$POKEPILOT_SWARM_MANAGER" 'df -Pk /' 2>/dev/null |
		awk 'NR == 2 {printf "%d", $4 / 1048576}')
	if [ -n "$manager_free" ] && [ "$manager_free" -lt "$MIN_FREE_GB" ]; then
		report disk-manager 2 "swarm manager ${POKEPILOT_SWARM_MANAGER#*@} has ${manager_free}G free (under ${MIN_FREE_GB}G); it holds the control plane and pokefarm_postgres"
	else
		report disk-manager 2 ""
	fi
fi

# A local df cannot flake, so no grace: a full disk here breaks the fixer.
local_free=$(free_gb /)
if [ -n "$local_free" ] && [ "$local_free" -lt "$MIN_FREE_GB" ]; then
	report disk-local 1 "$(hostname) has ${local_free}G free (under ${MIN_FREE_GB}G); the fixer builds and runs here"
else
	report disk-local 1 ""
fi

# --- llm: the planner endpoint is a single point of failure ----------------
# Every campaign's planner talks to one endpoint, and nothing fails over when it
# goes away: runs simply stall until it returns. The dashboard already reports
# the endpoint each live run is actually using, so probe those exact addresses
# rather than hardcoding a host that a config change would silently invalidate.
if [ -f "$dash" ]; then
	endpoints=$(python3 -c '
import json, sys
try:
    runs = json.load(open(sys.argv[1]))["runs"]
except Exception:
    runs = []
seen = []
for r in runs:
    ep = ((r.get("stats") or {}).get("endpoint") or "").rstrip("/")
    if ep.startswith("http") and ep not in seen:
        seen.append(ep)
print("\n".join(seen))
' "$dash" 2>/dev/null)
	dead=""
	while read -r ep; do
		[ -n "$ep" ] || continue
		curl -fsS -m 15 -o /dev/null "$ep/health" >/dev/null 2>&1 || dead="${dead:+$dead, }$ep"
	done <<<"$endpoints"
	report llm-endpoint 2 "${dead:+planner endpoint unreachable: $dead (runs stall until it returns)}"
fi

# --- fixer: timer enabled and its last tick did not crash -----------------
if [ -n "$FIXER_SERVICE" ]; then
	replicas=$(docker service ls --filter "name=$FIXER_SERVICE" --format '{{.Replicas}}' 2>/dev/null | head -1)
	running=${replicas%%/*} want=${replicas#*/}
	if [ -z "$replicas" ] || [ "$running" != "${want%% *}" ]; then
		report fixer 3 "swarm fixer $FIXER_SERVICE at ${replicas:-no} replicas (docker service ps $FIXER_SERVICE)"
	else
		report fixer 1 ""
	fi
elif ! systemctl --user is-active -q qwagent-triage.timer; then
	report fixer 1 "fixer timer qwagent-triage.timer is not active"
elif [ "$(systemctl --user show -p Result --value qwagent-triage.service)" != success ]; then
	report fixer 3 "fixer tick keeps failing (journalctl --user -u qwagent-triage.service)"
else
	report fixer 1 ""
fi

# --- fixer ladder: paid cap and keys every tier failed on ------------------
if [ -f "$ledger" ]; then
	paid=$(awk -F'\t' -v since=$((now - 86400)) '$4 == "started" && $3 != "opencode" && $1 >= since' "$ledger" | wc -l)
	if [ "$paid" -ge "$PAID_CAP" ]; then
		report paid-cap 1 "paid fixer cap reached ($paid/$PAID_CAP starts in 24h); only qwen runs until it ages out"
	else
		report paid-cap 1 ""
	fi
	spent=""
	if [ -x "$TRIAGE_STATE/qwagent-triage" ]; then
		spent=$("$TRIAGE_STATE/qwagent-triage" ladder --ledger "$ledger" \
			--tiers "${POKEPILOT_TRIAGE_LADDER:-opencode:2,cursor:2,claude:2}" \
			--available opencode,cursor,claude --paid-daily-cap 1000000 2>/dev/null | paste -sd, -)
	fi
	# Spent (every tier failed) or parked (a paid tier's verdict; see the
	# issue comment). Parks lift on their own when the failure recurs.
	report needs-human 1 "${spent:+fixer stopped on triage keys (all tiers failed or verdict parked, see issue comments): $spent}"
fi

# --- merges: a fixer PR stuck open means CI keeps failing ------------------
stuck=$(gh pr list --state open --search 'in:title "[triage:"' --limit 50 --json number,createdAt 2>/dev/null |
	python3 -c '
import json, sys, datetime as d
now = d.datetime.now(d.timezone.utc)
old = [str(p["number"]) for p in json.load(sys.stdin)
       if (now - d.datetime.fromisoformat(p["createdAt"].replace("Z", "+00:00"))).total_seconds() > 12 * 3600]
print(",".join("#" + n for n in old))
' 2>/dev/null)
report stuck-prs 1 "${stuck:+fixer PRs open over 12h (CI failing?): $stuck}"

# --- deploy: Swarm rolled a service back from a crash-looping image -------
# Repeated rollbacks mean merged fixes keep shipping broken images. Freeze only
# the DEPLOY path (pokefarm-pull.timer): runs keep playing on the image that is
# up, so the farm stays busy, and deploys resume by themselves after the freeze.
FREEZE_ROLLBACKS=${POKEPILOT_FREEZE_ROLLBACKS:-3}
FREEZE_SECONDS=${POKEPILOT_FREEZE_SECONDS:-86400}
deploy_note=""
if [ -n "${POKEPILOT_SWARM_MANAGER:-}" ]; then
	ssh_manager() { timeout 60 ssh -o BatchMode=yes -o ConnectTimeout=10 "$POKEPILOT_SWARM_MANAGER" "$@" 2>/dev/null; }
	svc=$(ssh_manager 'for s in $(docker service ls --filter label=com.docker.stack.namespace=pokefarm -q); do docker service inspect "$s" --format "{{.Spec.Name}} {{if .UpdateStatus}}{{.UpdateStatus.State}} {{.UpdateStatus.CompletedAt}}{{end}}"; done')
	rolled=$(awk '$2 ~ /^rollback/ {print $1}' <<<"$svc" | paste -sd, -)
	report rollback 1 "${rolled:+Swarm rolled back a crash-looping image: $rolled (rollout holds until the next merge)}"

	# One event per (service, completion time); the status stays until the next update.
	events="$STATE/rollback-events"
	while read -r name state completed; do
		case "$state" in rollback*) ;; *) continue ;; esac
		grep -qF " $name $completed" "$events" 2>/dev/null || echo "$now $name $completed" >>"$events"
	done <<<"$svc"
	recent=$(awk -v since=$((now - FREEZE_SECONDS)) '$1 >= since' "$events" 2>/dev/null | wc -l)

	frozen_at=$(cat "$STATE/deploy-frozen" 2>/dev/null || echo 0)
	if [ "$frozen_at" -gt 0 ]; then
		deploy_note="deploys frozen since $(date -d "@$frozen_at" +%H:%M) after $FREEZE_ROLLBACKS+ rollbacks; runs unaffected"
		if [ $((now - frozen_at)) -ge "$FREEZE_SECONDS" ] && ssh_manager 'systemctl start pokefarm-pull.timer'; then
			rm -f "$STATE/deploy-frozen"
			deploy_note=""
			notify "▶️ PokePilot: deploys resumed after the ${FREEZE_SECONDS}s freeze"
		fi
	elif [ "$recent" -ge "$FREEZE_ROLLBACKS" ] && ssh_manager 'systemctl stop pokefarm-pull.timer'; then
		echo "$now" >"$STATE/deploy-frozen"
		deploy_note="deploys frozen; runs unaffected"
		notify "🛑 PokePilot: $recent Swarm rollbacks in the last ${FREEZE_SECONDS}s. Deploys are frozen (pokefarm-pull.timer stopped); runs keep going on the current image and deploys resume automatically after the freeze."
	fi
fi

# --- daily digest ----------------------------------------------------------
today=$(date +%F)
if [ "$(date +%H)" -ge "$DIGEST_HOUR" ] && [ "$(cat "$STATE/digest-day" 2>/dev/null)" != "$today" ]; then
	since=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)
	merged=$(gh pr list --state merged --search "merged:>=$since" --limit 100 --json number | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))' 2>/dev/null || echo "?")
	opened=$(gh issue list --state all --search "\"[farm]\" in:title created:>=$since" --limit 200 --json number | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))' 2>/dev/null || echo "?")
	closed=$(gh issue list --state closed --search "\"[farm]\" in:title closed:>=$since" --limit 200 --json number | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))' 2>/dev/null || echo "?")
	runs=$(python3 -c '
import json, sys
prev_path = sys.argv[2]
try:
    prev = json.load(open(prev_path))
except Exception:
    prev = {}
cur = {}
for r in json.load(open(sys.argv[1]))["runs"]:
    if r.get("status") == "done":
        continue
    # player resets to empty while an attempt is requeued; recovery_badges is durable.
    badges = max(len((r.get("player") or {}).get("badges") or []), int(r.get("recovery_badges") or 0))
    goal = (r.get("stats") or {}).get("goal_summary") or r.get("goal") or ""
    rid = r.get("run_id")
    cur[rid] = badges
    # vs the previous digest: a flat badge count for a day is the stuck signal.
    delta = "" if rid not in prev else (" (%+d)" % (badges - prev[rid]) if badges != prev[rid] else " (no change)")
    print("• %s %s: %d badges%s, %s, attempt %s, lost %s" % (
        r.get("game"), r.get("play_style") or "", badges, delta, goal,
        r.get("attempts"), r.get("loss_recoveries") or 0))
json.dump(cur, open(prev_path, "w"))
' "$dash" "$STATE/digest-badges.json" 2>/dev/null || echo "• dashboard unavailable")
	paid_today=$(awk -F'\t' -v since=$((now - 86400)) '$4 == "started" && $3 != "opencode" && $1 >= since' "$ledger" 2>/dev/null | wc -l)
	free_today=$(awk -F'\t' -v since=$((now - 86400)) '$4 == "started" && $3 == "opencode" && $1 >= since' "$ledger" 2>/dev/null | wc -l)
	notify "📊 PokePilot daily
PRs merged: $merged · farm issues opened: $opened, closed: $closed
fixer starts: qwen $free_today, paid $paid_today/$PAID_CAP${deploy_note:+
$deploy_note}
$runs"
	echo "$today" >"$STATE/digest-day"
fi
