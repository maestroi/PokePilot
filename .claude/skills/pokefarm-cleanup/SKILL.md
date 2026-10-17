---
name: pokefarm-cleanup
description: Use when asked to delete obsolete PokeFarm run data for a known/fixed bug or one triage failure group. Resolves the stable triage key or linked issue number, dry-runs the exact matching set, then deletes through pokeui so S3/replay artifacts are purged before wall history.
---

# PokeFarm bug-run cleanup

Use this only for deleting historical run data after the user says a bug/failure group is no longer useful. Do not select runs by vague text similarity and do not bulk-delete from `pokepilot_list_runs` directly.

## 1. Resolve the exact failure group

Start with:

```
pokepilot_get_triage(include_resolved=true, limit=20)
```

MCP returns compact failure groups (stable `key`, `pattern`, issue number/status,
and up to three representative run IDs) with a `total` count and optional
`next_offset`. **Do not treat the first page as the whole history.** If the
named key or issue is not present, call
`pokepilot_get_triage(include_resolved=true, limit=20, offset=<next_offset>)`
until the match is found or there are no more pages. If several groups can
match the requested issue, check the remaining pages rather than guessing.

Choose either the stable triage `key` or linked issue number matching the
named bug. Prefer a group whose linked issue is resolved/fixed when the request
says the problem is solved. The compact result is sufficient to choose an exact
key; do not request `view="full"` or all `run_ids` just for selection.

## 2. Dry-run the cleanup

From the PokePilot repository, use whichever identifier the operator has:

```bash
go run ./cmd/pokecleanup -issue 232
go run ./cmd/pokecleanup -key <triage-key>
```

The command fetches the full finished-run catalog, applies the same normalized failure pattern as pokewall, and prints every matching run id. The sampled `run_ids` returned by MCP are deliberately not used as the deletion set.

Check that the printed issue/key, pattern, and count match the intended bug. The dry run never deletes anything.

## 3. Delete after the user has asked for it

The user's request to remove that bug group's data is the destructive-action confirmation. After the dry-run set is correct, run one of:

```bash
go run ./cmd/pokecleanup -issue 232 -yes
go run ./cmd/pokecleanup -key <triage-key> -yes
```

During deletion the command prints live progress such as `930 / 5041`, plus deleted/failed counts. It calls pokeui's existing `DELETE /v1/runs/{id}` route with bounded concurrency. That route purges run-owned S3/replay artifacts first and removes pokewall history only after artifact cleanup succeeds. Failed deletions remain in history and are reported so they can be retried.

The command only matches finished `error`/`lost` runs in the exact triage pattern; active runs and successful runs are excluded.

## 4. Report the result

State the issue number and triage key/pattern, how many runs were deleted, and any run ids that failed. Do not claim a failed deletion was removed.
