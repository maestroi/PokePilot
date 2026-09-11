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

POKEPILOT_WALL=${POKEPILOT_WALL:-https://pokemon.labstack.cc}
POKEPILOT_TRIAGE_STATE=${POKEPILOT_TRIAGE_STATE:-$HOME/.local/share/pokepilot/qwagent-triage}
POKEPILOT_TRIAGE_TREE=${POKEPILOT_TRIAGE_TREE:-$HOME/Documents/projects/PokePilot-qwagent-triage}
PROMPT=${POKEPILOT_TRIAGE_PROMPT:-$SCRIPT_DIR/qwagent-triage.prompt.md}
export PATH="$HOME/.opencode/bin:$HOME/go/bin:$HOME/.local/bin:/usr/local/go/bin:$PATH"

DRY_RUN=0
LOCKED=0
for arg in "$@"; do
	case "$arg" in
	--dry-run) DRY_RUN=1 ;;
	--locked) LOCKED=1 ;;
	esac
done

log() { echo "qwagent-triage: $*" >&2; }

mkdir -p "$POKEPILOT_TRIAGE_STATE"

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

pick_next() {
	local triage titles pick_args status
	if ! triage=$(curl -fsS -H "Authorization: Bearer ${POKEPILOT_MCP_TOKEN}" "${POKEPILOT_WALL}/v1/triage"); then
		log "wall unreachable; skip"
		return 2
	fi
	titles=$(gh pr list --repo "$(gh_repo)" --state open --limit 50 --json title --jq '.[].title' 2>/dev/null || true)
	pick_args=(pick)
	while IFS= read -r title; do
		[ -z "$title" ] && continue
		pick_args+=(--claimed "$title")
	done <<<"$titles"
	# go run rewrites a child exit 2 into its own exit 1; build so idle stays 2.
	bin="$POKEPILOT_TRIAGE_STATE/qwagent-triage"
	(cd "$POKEPILOT_ROOT" && go build -o "$bin" ./cmd/qwagent-triage)
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

OPENCODE_CMD=(opencode run --auto --model qwen3.8-27b/qwen3.8-27b
	--dir "$POKEPILOT_TRIAGE_TREE"
	--title "farm triage ${KEY}"
	--file "$POKEPILOT_TRIAGE_STATE/packet.md"
	--
	"Follow the attached farm triage packet. Do not pick a different failure.")

if [ "$DRY_RUN" -eq 1 ]; then
	printf '%s\n' "$PICK_JSON"
	echo "would claim $KEY (run $RUN_ID)"
	echo "would run: ${OPENCODE_CMD[*]}"
	exit 0
fi

log "claiming $KEY ($EXAMPLE) run=$RUN_ID"
# Investigate is a best-effort claim. Auto-filed issues are often already
# investigating, and the orchestrator then 409s (the wall currently maps
# that to 502). An open PR is the durable skip; do not abort the local agent.
invest_body=$(mktemp)
set +e
invest_code=$(curl -sS -o "$invest_body" -w '%{http_code}' -X POST \
	-H "Authorization: Bearer ${POKEPILOT_MCP_TOKEN}" \
	"${POKEPILOT_WALL}/v1/triage/${KEY}/investigate")
set -e
case "$invest_code" in
200 | 201 | 202 | 409) ;;
*)
	log "investigate HTTP ${invest_code:-err}: $(tr '\n' ' ' <"$invest_body"); continuing locally"
	;;
esac
rm -f "$invest_body"

if [ ! -d "$POKEPILOT_TRIAGE_TREE/.git" ]; then
	mkdir -p "$(dirname "$POKEPILOT_TRIAGE_TREE")"
	if ! git clone --reference "$POKEPILOT_ROOT" "$POKEPILOT_ROOT" "$POKEPILOT_TRIAGE_TREE"; then
		git clone "$(git -C "$POKEPILOT_ROOT" remote get-url origin)" "$POKEPILOT_TRIAGE_TREE"
	fi
fi
git -C "$POKEPILOT_TRIAGE_TREE" fetch origin
git -C "$POKEPILOT_TRIAGE_TREE" checkout main
git -C "$POKEPILOT_TRIAGE_TREE" reset --hard origin/main
git -C "$POKEPILOT_TRIAGE_TREE" clean -fd

printf '%s\n' "$PICK_JSON" >"$POKEPILOT_TRIAGE_STATE/packet.json"
{
	cat "$PROMPT"
	printf '\n\n## Packet\n\n```json\n'
	cat "$POKEPILOT_TRIAGE_STATE/packet.json"
	printf '```\n'
} >"$POKEPILOT_TRIAGE_STATE/packet.md"

set +e
"${OPENCODE_CMD[@]}"
agent_status=$?
set -e
if [ "$agent_status" -ne 0 ]; then
	log "opencode exited $agent_status; no PR"
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
	--body "Unattended qwagent attempt for run \`${RUN_ID}\` (${marker})."
log "opened PR for $KEY"
