# Full-game media E2E smoke test

## Operator UI: Media → Full E2E

The private operator UI exposes **Media → Full E2E** (`#media/e2e`). The panel is the primary interface for starting this test; the CLI below remains useful in unattended environments.

1. Select **Fresh full game** (Red, starter, seed, registered strategist deployment) or **Existing run** to attach to an already completed Champion run.
2. Keep **Upload to YouTube privately** unchecked for the first test. Start the test and follow the stage-level progress.
3. The interface checks the semantic Hall of Fame completion, requests the broadcast replay, producer assessment, edited episode and Short, selected thumbnail and both private publication candidates using the existing servers.
4. Review the two generated formats in Studio. To make **real private uploads** on a later run, tick the upload option **and** its explicit acknowledgment before pressing Start.
5. Use **Export report** to record run ID, stage results, content hashes, publication candidate IDs and provider video IDs. Links open the source run, Media Library, Studio or produced videos.

**Coordination lifetime:** This first UI slice drives existing backend APIs from an open browser tab. Background game / renderer / already-dispatched upload jobs are server-owned, but *progression to the next step requires this tab to remain open*. Leaving Media → Full E2E, reloading, closing the browser or clicking **Stop coordinating** halts the coordinator and does not cancel the existing game, render or upload. Use **Existing run** to reattach and repeat idempotent preparation against durable run artifacts. A persisted server-side E2E orchestration queue is the next enhancement tracked in #2794.

The UI does not allow public or unlisted upload from this mode. It follows Studio's producer verdict and quality rules and never overrides `archive_only` / `do_not_publish`. A successful private-upload receipt means a provider ID was recorded for a candidate submitted as private; it is *not yet independent provider-side visibility or MP4 playback verification*.



Tracking issue: [#2794](https://github.com/maestroi/PokePilot/issues/2794).

This operator smoke test connects the **existing** farm, verified replay, Studio producer/editor, episode/Short packaging, quality-gated publication queue, and resumable YouTube uploader. It creates **no second gameplay runner, renderer, or uploader**. It is a CLI proof slice, not a durable on-server pipeline or UI mode. The CLI process must remain running through the test.

## Safe first test (no upload)

Requires a reachable **private operator** `pokeui`, connected `pokewall` / `pokereplay`, durable run recording and source ROM on an authorized runner. Test against a supported Red full-campaign configuration; do not use public spectator URL.

```sh
python3 tools/full_game_media_e2e.py \
  --base-url http://localhost:8080 \
  --launch --game pokemon-red --starter squirtle \
  --seed 1 --deployment YOUR_REGISTERED_MODEL \
  --timeout-seconds 172800 --report e2e-report.json
```

To process an **existing** run, substitute `--run-id YOUR_RUN_ID` for `--launch`. The run must have the full `elite-four` / Champion goal and authoritative goal-completion evidence: `status=done`, `reason=done`, and `stats.goal_complete=true` or a Hall of Fame milestone. Eight badges alone are insufficient. This implementation permits Yellow only with `--starter pikachu` but that game's full-game qualification must be independently verified; Red is the initial pilot.

The script requests a **broadcast** MP4, then calls the evidence-backed producer. A `do_not_publish` or `archive_only` verdict records an archive candidate and **fails the two-video acceptance test**, without uploading. A `short_only` result may generate its one allowed Short, but also fails the expected full episode + Short acceptance gate. A `full_episode` verdict prepares one landscape episode, one portrait Short, immutable edit plans/packages, one branded episode thumbnail, and two quality-gated **private** publication candidates. Existing quality gates may reject any candidate; this is a test failure, not permission to bypass safeguards.

## Optional real private YouTube upload

**This performs real channel uploads**, consuming bandwidth, provider quota, tokens and time. Use a test channel and only an operator who is authorized to publish. Add `--upload-private` to the command above **after** the dry run succeeds.

```sh
python3 tools/full_game_media_e2e.py \
  --base-url http://localhost:8080 \
  --run-id YOUR_FINISHED_RUN_ID \
  --upload-private --timeout-seconds 172800 \
  --report e2e-private-report.json
```

The switch checks uploader configuration, requests *only* `privacy=private`, invokes the existing Studio **approve** then **publish** actions, waits through the channel minimum upload interval (default 15 minutes), and checks the returned durable queue state and YouTube provider IDs. It cannot request public or unlisted videos. Credentials belong in `pokereplay` server-side environment, never in CLI args or reports. If your private operator reverse proxy needs a bearer token, supply it through `POKEPILOT_E2E_TOKEN` or the name set in `--token-env`; tokens aren't copied into JSON reports. HTTP redirects are disabled to avoid forwarding credentials.

**Privacy validation limit:** the v1 API exposes the requested `private` visibility and the provider ID; it does not independently read the resulting YouTube video's visibility back from the YouTube API. Do not treat `passed` as a final externally verified privacy/asset-quality proof until that check and MP4 probes are added. Check the two resulting videos in the account before any public use. Publishing retries rely on #178/#803's resumable/idempotent upload infrastructure; do not run multiple concurrent publisher replicas.

## Evidence and failure behavior

The local JSON report records the run ID/game, stage history, producer decision, immutable plan/package/candidate hashes, publication state and provider IDs/URLs when applicable; it does **not** contain ROMs, model traces, OAuth tokens, or game screenshots. The CLI exits nonzero at the first blocked stage and records `failed_stage` and `error` when `--report` is supplied. After fixing a failure, retry with `--run-id` to reuse the same immutable run evidence rather than restarting gameplay. Uploads run only in explicitly opted-in mode.

To run ROM-free API contract tests:

```sh
python3 -m unittest discover -s tests -p 'test_full_game_media_e2e.py' -v
```

The tests simulate server responses. **They do not prove actual game completion, video codecs/aspect ratio, real S3 durability, actual YouTube visibility or external replay playback.** The private on-demand/overnight test environment, durable Swarm orchestrator, UI control, external YouTube privacy readback, asset probes and crash/concurrency verification are tracked in #2794. Public CI must remain ROM-free and must never receive YouTube secrets.
