#!/usr/bin/env bash
# Unattended farm watchdog: checks the farm, fixer, and deploy health and
# messages a Telegram chat when a check turns bad, every 12h while it stays
# bad, and when it recovers. Sends one digest per day. State lives in
# $POKEPILOT_WATCH_STATE; a check that never changes never re-notifies.
#
# Required in ~/.config/pokepilot/env: TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID,
# POKEPILOT_WALL_URL (http://<swarm-node>:18080). Optional:
# POKEPILOT_SWARM_MANAGER (ssh target) enables the rollback check.
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

# --- fixer: timer enabled and its last tick did not crash -----------------
if ! systemctl --user is-active -q qwagent-triage.timer; then
	report fixer 1 "fixer timer qwagent-triage.timer is not active"
elif [ "$(systemctl --user show -p Result --value qwagent-triage.service)" != success ]; then
	report fixer 3 "fixer tick keeps failing (journalctl --user -u qwagent-triage.service)"
else
	report fixer 1 ""
fi

# --- fixer ladder: paid cap and keys every tier failed on ------------------
ledger="$TRIAGE_STATE/ledger.tsv"
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
if [ -n "${POKEPILOT_SWARM_MANAGER:-}" ]; then
	rolled=$(timeout 60 ssh -o BatchMode=yes -o ConnectTimeout=10 "$POKEPILOT_SWARM_MANAGER" \
		'for s in $(docker service ls --filter label=com.docker.stack.namespace=pokefarm -q); do docker service inspect "$s" --format "{{.Spec.Name}} {{if .UpdateStatus}}{{.UpdateStatus.State}}{{end}}"; done' 2>/dev/null |
		awk '$2 ~ /^rollback/ {print $1}' | paste -sd, -)
	report rollback 1 "${rolled:+Swarm rolled back a crash-looping image: $rolled (rollout holds until the next merge)}"
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
fixer starts: qwen $free_today, paid $paid_today/$PAID_CAP
$runs"
	echo "$today" >"$STATE/digest-day"
fi
