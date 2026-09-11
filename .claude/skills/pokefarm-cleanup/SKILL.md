---
name: pokefarm-cleanup
description: Use when asked to delete obsolete PokeFarm run data for a known/fixed bug or one triage failure group. Resolves the stable triage key, dry-runs the exact matching set, then deletes through pokeui so S3/replay artifacts are purged before wall history.
---

# PokeFarm bug-run cleanup

Use this only for deleting historical run data after the user says a bug/failure group is no longer useful. Do not select runs by vague text similarity and do not bulk-delete from `pokepilot_list_runs` directly.

## 1. Resolve the exact failure group

Start with:

```
pokepilot_get_triage(include_resolved=true)
```

Choose the stable triage `key` that matches the bug the user named. Prefer a group whose linked issue is already resolved/fixed when the request says the problem is solved. If more than one group plausibly matches, report the candidates instead of guessing.

## 2. Dry-run the cleanup

From the PokePilot repository:

```bash
go run ./cmd/pokecleanup -key <triage-key>
```

The command fetches the full finished-run catalog, applies the same normalized failure pattern as pokewall, and prints every matching run id. The five sample `run_ids` on `/v1/triage` are deliberately not used as the deletion set.

Check that the printed pattern and count match the intended bug. The dry run never deletes anything.

## 3. Delete after the user has asked for it

The user's request to remove that bug group's data is the destructive-action confirmation. After the dry-run set is correct, run:

```bash
go run ./cmd/pokecleanup -key <triage-key> -yes
```

The command calls pokeui's existing `DELETE /v1/runs/{id}` route with bounded concurrency. That route purges run-owned S3/replay artifacts first and removes pokewall history only after artifact cleanup succeeds. Failed deletions remain in history and are reported so they can be retried.

The command only matches finished `error`/`lost` runs in the exact triage pattern; active runs and successful runs are excluded.

## 4. Report the result

State the triage key/pattern, how many runs were deleted, and any run ids that failed. Do not claim a failed deletion was removed.
