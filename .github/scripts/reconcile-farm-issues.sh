#!/usr/bin/env bash
set -euo pipefail

: "${REPO:?REPO is required}"

generated_marker='<!-- pokepilot-generated:github-issues-v1 -->'
latest_prefix='<!-- pokepilot-latest-observed-revision:'
dry_run="${DRY_RUN:-0}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

issues_json="$tmpdir/issues.json"
pulls_json="$tmpdir/pulls.json"

gh api --paginate "/repos/${REPO}/issues?state=open&per_page=100" | jq -s 'add // []' >"$issues_json"
gh api --paginate "/repos/${REPO}/pulls?state=closed&base=main&per_page=100" | jq -s 'add // []' >"$pulls_json"

latest_revision() {
  local body="$1"
  local rev
  rev="$(
    grep -oE '<!-- pokepilot-latest-observed-revision:[^ ]+ -->' <<<"$body"       | tail -n1       | sed -E 's/^<!-- pokepilot-latest-observed-revision:([^ ]+) -->$/\1/'       || true
  )"
  if [[ -n "$rev" ]]; then
    printf '%s\n' "$rev"
    return 0
  fi
  grep -m1 -oE -- '- \*\*Revision:\*\* `[^`]+`' <<<"$body"     | sed -E 's/^- \*\*Revision:\*\* `([^`]+)`$/\1/'     || true
}

triage_key() {
  local body="$1"
  grep -m1 -oE -- '- \*\*Triage key:\*\* `[A-Za-z0-9._-]+`' <<<"$body"     | sed -E 's/^- \*\*Triage key:\*\* `([A-Za-z0-9._-]+)`$/\1/'     || true
}

close_issue() {
  local issue="$1"
  local key="$2"
  local observed="$3"
  local pr="$4"
  local repair_sha="$5"

  if [[ "$dry_run" == "1" ]]; then
    echo "Would close #${issue}: [triage:${key}] latest revision ${observed} is contained in repair #${pr} (${repair_sha})."
    return 0
  fi

  gh issue comment "$issue" --repo "$REPO" --body     "Lifecycle reconciliation: latest recorded farm revision \`${observed}\` predates and is contained in merged repair #${pr} (\`${repair_sha}\`) for \`[triage:${key}]\`. Closing this generated issue as repaired. A later runner revision that reproduces the fingerprint remains eligible to reopen it through the regression gate."
  gh issue close "$issue" --repo "$REPO" --reason completed
  echo "Closed generated farm issue #${issue}: repair #${pr} covers latest observation ${observed}."
}

closed=0
kept=0
skipped=0

while IFS= read -r issue; do
  number="$(jq -r '.number' <<<"$issue")"
  body="$(jq -r '.body // ""' <<<"$issue")"

  if [[ "$body" != *"$generated_marker"* ]]; then
    continue
  fi

  key="$(triage_key "$body")"
  observed="$(latest_revision "$body")"
  if [[ -z "$key" || -z "$observed" ]]; then
    echo "Keep #${number}: generated issue is missing triage key or observed revision."
    ((skipped+=1))
    continue
  fi

  if [[ ! "$observed" =~ ^[0-9a-fA-F]{7,40}$ ]]; then
    echo "Keep #${number}: observed revision ${observed} is not a Git commit id."
    ((skipped+=1))
    continue
  fi

  if ! observed_full="$(git rev-parse --verify "${observed}^{commit}" 2>/dev/null)"; then
    echo "Keep #${number}: observed revision ${observed} is not present in repository history."
    ((skipped+=1))
    continue
  fi

  marker="[triage:${key}]"
  repaired=0
  while IFS=$'\t' read -r pr repair_sha merged_at; do
    [[ -n "$pr" && -n "$repair_sha" ]] || continue
    if ! repair_full="$(git rev-parse --verify "${repair_sha}^{commit}" 2>/dev/null)"; then
      continue
    fi

    # Equality means the supposedly fixed build itself reproduced the bug.
    # A descendant observation is likewise a real current regression. Close
    # only when the latest observed runner revision is strictly older than and
    # contained by the merged repair.
    if [[ "$observed_full" == "$repair_full" ]]; then
      continue
    fi
    if git merge-base --is-ancestor "$observed_full" "$repair_full"; then
      close_issue "$number" "$key" "$observed_full" "$pr" "$repair_full"
      repaired=1
      ((closed+=1))
      break
    fi
  done < <(
    jq -r --arg marker "$marker" '
      [
        .[]
        | select(.merged_at != null)
        | select((((.title // "") + "\n" + (.body // "")) | contains($marker)))
        | [.number, .merge_commit_sha, .merged_at]
      ]
      | sort_by(.[2])
      | reverse[]
      | @tsv
    ' "$pulls_json"
  )

  if [[ "$repaired" -eq 0 ]]; then
    echo "Keep #${number}: no merged [triage:${key}] repair contains latest observation ${observed_full}."
    ((kept+=1))
  fi
done < <(
  jq -c --arg marker "$generated_marker" '
    .[]
    | select(.pull_request == null)
    | select((.body // "") | contains($marker))
  ' "$issues_json"
)

echo "Farm issue reconciliation complete: closed=${closed} kept=${kept} skipped=${skipped} dry_run=${dry_run}."
