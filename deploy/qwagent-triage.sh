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
POKEPILOT_TRIAGE_AGENT=${POKEPILOT_TRIAGE_AGENT:-auto}
POKEPILOT_CURSOR_MODEL=${POKEPILOT_CURSOR_MODEL:-}
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
	status=$("$bin" status 2>&1 || true)
	if printf '%s' "$status" | grep -Eiq 'not authenticated|not logged|logged out'; then
		return 1
	fi
	[ -n "$status" ]
}

select_agent_backend() {
	case "$POKEPILOT_TRIAGE_AGENT" in
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
		log "unknown POKEPILOT_TRIAGE_AGENT=$POKEPILOT_TRIAGE_AGENT; want auto, cursor, or opencode"
		return 1
		;;
	esac
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
	timeout --foreground 50m "$0" --locked "$@"
	exit $?
fi

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
	titles=$(gh pr list --repo "$(gh_repo)" --state open --limit 100 --json title --jq '.[].title' 2>/dev/null || true)
	merged_prs=$(gh pr list --repo "$(gh_repo)" --state closed --limit 200 --json title,mergedAt,mergeCommit 2>/dev/null || printf '[]')
	pick_args=(pick)
	while IFS= read -r title; do
		[ -z "$title" ] && continue
		pick_args+=(--claimed "$title")
	done <<<"$titles"

	# A merged [triage:key] PR is the local fallback for Orchestrator issue
	# state. Compare its merge commit with the build that produced the newest
	# representative failure. Old-build failures stay suppressed; a failure
	# from a build containing the repair is a concrete regression.
	triage_file=$(mktemp)
	merged_file=$(mktemp)
	printf '%s' "$triage" >"$triage_file"
	printf '%s' "$merged_prs" >"$merged_file"
	candidates=$(python3 - "$triage_file" "$merged_file" <<'PY'
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
    if key not in groups:
        continue
    merge_commit = pr.get("mergeCommit") or {}
    merge_sha = merge_commit.get("oid") or ""
    previous = latest.get(key)
    if previous is None or merged_at > previous[0]:
        latest[key] = (merged_at, merge_sha)

for key, (_, merge_sha) in latest.items():
    run_ids = groups[key].get("run_ids") or []
    run_id = str(run_ids[0]) if run_ids else ""
    print(f"{key}\t{run_id}\t{merge_sha}")
PY
)
	rm -f "$triage_file" "$merged_file"

	ancestry_ready=0
	if [ -n "$candidates" ]; then
		if git -C "$POKEPILOT_ROOT" fetch --quiet origin; then
			ancestry_ready=1
		else
			log "git fetch failed; merged triage repairs remain suppressed this tick"
		fi
	fi

	while IFS=$'\t' read -r key run_id merge_sha; do
		[ -z "$key" ] && continue
		if [ "$ancestry_ready" -ne 1 ] || [ -z "$run_id" ] || [ -z "$merge_sha" ]; then
			pick_args+=(--repaired "$key")
			continue
		fi
		local debug runner_version
		if ! debug=$("$bin" fetch-debug --endpoint "$POKEPILOT_MCP_URL" --run-id "$run_id"); then
			log "cannot read representative run $run_id for $key; keep merged repair suppressed"
			pick_args+=(--repaired "$key")
			continue
		fi
		runner_version=$(printf '%s' "$debug" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(((d.get("finish") or {}).get("runner_version") or "").strip())')
		if [ -z "$runner_version" ] || \
			! git -C "$POKEPILOT_ROOT" cat-file -e "${merge_sha}^{commit}" 2>/dev/null || \
			! git -C "$POKEPILOT_ROOT" cat-file -e "${runner_version}^{commit}" 2>/dev/null; then
			pick_args+=(--repaired "$key")
			continue
		fi
		if git -C "$POKEPILOT_ROOT" merge-base --is-ancestor "$merge_sha" "$runner_version"; then
			pick_args+=(--regressed "$key")
		else
			pick_args+=(--repaired "$key")
		fi
	done <<<"$candidates"

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

if ! PICK_JSON=$(pick_next); then
	exit 0
fi

KEY=$(printf '%s' "$PICK_JSON" | json_field key)
RUN_ID=$(printf '%s' "$PICK_JSON" | json_field run_id)
EXAMPLE=$(printf '%s' "$PICK_JSON" | json_field example)
if [ -z "$KEY" ]; then
	log "picker returned empty key; skip"
	exit 0
fi

if ! AGENT_BACKEND=$(select_agent_backend); then
	exit 0
fi

if [ "$DRY_RUN" -eq 1 ]; then
	printf '%s\n' "$PICK_JSON"
	echo "would claim $KEY (run $RUN_ID)"
	case "$AGENT_BACKEND" in
	cursor) echo "would run: Cursor CLI headless in $POKEPILOT_TRIAGE_TREE" ;;
	opencode) echo "would run: OpenCode qwagent in $POKEPILOT_TRIAGE_TREE" ;;
	esac
	exit 0
fi

log "claiming $KEY ($EXAMPLE) run=$RUN_ID"
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

printf '%s\n' "$PICK_JSON" >"$POKEPILOT_TRIAGE_STATE/packet.json"
{
	cat "$PROMPT"
	printf '\n\n## Packet\n\n```json\n'
	cat "$POKEPILOT_TRIAGE_STATE/packet.json"
	printf '```\n'
} >"$POKEPILOT_TRIAGE_STATE/packet.md"

set +e
case "$AGENT_BACKEND" in
cursor)
	CURSOR_BIN=$(cursor_binary)
	cursor_args=(-p --force --trust --approve-mcps
		--workspace "$POKEPILOT_TRIAGE_TREE"
		--output-format text)
	if [ -n "$POKEPILOT_CURSOR_MODEL" ]; then
		cursor_args+=(--model "$POKEPILOT_CURSOR_MODEL")
	fi
	cursor_packet="$POKEPILOT_TRIAGE_TREE/.pokepilot-triage-packet.md"
	if ! grep -qxF '.pokepilot-triage-packet.md' "$POKEPILOT_TRIAGE_TREE/.git/info/exclude" 2>/dev/null; then
		printf '%s\n' '.pokepilot-triage-packet.md' >>"$POKEPILOT_TRIAGE_TREE/.git/info/exclude"
	fi
	cp "$POKEPILOT_TRIAGE_STATE/packet.md" "$cursor_packet"
	"$CURSOR_BIN" "${cursor_args[@]}" \
		"Read @.pokepilot-triage-packet.md and follow it exactly. Do not pick a different failure."
	agent_status=$?
	rm -f "$cursor_packet"
	;;
opencode)
	opencode run --auto --model qwen3.8-27b/qwen3.8-27b \
		--dir "$POKEPILOT_TRIAGE_TREE" \
		--title "farm triage ${KEY}" \
		--file "$POKEPILOT_TRIAGE_STATE/packet.md" \
		-- \
		"Follow the attached farm triage packet. Do not pick a different failure."
	agent_status=$?
	;;
esac
set -e
if [ "$agent_status" -ne 0 ]; then
	log "$AGENT_BACKEND exited $agent_status; no PR"
	exit 0
fi

branch=$(git -C "$POKEPILOT_TRIAGE_TREE" rev-parse --abbrev-ref HEAD)
case "$branch" in
fix/*) ;;
*)
	log "agent left branch $branch; refuse PR"
	exit 0
	;;
esac

main_head=$(git -C "$POKEPILOT_TRIAGE_TREE" rev-parse main)
origin_main=$(git -C "$POKEPILOT_TRIAGE_TREE" rev-parse origin/main)
if [ "$main_head" != "$origin_main" ]; then
	log "main moved; refuse PR"
	exit 0
fi

bad=$(git -C "$POKEPILOT_TRIAGE_TREE" diff --name-only origin/main...HEAD | grep -E '\.(state|gb|sav)$|^skill/zz_.*_test\.go$' || true)
if [ -n "$bad" ]; then
	log "forbidden paths in commit; refuse PR:"
	printf '%s\n' "$bad" >&2
	exit 0
fi

marker="[triage:${KEY}]"
if gh pr list --repo "$(gh_repo)" --state open --head "$branch" --json title --jq '.[].title' | grep -F -q "$marker"; then
	log "PR already open for $KEY"
	exit 0
fi
if ! git -C "$POKEPILOT_TRIAGE_TREE" rev-parse --verify "origin/$branch" >/dev/null 2>&1; then
	log "branch $branch not pushed; refuse PR"
	exit 0
fi

gh pr create --repo "$(gh_repo)" --head "$branch" \
	--title "fix(farm): ${EXAMPLE} ${marker}" \
	--body "Unattended ${AGENT_BACKEND} repair attempt for run \`${RUN_ID}\` (${marker})."
log "opened PR for $KEY with $AGENT_BACKEND"
