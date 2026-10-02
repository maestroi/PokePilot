#!/usr/bin/env bash
# Swarm supervisor for one headless farm fixer. The ROM, GitHub token, Cursor
# API key, and MCP token come from the environment. This process clones the
# repo onto the bind-mounted state dir and then runs qwagent-triage.sh.
set -euo pipefail

POKEPILOT_GITHUB_REPO=${POKEPILOT_GITHUB_REPO:-maestroi/PokePilot}
POKEPILOT_ROOT=${POKEPILOT_ROOT:-/var/lib/pokefixer/source}
POKEPILOT_TRIAGE_TREE=${POKEPILOT_TRIAGE_TREE:-/var/lib/pokefixer/work}
POKEPILOT_TRIAGE_STATE=${POKEPILOT_TRIAGE_STATE:-/var/lib/pokefixer/state}
POKEMON_RED_ROM=${POKEMON_RED_ROM:-/rom/pokemon_red.gb}
POKEPILOT_TRIAGE_AGENT=${POKEPILOT_TRIAGE_AGENT:-ladder}
POKEPILOT_CURSOR_MODEL=${POKEPILOT_CURSOR_MODEL:-auto}
POKEPILOT_FIXER_INTERVAL=${POKEPILOT_FIXER_INTERVAL:-2m}
POKEPILOT_MCP_URL=${POKEPILOT_MCP_URL:-https://admin.rompilot.app/mcp}
POKEPILOT_OPENCODE_MODEL=${POKEPILOT_OPENCODE_MODEL:-qwen3.8-27b/qwen3.8-27b}
POKEPILOT_QWEN_URL=${POKEPILOT_QWEN_URL:-}

export GH_TOKEN="${GH_TOKEN:-${POKEPILOT_FIXER_GITHUB_TOKEN:-${POKEPILOT_GITHUB_TOKEN:-}}}"
export CURSOR_API_KEY="${CURSOR_API_KEY:-${POKEPILOT_FIXER_CURSOR_API_KEY:-}}"
export POKEPILOT_ROOT POKEPILOT_TRIAGE_TREE POKEPILOT_TRIAGE_STATE
export POKEMON_RED_ROM POKEPILOT_TRIAGE_AGENT POKEPILOT_CURSOR_MODEL POKEPILOT_MCP_URL
export POKEPILOT_OPENCODE_MODEL POKEPILOT_QWEN_URL
export GOCACHE="${GOCACHE:-/var/lib/pokefixer/gocache}"
export GOMODCACHE="${GOMODCACHE:-/var/lib/pokefixer/gomodcache}"

log() { echo "fixer: $*" >&2; }

if [ -z "$GH_TOKEN" ]; then
	log "GitHub token missing; set POKEPILOT_FIXER_GITHUB_TOKEN (Issues and Pull requests write)"
	exit 1
fi
if [ -z "${POKEPILOT_MCP_TOKEN:-}" ]; then
	log "POKEPILOT_MCP_TOKEN missing"
	exit 1
fi
if [ -z "$CURSOR_API_KEY" ]; then
	log "Cursor API key missing; set POKEPILOT_FIXER_CURSOR_API_KEY from the account whose allowance should pay"
	exit 1
fi
if [ ! -f "$POKEMON_RED_ROM" ]; then
	log "ROM not mounted at $POKEMON_RED_ROM"
	exit 1
fi
if ! command -v agent >/dev/null 2>&1; then
	log "Cursor CLI (agent) is not installed in this image"
	exit 1
fi

# OpenCode reaches qwen over the LAN; without a URL the qwen tier is skipped
# and the ladder starts at Cursor.
if [ -n "$POKEPILOT_QWEN_URL" ]; then
	provider=${POKEPILOT_OPENCODE_MODEL%%/*} model=${POKEPILOT_OPENCODE_MODEL#*/}
	mkdir -p "$HOME/.config/opencode"
	cat >"$HOME/.config/opencode/opencode.json" <<EOF
{
  "provider": {
    "$provider": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "$provider (fixer)",
      "options": { "baseURL": "$POKEPILOT_QWEN_URL" },
      "models": {
        "$model": {
          "name": "$model",
          "limit": { "context": 131072, "output": 32768 },
          "options": { "extraBody": { "chat_template_kwargs": { "enable_thinking": true, "reasoning_effort": "medium" } } }
        }
      }
    }
  }
}
EOF
else
	rm -f "$HOME/.opencode/bin/opencode"
	log "POKEPILOT_QWEN_URL unset; qwen tier disabled"
fi

git config --global user.name "${GIT_AUTHOR_NAME:-pokefixer}"
git config --global user.email "${GIT_AUTHOR_EMAIL:-pokefixer@users.noreply.github.com}"
git config --global init.defaultBranch main
# HTTPS remotes then push with GH_TOKEN. An SSH key is not required.
gh auth setup-git

origin="https://github.com/${POKEPILOT_GITHUB_REPO}.git"
mkdir -p "$(dirname "$POKEPILOT_ROOT")" "$POKEPILOT_TRIAGE_STATE" "$GOCACHE" "$GOMODCACHE"
if [ ! -d "$POKEPILOT_ROOT/.git" ]; then
	log "cloning $origin"
	git clone "$origin" "$POKEPILOT_ROOT"
fi
git -C "$POKEPILOT_ROOT" remote set-url origin "$origin"

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# The image copy is the supervisor's script. POKEPILOT_ROOT is only the
# checkout the picker is built from and the tree the agent resets.
export POKEPILOT_TRIAGE_PROMPT="${POKEPILOT_TRIAGE_PROMPT:-$script_dir/qwagent-triage.prompt.md}"
triage_script="${POKEPILOT_TRIAGE_SCRIPT:-$script_dir/qwagent-triage.sh}"

log "ladder=${POKEPILOT_TRIAGE_LADDER:-default} qwen=${POKEPILOT_QWEN_URL:-off} cursor model=$POKEPILOT_CURSOR_MODEL interval=$POKEPILOT_FIXER_INTERVAL rom=$POKEMON_RED_ROM"
while true; do
	if git -C "$POKEPILOT_ROOT" fetch --prune origin && git -C "$POKEPILOT_ROOT" checkout -f main && git -C "$POKEPILOT_ROOT" reset --hard origin/main; then
		set +e
		"$triage_script"
		status=$?
		set -e
		log "attempt exited $status"
	else
		log "could not update $POKEPILOT_ROOT; skipping this tick"
	fi
	sleep "$POKEPILOT_FIXER_INTERVAL"
done
