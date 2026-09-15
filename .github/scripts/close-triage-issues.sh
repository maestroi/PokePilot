#!/usr/bin/env bash
set -euo pipefail

: "${PR:?PR is required}"
: "${MERGE_SHA:?MERGE_SHA is required}"
: "${REPO:?REPO is required}"

# Autonomous repair PRs traditionally put [triage:key] in the title, while
# manual/interactive repair PRs often keep it in the body. Both are part of
# the PR's repair contract, so scan both and deduplicate before matching
# generated farm issues.
pr_text="$(gh pr view "$PR" --json title,body --jq '[.title, (.body // "")] | join("\n")')"
mapfile -t keys < <(
  grep -oE '\[triage:[A-Za-z0-9._-]+\]' <<<"$pr_text" \
    | sed -E 's/^\[triage:([^]]+)\]$/\1/' \
    | sort -u \
    || true
)
if (( ${#keys[@]} == 0 )); then
  echo "PR #${PR} has no triage key in its title or body; nothing to close."
  exit 0
fi

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
    gh issue comment "$issue" --body \
      "Repair merged in #${PR} at \`${MERGE_SHA}\` with \`[triage:${key}]\`. Closing this farm fingerprint automatically; a recurrence from a runner containing the repair is still eligible to reopen as a regression through the revision gate."
    gh issue close "$issue" --reason completed
    echo "Closed issue #${issue} for triage key ${key}."
  done
done
