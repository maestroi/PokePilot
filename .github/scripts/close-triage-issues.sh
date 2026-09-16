#!/usr/bin/env bash
set -euo pipefail

: "${PR:?PR is required}"
: "${MERGE_SHA:?MERGE_SHA is required}"
: "${REPO:?REPO is required}"

# Autonomous repair PRs traditionally put [triage:key] in the title, while
# manual/interactive repair PRs often keep it in the body. Both are part of
# the PR's repair contract, so scan both and deduplicate before matching
# generated farm issues. Broader repairs may also explicitly claim individual
# generated issues with [farm-issue:<number>]; those markers are intentionally
# exact so a PR merely mentioning an issue does not close it by accident.
pr_text="$(gh pr view "$PR" --json title,body --jq '[.title, (.body // "")] | join("\n")')"
mapfile -t keys < <(
  grep -oE '\[triage:[A-Za-z0-9._-]+\]' <<<"$pr_text" \
    | sed -E 's/^\[triage:([^]]+)\]$/\1/' \
    | sort -u \
    || true
)
mapfile -t explicit_issues < <(
  grep -oE '\[farm-issue:[0-9]+\]' <<<"$pr_text" \
    | sed -E 's/^\[farm-issue:([0-9]+)\]$/\1/' \
    | sort -nu \
    || true
)

if (( ${#keys[@]} == 0 && ${#explicit_issues[@]} == 0 )); then
  echo "PR #${PR} has no triage or explicit farm-issue repair markers; nothing to close."
  exit 0
fi

declare -A closed_issues=()
generated_marker='<!-- pokepilot-generated:github-issues-v1 -->'

close_generated_issue() {
  local issue="$1"
  local repair_marker="$2"

  if [[ -n "${closed_issues[$issue]:-}" ]]; then
    return 0
  fi

  local issue_json
  if ! issue_json="$(gh api "/repos/${REPO}/issues/${issue}" 2>/dev/null)"; then
    echo "Skipping ${repair_marker}: issue #${issue} does not exist or is not readable."
    return 0
  fi

  if [[ "$(jq -r 'if .pull_request == null then "issue" else "pull_request" end' <<<"$issue_json")" != "issue" ]]; then
    echo "Skipping ${repair_marker}: #${issue} is a pull request, not a farm issue."
    return 0
  fi
  if [[ "$(jq -r '.state // ""' <<<"$issue_json")" != "open" ]]; then
    echo "Skipping ${repair_marker}: issue #${issue} is already closed."
    return 0
  fi
  if ! jq -e --arg marker "$generated_marker" '(.body // "") | contains($marker)' >/dev/null <<<"$issue_json"; then
    echo "Skipping ${repair_marker}: issue #${issue} is not a generated PokePilot farm issue."
    return 0
  fi

  gh issue comment "$issue" --body \
    "Repair merged in #${PR} at \`${MERGE_SHA}\` with \`${repair_marker}\`. Closing this generated farm issue automatically; a recurrence from a runner containing the repair is still eligible to reopen as a regression through the revision gate."
  gh issue close "$issue" --reason completed
  closed_issues[$issue]=1
  echo "Closed generated farm issue #${issue} for ${repair_marker}."
}

for key in "${keys[@]}"; do
  marker="- **Triage key:** \`${key}\`"
  mapfile -t issues < <(
    gh api --paginate "/repos/${REPO}/issues?state=open&per_page=100" \
      | jq -r --arg marker "$marker" '
          .[]
          | select(.pull_request == null)
          | select((.body // "") | contains($marker))
          | .number
        '
  )

  if (( ${#issues[@]} == 0 )); then
    echo "No open farm issue matches triage key ${key}."
    continue
  fi

  for issue in "${issues[@]}"; do
    close_generated_issue "$issue" "[triage:${key}]"
  done
done

for issue in "${explicit_issues[@]}"; do
  close_generated_issue "$issue" "[farm-issue:${issue}]"
done
