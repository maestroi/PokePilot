#!/usr/bin/env bash
# Optional oneshot: pick one unused PokeFarm triage key and let local qwagent
# try a fix in a dedicated worktree. Safe to commit: no hosts beyond the
# public wall default, no tokens.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
POKEPILOT_ROOT=${POKEPILOT_ROOT:-$ROOT}

ENV_FILE=${POKEPILOT_ENV:-$HOME/.config/pokepilot/env}
if [ -f "$ENV_FILE" ]; then
	set -a
	# shellcheck disable=SC1090
	. "$ENV_FILE"
	set +a
fi

POKEPILOT_MCP_URL=${POKEPILOT_MCP_URL:-https://admin.rompilot.app/mcp}
POKEPILOT_TRIAGE_STATE=${POKEPILOT_TRIAGE_STATE:-$HOME/.local/share/pokepilot/qwagent-triage}
POKEPILOT_TRIAGE_TREE=${POKEPILOT_TRIAGE_TREE:-$HOME/Documents/projects/PokePilot-qwagent-triage}
PROMPT=${POKEPILOT_TRIAGE_PROMPT:-$SCRIPT_DIR/qwagent-triage.prompt.md}
POKEPILOT_TRIAGE_AGENT=${POKEPILOT_TRIAGE_AGENT:-ladder}
POKEPILOT_CURSOR_MODEL=${POKEPILOT_CURSOR_MODEL:-}
POKEPILOT_OPENCODE_MODEL=${POKEPILOT_OPENCODE_MODEL:-qwen3.8-27b/qwen3.8-27b}
POKEPILOT_CLAUDE_MODEL=${POKEPILOT_CLAUDE_MODEL:-claude-opus-5-5}
# ladder: free local qwen first, then paid tiers, per triage key. A key that
# spends every tier stops being picked until it opens a PR or the ledger is
# cleared; paid tiers share one rolling 24h start cap.
POKEPILOT_TRIAGE_LADDER=${POKEPILOT_TRIAGE_LADDER:-opencode:2,cursor:2,claude:2}
POKEPILOT_PAID_DAILY_CAP=${POKEPILOT_PAID_DAILY_CAP:-20}
# Replicas (deploy/fixer.yml) keep per-replica state but share the ledger,
# so budgets and the paid cap stay global, and a claims dir, so two replicas
# never work one key. POKEPILOT_QWEN_LOCK serializes the single qwen slot;
# POKEPILOT_QWEN_URL also sees it busy with work outside this fixer.
POKEPILOT_TRIAGE_LEDGER=${POKEPILOT_TRIAGE_LEDGER:-$POKEPILOT_TRIAGE_STATE/ledger.tsv}
POKEPILOT_TRIAGE_CLAIMS=${POKEPILOT_TRIAGE_CLAIMS:-}
POKEPILOT_QWEN_LOCK=${POKEPILOT_QWEN_LOCK:-}
POKEPILOT_QWEN_URL=${POKEPILOT_QWEN_URL:-}
# Local qwen PRs land in 20-31m; runs past that die on context blowups
# ("Compaction summary reached the output token limit", socket closed) after
# burning the full 50m. Cut it loose early so the ladder escalates sooner.
POKEPILOT_OPENCODE_BUDGET=${POKEPILOT_OPENCODE_BUDGET:-35m}
# Whole-attempt kill (prep, agent, PR). The Swarm fixer raises both: with two
# replicas a slow qwen attempt no longer holds up the queue.
POKEPILOT_ATTEMPT_TIMEOUT=${POKEPILOT_ATTEMPT_TIMEOUT:-50m}
export PATH="$HOME/.cursor/bin:$HOME/.opencode/bin:$HOME/go/bin:$HOME/.local/bin:/usr/local/go/bin:$PATH"

DRY_RUN=0
LOCKED=0
PREPARE_ONLY=0
for arg in "$@"; do
	case "$arg" in
	--dry-run) DRY_RUN=1 ;;
	--locked) LOCKED=1 ;;
	--prepare-tree) PREPARE_ONLY=1 ;;
	esac
done

log() { echo "qwagent-triage: $*" >&2; }

cursor_binary() {
	if command -v agent >/dev/null 2>&1; then
		command -v agent
		return 0
	fi
	if command -v cursor-agent >/dev/null 2>&1; then
		command -v cursor-agent
		return 0
	fi
	return 1
}

cursor_authenticated() {
	local bin status
	bin=$(cursor_binary) || return 1
	# `agent status` reports "Not logged in" for an API key that -p accepts.
	[ -n "${CURSOR_API_KEY:-}" ] && return 0
	status=$("$bin" status 2>&1 || true)
	if printf '%s' "$status" | grep -Eiq 'not authenticated|not logged|logged out'; then
		return 1
	fi
	[ -n "$status" ]
}

claude_authenticated() {
	command -v claude >/dev/null 2>&1 || return 1
	[ -n "${ANTHROPIC_API_KEY:-}${CLAUDE_CODE_OAUTH_TOKEN:-}" ] && return 0
	claude auth status 2>/dev/null | grep -q '"loggedIn": *true'
}

# qwen_free runs in the main shell: fd 8 holds the qwen lock until the
# opencode attempt exits. A busy slot drops opencode from this tick's ladder,
# so the next tier (Cursor auto) runs instead of queueing behind it.
qwen_free() {
	command -v opencode >/dev/null 2>&1 || return 1
	if [ -n "$POKEPILOT_QWEN_LOCK" ]; then
		exec 8>"$POKEPILOT_QWEN_LOCK"
		flock -n 8 || { exec 8>&-; return 1; }
	fi
	if [ -n "$POKEPILOT_QWEN_URL" ] &&
		curl -fsS -m 5 "${POKEPILOT_QWEN_URL%/v1}/slots" 2>/dev/null | grep -q '"is_processing":true'; then
		exec 8>&-
		return 1
	fi
}

available_backends() {
	local out=()
	[ "$QWEN_FREE" -eq 1 ] && out+=(opencode)
	cursor_authenticated && out+=(cursor)
	claude_authenticated && out+=(claude)
	local IFS=,
	printf '%s' "${out[*]}"
}

# ladder_args is shared by key selection and the picker's blocked-key list.
ladder_args() {
	printf '%s\n' --ledger "$POKEPILOT_TRIAGE_LEDGER" \
		--tiers "$POKEPILOT_TRIAGE_LADDER" \
		--paid-daily-cap "$POKEPILOT_PAID_DAILY_CAP" \
		--available "$LADDER_AVAILABLE"
}

# ladder_backend KEY [COUNT]: COUNT is the group's occurrence count, which
# lifts a parked verdict once the failure occurs again.
ladder_backend() {
	local args
	mapfile -t args < <(ladder_args)
	"$POKEPILOT_TRIAGE_STATE/qwagent-triage" ladder "${args[@]}" --key "$1" --count "${2:-0}"
}

select_agent_backend() {
	case "$POKEPILOT_TRIAGE_AGENT" in
	ladder)
		if ! ladder_backend "$KEY" "$COUNT"; then
			log "ladder has no backend for $KEY now (tiers spent or paid cap reached)"
			return 1
		fi
		;;
	claude)
		if ! claude_authenticated; then
			log "Claude Code is not installed or not logged in; skip"
			return 1
		fi
		echo claude
		;;
	auto)
		if cursor_authenticated; then
			echo cursor
			return 0
		fi
		if command -v opencode >/dev/null 2>&1; then
			echo opencode
			return 0
		fi
		log "no authenticated Cursor CLI or OpenCode binary found; skip"
		return 1
		;;
	cursor)
		if ! cursor_binary >/dev/null; then
			log "Cursor CLI not installed; install it from cursor.com/cli"
			return 1
		fi
		if ! cursor_authenticated; then
			log "Cursor CLI is not authenticated; run 'agent login' once with your Cursor account"
			return 1
		fi
		echo cursor
		;;
	opencode)
		if ! command -v opencode >/dev/null 2>&1; then
			log "OpenCode is not installed; skip"
			return 1
		fi
		echo opencode
		;;
	*)
		log "unknown POKEPILOT_TRIAGE_AGENT=$POKEPILOT_TRIAGE_AGENT; want ladder, auto, cursor, opencode, or claude"
		return 1
		;;
	esac
}

selected_agent_model() {
	case "$AGENT_BACKEND" in
	cursor/*) printf '%s' "${AGENT_BACKEND#cursor/}" ;;
	cursor)
		if [ -n "$POKEPILOT_CURSOR_MODEL" ]; then
			printf '%s' "$POKEPILOT_CURSOR_MODEL"
		else
			printf '%s' "auto"
		fi
		;;
	opencode) printf '%s' "$POKEPILOT_OPENCODE_MODEL" ;;
	claude) printf '%s' "$POKEPILOT_CLAUDE_MODEL" ;;
	esac
}

record_solver_attempt() {
	local state=$1 note=${2:-} branch_name=${3:-} pr_number=${4:-0} pr_url=${5:-} exit_code=${6:-0}
	# Local ladder ledger: a start without a later "pr" is a failed attempt,
	# including a 50m timeout kill that never reaches a terminal record.
	case "$state" in
	started) printf '%s\t%s\t%s\tstarted\n' "$(date +%s)" "$KEY" "$AGENT_BACKEND" >>"$POKEPILOT_TRIAGE_LEDGER" ;;
	pr_opened | pr_updated) printf '%s\t%s\t%s\tpr\n' "$(date +%s)" "$KEY" "$AGENT_BACKEND" >>"$POKEPILOT_TRIAGE_LEDGER" ;;
	esac
	"$POKEPILOT_TRIAGE_STATE/qwagent-triage" record-attempt \
		--endpoint "$POKEPILOT_MCP_URL" \
		--key "$KEY" \
		--id "$ATTEMPT_ID" \
		--backend "$AGENT_BACKEND" \
		--model "$SOLVER_MODEL" \
		--state "$state" \
		--run-id "$RUN_ID" \
		--branch "$branch_name" \
		--pr-number "$pr_number" \
		--pr-url "$pr_url" \
		--exit-code "$exit_code" \
		--note "$note" \
		--started-at "$ATTEMPT_STARTED_AT" >/dev/null 2>&1 || log "could not record solver attempt $ATTEMPT_ID state=$state"
}

prepare_triage_tree() {
	# Share objects with the local checkout, but fetch and push the
	# primary repo's origin. A tree cloned from the checkout itself only
	# sees that checkout's main, and push never reaches GitHub.
	local upstream
	upstream=$(git -C "$POKEPILOT_ROOT" remote get-url origin 2>/dev/null || true)
	if [ -z "$upstream" ]; then
		upstream=$POKEPILOT_ROOT
	fi
	if [ ! -d "$POKEPILOT_TRIAGE_TREE/.git" ]; then
		mkdir -p "$(dirname "$POKEPILOT_TRIAGE_TREE")"
		if ! git clone --reference "$POKEPILOT_ROOT" "$upstream" "$POKEPILOT_TRIAGE_TREE"; then
			git clone "$upstream" "$POKEPILOT_TRIAGE_TREE"
		fi
	elif [ "$upstream" != "$POKEPILOT_ROOT" ]; then
		git -C "$POKEPILOT_TRIAGE_TREE" remote set-url origin "$upstream"
	fi
	git -C "$POKEPILOT_TRIAGE_TREE" fetch origin
	# A timed-out attempt leaves this dedicated tree dirty. checkout then
	# aborts and the oneshot exits 1 before reset --hard can rebuild main.
	git -C "$POKEPILOT_TRIAGE_TREE" reset --hard HEAD
	git -C "$POKEPILOT_TRIAGE_TREE" clean -fd
	git -C "$POKEPILOT_TRIAGE_TREE" checkout -f main
	git -C "$POKEPILOT_TRIAGE_TREE" reset --hard origin/main
	git -C "$POKEPILOT_TRIAGE_TREE" branch --set-upstream-to=origin/main main
	git -C "$POKEPILOT_TRIAGE_TREE" clean -fd
}

mkdir -p "$POKEPILOT_TRIAGE_STATE"

if [ "$PREPARE_ONLY" -eq 1 ]; then
	prepare_triage_tree
	exit 0
fi

if [ -z "${POKEPILOT_MCP_TOKEN:-}" ]; then
	log "POKEPILOT_MCP_TOKEN unset; skip"
	exit 0
fi

if [ "$DRY_RUN" -eq 0 ] && [ "$LOCKED" -eq 0 ]; then
	exec 9>"$POKEPILOT_TRIAGE_STATE/lock"
	if ! flock -n 9; then
		log "lock held; skip"
		exit 0
	fi
	# No --foreground: the TERM must reach the agent CLI too, or bash defers its
	# TERM trap until the agent finishes by itself (a 50m budget ran 61m and never
	# recorded a terminal attempt). -k hard-kills a hung agent.
	timeout -k 60 "$POKEPILOT_ATTEMPT_TIMEOUT" "$0" --locked "$@"
	exit $?
fi

# cursor_log turns Cursor's stream-json into one log line per assistant
# message and tool call (text mode prints nothing until the agent exits).
cursor_log() {
	python3 -u -c '
import json, sys
for line in sys.stdin:
    try:
        e = json.loads(line)
    except ValueError:
        print(line.rstrip()[:300])
        continue
    t = e.get("type")
    if t == "assistant":
        text = " ".join(c.get("text", "") for c in e["message"]["content"]).strip()
        if text:
            print("cursor: " + text.replace("\n", " ")[:300])
    elif t == "tool_call" and e.get("subtype") == "started":
        name, call = next(iter(e["tool_call"].items()))
        args = call.get("args") or {} if isinstance(call, dict) else {}
        hint = next((str(args[k]) for k in ("command", "path", "pattern", "query", "toolName", "name") if args.get(k)), "")
        print("cursor > " + name.replace("ToolCall", "") + " " + hint.replace("\n", " ")[:200])
    elif t == "result":
        print("cursor: finished in %ss error=%s" % (e.get("duration_ms", 0) // 1000, e.get("is_error")))
'
}

json_field() {
	python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1],"") or "")' "$1"
}

gh_repo() {
	local url
	url=$(git -C "$POKEPILOT_ROOT" remote get-url origin 2>/dev/null || true)
	url=${url%.git}
	case "$url" in
	git@*:*/*)
		echo "${url#*:}"
		;;
	https://github.com/*)
		echo "${url#https://github.com/}"
		;;
	*)
		echo "maestroi/pokepilot"
		;;
	esac
}

CLAIMED_ISSUE_NUMBER=""
KEEP_ISSUE_CLAIM=0

release_issue_claim() {
	local login
	[ -n "${CLAIMED_ISSUE_NUMBER:-}" ] || return 0
	[ "${KEEP_ISSUE_CLAIM:-0}" -eq 0 ] || return 0
	login=$(gh api user --jq .login 2>/dev/null || true)
	[ -n "$login" ] || return 0
	if gh issue edit "$CLAIMED_ISSUE_NUMBER" --repo "$(gh_repo)" --remove-assignee "$login" >/dev/null 2>&1; then
		log "released GitHub issue #$CLAIMED_ISSUE_NUMBER from @$login"
	fi
	CLAIMED_ISSUE_NUMBER=""
}

claim_issue() {
	local issue=$1 login assignees
	[ -n "$issue" ] || return 0
	login=$(gh api user --jq .login 2>/dev/null || true)
	if [ -z "$login" ]; then
		log "cannot determine authenticated GitHub user; refusing an invisible issue claim"
		return 1
	fi
	assignees=$(gh issue view "$issue" --repo "$(gh_repo)" --json assignees --jq '.assignees[].login' 2>/dev/null || true)
	if [ -n "$assignees" ]; then
		log "GitHub issue #$issue is already assigned ($(printf '%s' "$assignees" | paste -sd, -)); another agent owns it"
		return 2
	fi
	if ! gh issue edit "$issue" --repo "$(gh_repo)" --add-assignee "$login" >/dev/null; then
		log "failed to claim GitHub issue #$issue as @$login"
		return 1
	fi
	CLAIMED_ISSUE_NUMBER=$issue
	log "claimed GitHub issue #$issue as @$login"
	return 0
}

CLAIM_DIR=""
trap 'release_issue_claim; [ -z "$CLAIM_DIR" ] || rm -rf "$CLAIM_DIR"' EXIT
trap 'exit 130' INT
on_term() {
	if [ -n "${ATTEMPT_ID:-}" ]; then
		record_solver_attempt agent_failed "killed by the $POKEPILOT_ATTEMPT_TIMEOUT attempt timeout" "" 0 "" 124
	fi
	exit 143
}
trap on_term TERM

triage_bin() {
	local bin="$POKEPILOT_TRIAGE_STATE/qwagent-triage"
	# go run rewrites a child exit 2 into its own exit 1; build so idle stays 2.
	(cd "$POKEPILOT_ROOT" && go build -o "$bin" ./cmd/qwagent-triage)
	printf '%s' "$bin"
}

seed_rom() {
	local src=""
	if [ -n "${POKEMON_RED_ROM:-}" ] && [ -f "$POKEMON_RED_ROM" ]; then
		src=$POKEMON_RED_ROM
	elif [ -f "$HOME/.config/pokepilot/pokemon_red.gb" ]; then
		src=$HOME/.config/pokepilot/pokemon_red.gb
	elif [ -f "$POKEPILOT_ROOT/roms/pokemon_red.gb" ]; then
		src=$POKEPILOT_ROOT/roms/pokemon_red.gb
	else
		log "no pokemon_red.gb found for the worktree"
		return 0
	fi
	mkdir -p "$POKEPILOT_TRIAGE_TREE/roms"
	ln -sfn "$src" "$POKEPILOT_TRIAGE_TREE/roms/pokemon_red.gb"
	# Blue/Yellow/Gen 2 failures replay only when their cartridge sits in the
	# same ROM dir (POKEPILOT_ROM_DIR follows POKEMON_RED_ROM). Link siblings.
	local rom
	for rom in "$(dirname "$src")"/*.gb "$(dirname "$src")"/*.gbc; do
		[ -f "$rom" ] && [ "$rom" != "$src" ] && ln -sfn "$rom" "$POKEPILOT_TRIAGE_TREE/roms/$(basename "$rom")"
	done
	if ! grep -qxF 'roms/pokemon_red.gb' "$POKEPILOT_TRIAGE_TREE/.git/info/exclude" 2>/dev/null; then
		printf '%s\n' 'roms/pokemon_red.gb' >>"$POKEPILOT_TRIAGE_TREE/.git/info/exclude"
	fi
	export POKEMON_RED_ROM="$POKEPILOT_TRIAGE_TREE/roms/pokemon_red.gb"
}

pick_next() {
	local triage titles merged_prs pick_args status bin
	local triage_file merged_file candidates ancestry_ready
	bin=$(triage_bin)
	if ! triage=$("$bin" fetch-triage --endpoint "$POKEPILOT_MCP_URL"); then
		log "MCP triage unreachable; skip"
		return 2
	fi
	open_prs=$(gh pr list --repo "$(gh_repo)" --state open --limit 100 --json number,title,headRefName,url,statusCheckRollup,mergeable 2>/dev/null || printf '[]')
	own_err=$(mktemp)
	set +e
	own_pr=$(printf '%s' "$open_prs" | "$bin" pick-own-pr 2>"$own_err")
	own_status=$?
	set -e
	if [ "$own_status" -eq 0 ] && [ "$POKEPILOT_TRIAGE_AGENT" = ladder ] &&
		! ladder_backend "$(printf '%s' "$own_pr" | json_field key)" >/dev/null 2>&1; then
		log "own PR repair key is spent or paid-capped; picking a fresh failure"
		own_status=2
	fi
	if [ "$own_status" -eq 0 ] && [ -n "$POKEPILOT_TRIAGE_CLAIMS" ] &&
		[ -d "$POKEPILOT_TRIAGE_CLAIMS/$(printf '%s' "$own_pr" | json_field key)" ]; then
		log "own PR repair is held by another replica; picking a fresh failure"
		own_status=2
	fi
	if [ "$own_status" -eq 0 ]; then
		rm -f "$own_err"
		printf '%s' "$own_pr"
		return 0
	fi
	if [ "$own_status" -ne 2 ]; then
		log "own-pr picker failed (exit $own_status): $(tr '\n' ' ' <"$own_err")"
	fi
	rm -f "$own_err"

	titles=$(gh pr list --repo "$(gh_repo)" --state open --limit 100 --json title --jq '.[].title' 2>/dev/null || true)
	assigned_issues=$(gh issue list --repo "$(gh_repo)" --state open --limit 200 --json body,assignees 2>/dev/null || printf '[]')
	merged_prs=$(gh pr list --repo "$(gh_repo)" --state closed --limit 200 --json title,mergedAt,mergeCommit 2>/dev/null || printf '[]')
	pick_args=(pick)
	if [ -n "$POKEPILOT_TRIAGE_CLAIMS" ]; then
		# A claim outlives its attempt only when the replica was killed.
		# ponytail: fixed 3h, above any sane POKEPILOT_ATTEMPT_TIMEOUT.
		find "$POKEPILOT_TRIAGE_CLAIMS" -mindepth 1 -maxdepth 1 -mmin +180 -exec rm -rf {} + 2>/dev/null || true
		for claim in "$POKEPILOT_TRIAGE_CLAIMS"/*; do
			[ -d "$claim" ] && pick_args+=(--claimed "[triage:$(basename "$claim")]")
		done
	fi
	if [ "$POKEPILOT_TRIAGE_AGENT" = ladder ]; then
		local ladder_list ladder_triage
		mapfile -t ladder_list < <(ladder_args)
		ladder_triage=$(mktemp)
		printf '%s' "$triage" >"$ladder_triage"
		while IFS= read -r key; do
			[ -n "$key" ] && pick_args+=(--claimed "[triage:$key]")
		done < <("$bin" ladder "${ladder_list[@]}" --triage "$ladder_triage")
		rm -f "$ladder_triage"
	fi
	while IFS= read -r title; do
		[ -z "$title" ] && continue
		pick_args+=(--claimed "$title")
	done <<<"$titles"
	while IFS= read -r key; do
		[ -z "$key" ] && continue
		# Reuse the picker's existing stable marker parser: an assigned generated
		# farm issue is an earlier claim than an eventual [triage:key] PR.
		pick_args+=(--claimed "[triage:$key]")
	done < <(printf '%s' "$assigned_issues" | python3 -c '
import json
import sys

tick = chr(96)
for issue in json.load(sys.stdin):
    if not issue.get("assignees"):
        continue
    for line in (issue.get("body") or "").splitlines():
        if not line.startswith("- **Triage key:**"):
            continue
        parts = line.split(tick)
        if len(parts) >= 3 and parts[1].strip():
            print(parts[1].strip())
            break
')

	# A merged [triage:key] PR suppresses that key unless the fingerprint's
	# last_observed_revision contains the merge. The run's latest finish is
	# the wrong revision: a later attempt can fail differently on a newer build.
	triage_file=$(mktemp)
	merged_file=$(mktemp)
	printf '%s' "$triage" >"$triage_file"
	printf '%s' "$merged_prs" >"$merged_file"
	repairs=$(python3 - "$triage_file" "$merged_file" <<'PY'
import json
import re
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    groups = {str(g.get("key") or ""): g for g in json.load(f)}
with open(sys.argv[2], encoding="utf-8") as f:
    prs = json.load(f)

marker = re.compile(r"\[triage:([^\]]+)\]")
latest = {}
for pr in prs:
    merged_at = pr.get("mergedAt") or ""
    if not merged_at:
        continue
    match = marker.search(pr.get("title") or "")
    if not match:
        continue
    key = match.group(1).strip()
    group = groups.get(key)
    if group is None:
        continue
    merge_sha = ((pr.get("mergeCommit") or {}).get("oid") or "")
    issue = group.get("issue") or {}
    observed = str(issue.get("last_observed_revision") or "").strip()
    previous = latest.get(key)
    if previous is None or merged_at > previous[0]:
        latest[key] = (merged_at, merge_sha, observed)

rows = []
for key, (_, merge_sha, observed) in latest.items():
    fixed = str((groups[key].get("issue") or {}).get("fixed_revision") or "").strip()
    rows.append({"key": key, "merge_sha": merge_sha, "observed_revision": observed, "fixed_revision": fixed})
json.dump(rows, sys.stdout)
PY
)
	rm -f "$triage_file" "$merged_file"

	ancestry_ready=0
	if [ "$repairs" != "[]" ] && [ -n "$repairs" ]; then
		if git -C "$POKEPILOT_ROOT" fetch --quiet origin; then
			ancestry_ready=1
		else
			log "git fetch failed; merged triage repairs remain suppressed this tick"
		fi
	fi

	if [ "$ancestry_ready" -eq 1 ]; then
		set +e
		class_json=$(printf '%s' "$repairs" | "$bin" classify-repairs --repo "$POKEPILOT_ROOT")
		class_status=$?
		set -e
		if [ "$class_status" -ne 0 ]; then
			log "classify-repairs failed; suppress merged repairs"
			ancestry_ready=0
		fi
	fi
	if [ "$ancestry_ready" -eq 1 ]; then
		classified=$(printf '%s' "$class_json" | python3 -c '
import json, sys
data = json.load(sys.stdin)
for key in data.get("repaired") or []:
    print("repaired\t" + str(key))
for key in data.get("regressed") or []:
    print("regressed\t" + str(key))
')
	else
		classified=$(printf '%s' "$repairs" | python3 -c '
import json, sys
for row in json.load(sys.stdin):
    key = str(row.get("key") or "").strip()
    if key:
        print("repaired\t" + key)
')
	fi
	while IFS=$'\t' read -r kind key; do
		[ -z "$key" ] && continue
		case "$kind" in
		repaired) pick_args+=(--repaired "$key") ;;
		regressed) pick_args+=(--regressed "$key") ;;
		esac
	done <<<"$classified"

	set +e
	PICK_JSON=$(printf '%s' "$triage" | "$bin" "${pick_args[@]}")
	status=$?
	set -e
	if [ "$status" -eq 2 ]; then
		log "no actionable unclaimed group; idle"
		return 2
	fi
	if [ "$status" -ne 0 ]; then
		log "picker failed (exit $status); skip"
		return 2
	fi
	printf '%s' "$PICK_JSON"
}

# Probed once per tick: cursor/claude auth checks are slow CLI calls.
# Picking assumes qwen is usable; qwen_free takes the real lock only once
# this replica owns a key. Locking during the pick made a replica that was
# only picking (or then lost the claim) push the other one off qwen.
QWEN_FREE=0
if [ "$POKEPILOT_TRIAGE_AGENT" = ladder ] && command -v opencode >/dev/null 2>&1; then
	QWEN_FREE=1
fi
LADDER_AVAILABLE=$(available_backends)
if ! PICK_JSON=$(pick_next); then
	exit 0
fi

KEY=$(printf '%s' "$PICK_JSON" | json_field key)
RUN_ID=$(printf '%s' "$PICK_JSON" | json_field run_id)
EXAMPLE=$(printf '%s' "$PICK_JSON" | json_field example)
MODE=$(printf '%s' "$PICK_JSON" | json_field mode)
HEAD_REF=$(printf '%s' "$PICK_JSON" | json_field head_ref)
PR_NUMBER=$(printf '%s' "$PICK_JSON" | json_field pr_number)
PR_URL=$(printf '%s' "$PICK_JSON" | json_field pr_url)
ISSUE_NUMBER=$(printf '%s' "$PICK_JSON" | json_field issue_number)
COUNT=$(printf '%s' "$PICK_JSON" | json_field count)
if [ -z "$KEY" ]; then
	log "picker returned empty key; skip"
	exit 0
fi

# mkdir is the atomic cross-replica claim; the loser idles one tick.
if [ "$DRY_RUN" -eq 0 ] && [ -n "$POKEPILOT_TRIAGE_CLAIMS" ]; then
	mkdir -p "$POKEPILOT_TRIAGE_CLAIMS"
	if ! mkdir "$POKEPILOT_TRIAGE_CLAIMS/$KEY" 2>/dev/null; then
		log "another fixer replica holds $KEY; idle"
		exit 0
	fi
	CLAIM_DIR="$POKEPILOT_TRIAGE_CLAIMS/$KEY"
fi

if [ "$QWEN_FREE" -eq 1 ] && ! qwen_free; then
	QWEN_FREE=0
	LADDER_AVAILABLE=$(available_backends)
	log "qwen busy; ladder skips opencode for $KEY this tick"
fi
if ! AGENT_BACKEND=$(select_agent_backend); then
	exit 0
fi
[ "$AGENT_BACKEND" = opencode ] || exec 8>&-


if [ "$DRY_RUN" -eq 1 ]; then
	printf '%s\n' "$PICK_JSON"
	if [ "$MODE" = "repair_pr" ]; then
		echo "would repair PR #$PR_NUMBER on $HEAD_REF"
	elif [ -n "$ISSUE_NUMBER" ]; then
		echo "would claim GitHub issue #$ISSUE_NUMBER for $KEY (run $RUN_ID)"
	else
		echo "would claim $KEY (run $RUN_ID; no generated GitHub issue)"
	fi
	SOLVER_MODEL=$(selected_agent_model)
	case "$AGENT_BACKEND" in
	cursor | cursor/*) echo "would run: Cursor CLI model=$SOLVER_MODEL in $POKEPILOT_TRIAGE_TREE" ;;
	opencode) echo "would run: OpenCode model=$SOLVER_MODEL in $POKEPILOT_TRIAGE_TREE" ;;
	claude) echo "would run: Claude Code model=$SOLVER_MODEL in $POKEPILOT_TRIAGE_TREE" ;;
	esac
	exit 0
fi

if [ "$MODE" != "repair_pr" ] && [ -n "$ISSUE_NUMBER" ]; then
	set +e
	claim_issue "$ISSUE_NUMBER"
	claim_status=$?
	set -e
	if [ "$claim_status" -eq 2 ]; then
		# Another agent won the GitHub-visible claim after selection. Leave this
		# tick idle rather than racing it; the next timer tick will pick again.
		exit 0
	fi
	if [ "$claim_status" -ne 0 ]; then
		log "could not establish a GitHub-visible claim; skip"
		exit 0
	fi
fi

if [ "$MODE" = "repair_pr" ]; then
	log "repairing PR #$PR_NUMBER ($HEAD_REF) for $KEY: $EXAMPLE"
else
	log "claiming $KEY ($EXAMPLE) run=$RUN_ID"
fi
# Investigate is a best-effort claim. Auto-filed issues are often already
# investigating, and the orchestrator then 409s (the wall currently maps
# that to 502). An open PR is the durable skip; do not abort the local agent.
set +e
invest_out=$("$POKEPILOT_TRIAGE_STATE/qwagent-triage" investigate --endpoint "$POKEPILOT_MCP_URL" --key "$KEY" 2>&1)
invest_status=$?
set -e
if [ "$invest_status" -ne 0 ]; then
	log "investigate failed: $(printf '%s' "$invest_out" | tr '\n' ' '); continuing locally"
fi

prepare_triage_tree
seed_rom
if [ "$MODE" = "repair_pr" ]; then
	if [ -z "$HEAD_REF" ]; then
		log "repair packet missing head_ref; skip"
		exit 0
	fi
	if ! git -C "$POKEPILOT_TRIAGE_TREE" fetch origin "$HEAD_REF"; then
		log "cannot fetch $HEAD_REF; skip"
		exit 0
	fi
	git -C "$POKEPILOT_TRIAGE_TREE" checkout -f -B "$HEAD_REF" "origin/$HEAD_REF"
fi

printf '%s\n' "$PICK_JSON" >"$POKEPILOT_TRIAGE_STATE/packet.json"
DEBUG_PACKET="$POKEPILOT_TRIAGE_STATE/debug-packet.json"
rm -f "$DEBUG_PACKET"
if [ "$MODE" != "repair_pr" ] && [ -n "$RUN_ID" ]; then
	set +e
	(
		cd "$POKEPILOT_TRIAGE_TREE"
		make -s debug RUN="$RUN_ID" DEBUG_MODE=tiny KEY="$KEY"
	) >"$DEBUG_PACKET"
	debug_status=$?
	set -e
	if [ "$debug_status" -eq 0 ]; then
		log "prepared compact debug packet for $RUN_ID"
	else
		log "compact debug preparation failed (exit $debug_status); agent may use the escalation path"
		rm -f "$DEBUG_PACKET"
	fi
fi
{
	cat "$PROMPT"
	printf '\n\n## Packet\n\n```json\n'
	cat "$POKEPILOT_TRIAGE_STATE/packet.json"
	printf '```\n'
	if [ -f "$DEBUG_PACKET" ]; then
		printf '\n## Prepared debug packet\n\nUse this directly; do not repeat its MCP/artifact/source-discovery work.\n\n```json\n'
		cat "$DEBUG_PACKET"
		printf '```\n'
	fi
} >"$POKEPILOT_TRIAGE_STATE/packet.md"

ATTEMPT_STARTED_AT=$(date +%s)
ATTEMPT_ID="${KEY}-${ATTEMPT_STARTED_AT}-${BASHPID}"
SOLVER_MODEL=$(selected_agent_model)
VERDICT_FILE="$POKEPILOT_TRIAGE_TREE/.pokepilot-verdict.json"
rm -f "$VERDICT_FILE"
if ! grep -qxF '.pokepilot-verdict.json' "$POKEPILOT_TRIAGE_TREE/.git/info/exclude" 2>/dev/null; then
	printf '%s\n' '.pokepilot-verdict.json' >>"$POKEPILOT_TRIAGE_TREE/.git/info/exclude"
fi
record_solver_attempt started "coding agent launched"

set +e
case "$AGENT_BACKEND" in
cursor | cursor/*)
	CURSOR_BIN=$(cursor_binary)
	cursor_args=(-p --force --trust --approve-mcps
		--workspace "$POKEPILOT_TRIAGE_TREE"
		--output-format stream-json
		--model "$SOLVER_MODEL")
	cursor_packet="$POKEPILOT_TRIAGE_TREE/.pokepilot-triage-packet.md"
	if ! grep -qxF '.pokepilot-triage-packet.md' "$POKEPILOT_TRIAGE_TREE/.git/info/exclude" 2>/dev/null; then
		printf '%s\n' '.pokepilot-triage-packet.md' >>"$POKEPILOT_TRIAGE_TREE/.git/info/exclude"
	fi
	cp "$POKEPILOT_TRIAGE_STATE/packet.md" "$cursor_packet"
	"$CURSOR_BIN" "${cursor_args[@]}" \
		"Read @.pokepilot-triage-packet.md and follow it exactly. Do not pick a different failure." | cursor_log
	agent_status=${PIPESTATUS[0]}
	rm -f "$cursor_packet"
	;;
claude)
	claude_packet="$POKEPILOT_TRIAGE_TREE/.pokepilot-triage-packet.md"
	if ! grep -qxF '.pokepilot-triage-packet.md' "$POKEPILOT_TRIAGE_TREE/.git/info/exclude" 2>/dev/null; then
		printf '%s\n' '.pokepilot-triage-packet.md' >>"$POKEPILOT_TRIAGE_TREE/.git/info/exclude"
	fi
	cp "$POKEPILOT_TRIAGE_STATE/packet.md" "$claude_packet"
	# IS_SANDBOX lets skip-permissions run as root inside the fixer container.
	(cd "$POKEPILOT_TRIAGE_TREE" && IS_SANDBOX=1 claude -p \
		--model "$POKEPILOT_CLAUDE_MODEL" \
		--dangerously-skip-permissions \
		--output-format text \
		"Read .pokepilot-triage-packet.md and follow it exactly. Do not pick a different failure.")
	agent_status=$?
	rm -f "$claude_packet"
	;;
opencode)
	# opencode v2 dropped `run --dir`: the session works in the current
	# directory, and --standalone keeps a shared background server (rooted
	# elsewhere) from owning the session. 1.x (the fixer image) has neither a
	# background server nor the flag.
	opencode_args=()
	opencode run --help 2>&1 | grep -q -- --standalone && opencode_args+=(--standalone)
	(cd "$POKEPILOT_TRIAGE_TREE" && timeout -k 30 "$POKEPILOT_OPENCODE_BUDGET" opencode run --auto "${opencode_args[@]}" --model "$POKEPILOT_OPENCODE_MODEL" \
		--title "farm triage ${KEY}" \
		--file "$POKEPILOT_TRIAGE_STATE/packet.md" \
		-- \
		"Follow the attached farm triage packet. Do not pick a different failure.")
	agent_status=$?
	;;
esac
set -e
if [ "$agent_status" -ne 0 ]; then
	record_solver_attempt agent_failed "$AGENT_BACKEND exited before producing a usable PR" "" 0 "" "$agent_status"
	log "$AGENT_BACKEND exited $agent_status; no PR"
	exit 0
fi

# The prompt has the agent push and `gh pr create` itself, often ending back
# on main. Judge the attempt by the PR it opened, not by the branch it left.
agent_opened_pr() {
	gh pr list --repo "$(gh_repo)" --state open --limit 100 --json number,url,headRefName,title \
		--jq "[.[] | select(.title | contains(\"[triage:${KEY}]\"))][0] // empty" 2>/dev/null || true
}

branch=$(git -C "$POKEPILOT_TRIAGE_TREE" rev-parse --abbrev-ref HEAD)
if [ "$MODE" != "repair_pr" ]; then
	agent_pr=$(agent_opened_pr)
	if [ -n "$agent_pr" ]; then
		record_solver_attempt pr_opened "agent opened the triage PR itself" "$(printf '%s' "$agent_pr" | json_field headRefName)" \
			"$(printf '%s' "$agent_pr" | json_field number)" "$(printf '%s' "$agent_pr" | json_field url)"
		KEEP_ISSUE_CLAIM=1
		log "PR already open for $KEY"
		exit 0
	fi
fi
if [ "$MODE" = "repair_pr" ]; then
	if [ "$branch" != "$HEAD_REF" ]; then
		record_solver_attempt no_pr "repair left the expected PR branch" "$branch"
		log "repair left branch $branch; want $HEAD_REF"
		exit 0
	fi
	if ! git -C "$POKEPILOT_TRIAGE_TREE" rev-parse --verify "origin/$HEAD_REF" >/dev/null 2>&1; then
		record_solver_attempt no_pr "repair branch was not pushed" "$HEAD_REF"
		log "repair branch $HEAD_REF not pushed"
		exit 0
	fi
	record_solver_attempt pr_updated "updated an existing triage PR after failed checks" "$HEAD_REF" "${PR_NUMBER:-0}" "$PR_URL"
	log "updated PR #$PR_NUMBER for $KEY"
	exit 0
fi
case "$branch" in
fix/*) ;;
*)
	# A structured "not shipping" verdict. From a paid tier it parks the key
	# until the failure occurs again, so no tier redoes the same conclusion.
	# The free tier's verdict escalates once for a paid confirmation.
	verdict="" reason=""
	if [ "$MODE" != repair_pr ] && [ -f "$VERDICT_FILE" ]; then
		verdict=$(json_field verdict <"$VERDICT_FILE" 2>/dev/null || true)
		reason=$(json_field reason <"$VERDICT_FILE" 2>/dev/null || true)
	fi
	case "$verdict" in
	already_fixed | cannot_reproduce | not_a_defect | needs_human)
		if [ -n "$ISSUE_NUMBER" ]; then
			gh issue comment "$ISSUE_NUMBER" --repo "$(gh_repo)" --body "Fixer verdict from ${AGENT_BACKEND} (\`${SOLVER_MODEL}\`) on run \`${RUN_ID}\`: **${verdict}**

${reason}" >/dev/null 2>&1 || log "could not comment verdict on #$ISSUE_NUMBER"
		fi
		if [ "$AGENT_BACKEND" != opencode ]; then
			printf '%s\t%s\t%s\tparked\t%s\n' "$(date +%s)" "$KEY" "$AGENT_BACKEND" "${COUNT:-0}" >>"$POKEPILOT_TRIAGE_LEDGER"
			record_solver_attempt parked "$verdict: $reason" "$branch"
			log "parked $KEY ($verdict) until it occurs again"
		else
			record_solver_attempt no_pr "free-tier verdict $verdict; escalating for confirmation" "$branch"
			log "free-tier verdict $verdict on $KEY; next tier confirms"
		fi
		;;
	*)
		record_solver_attempt no_pr "agent did not leave a fix/* branch" "$branch"
		log "agent left branch $branch; refuse PR"
		;;
	esac
	exit 0
	;;
esac

# main merges every few minutes, so a fix that took an hour is usually behind
# it. Refusing threw away verified fixes; open the PR and let CI and the
# merge-conflict repair path (pick-own-pr) deal with the drift.
git -C "$POKEPILOT_TRIAGE_TREE" fetch -q origin main || true
if ! git -C "$POKEPILOT_TRIAGE_TREE" merge-base --is-ancestor origin/main HEAD; then
	log "$branch is behind origin/main; opening the PR anyway"
fi

bad=$(git -C "$POKEPILOT_TRIAGE_TREE" diff --name-only origin/main...HEAD | grep -E '\.(state|gb|sav)$|^skill/zz_.*_test\.go$' || true)
if [ -n "$bad" ]; then
	record_solver_attempt no_pr "agent committed forbidden ROM/state/scratch paths" "$branch"
	log "forbidden paths in commit; refuse PR:"
	printf '%s\n' "$bad" >&2
	exit 0
fi

marker="[triage:${KEY}]"
if gh pr list --repo "$(gh_repo)" --state open --head "$branch" --json title --jq '.[].title' | grep -F -q "$marker"; then
	pr_json=$(gh pr list --repo "$(gh_repo)" --state open --head "$branch" --limit 1 --json number,url --jq '.[0]' 2>/dev/null || printf '{}')
	existing_pr_number=$(printf '%s' "$pr_json" | json_field number)
	existing_pr_url=$(printf '%s' "$pr_json" | json_field url)
	record_solver_attempt pr_opened "PR already existed for the produced branch" "$branch" "${existing_pr_number:-0}" "$existing_pr_url"
	KEEP_ISSUE_CLAIM=1
	log "PR already open for $KEY"
	exit 0
fi
if ! git -C "$POKEPILOT_TRIAGE_TREE" rev-parse --verify "origin/$branch" >/dev/null 2>&1; then
	record_solver_attempt no_pr "agent did not push the fix branch" "$branch"
	log "branch $branch not pushed; refuse PR"
	exit 0
fi

farm_issue_marker=""
if [ -n "$ISSUE_NUMBER" ]; then
	farm_issue_marker=" [farm-issue:${ISSUE_NUMBER}]"
fi
set +e
created_pr_url=$(gh pr create --repo "$(gh_repo)" --head "$branch" \
	--title "fix(farm): ${EXAMPLE} ${marker}" \
	--body "Unattended ${AGENT_BACKEND} repair attempt for run \`${RUN_ID}\` (${marker}).${farm_issue_marker} Solver: \`${SOLVER_MODEL}\`. Solver attempt: \`${ATTEMPT_ID}\`.")
pr_status=$?
set -e
if [ "$pr_status" -ne 0 ]; then
	record_solver_attempt no_pr "gh pr create failed" "$branch" 0 "" "$pr_status"
	log "gh pr create failed for $KEY"
	exit 0
fi
created_pr_number=$(gh pr view "$created_pr_url" --repo "$(gh_repo)" --json number --jq '.number' 2>/dev/null || true)
record_solver_attempt pr_opened "opened a triage repair PR" "$branch" "${created_pr_number:-0}" "$created_pr_url"
KEEP_ISSUE_CLAIM=1
log "opened PR for $KEY with $AGENT_BACKEND model=$SOLVER_MODEL"
