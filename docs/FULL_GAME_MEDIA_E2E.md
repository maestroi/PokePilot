# Full-game media E2E smoke test

## Finding a previous E2E run and checking whether it worked

Go to **Media → Full E2E**. The **E2E runs and Champion candidates** section now restores the last 100 browser E2E receipts across reloads (on the same browser/origin) and discovers recent Pokémon Red Champion-goal runs directly from the farm API, including still-running games. Farm-discovered runs are marked **Champion candidate**, not automatically claimed as E2E runs: old run specs did not record the E2E intent durably. If an older run has fallen out of the recent dashboard window, enter its ID in **Existing run**.

Select a run to see **Server evidence**, refreshed approximately every 10 seconds: the current game state, authoritative Champion goal flag, broadcast render state and episode/Short publication queue. A previous browser receipt is shown separately from independently fetched server state. **Two private upload receipts recorded** requires durable `published` candidates with private intent and provider video IDs; it does **not** independently prove YouTube-side visibility. **Episode + Short in publication queue** only proves both private candidates exist, not a finished upload.

An earlier `working` browser receipt is explicitly presented as **interrupted** after reload, never falsely shown as still executing or successful. **Resume production · no upload** attaches to the same run without creating a second game and completes remaining production stages if the game qualifies. It never silently restores consent to upload: real uploads must be explicitly enabled and confirmed again. This history is a **browser-local troubleshooting aid**, not yet the desired server-owned E2E job ledger, which remains tracked by #2794. Jobs started on another device or browser are discoverable through the farm's Champion-run history, but their original E2E intent may not be provable.

## Operator UI: New run → Full E2E (recommended)

Open the **private operator UI → New run**, choose **Pokémon Red** and **Play the game** as the LLM planner, then select **Run workflow → Full E2E · complete game → episode + Short**. The workflow locks the goal to *Beat the Elite Four and Champion*, makes the campaign finite, and uses resilient recovery while retaining the selected model, starter, seed, play style and FPS.

Press **Start Full E2E** **once**. The New run screen creates exactly one farm run and automatically starts production coordination for that accepted run ID. It navigates to **Media → Full E2E**, where you can watch the game, replay rendering, episode and portrait Short editing, quality-gated private candidates and (if opted in) upload receipts. No second Start click or second game run is required. You may navigate to other operator screens while the same page stays open; the active coordinator is shared at application scope rather than owned by the Media tab.

Uploads are **off by default**. To publish actual media, select **Also upload episode and Short privately to YouTube** and separately acknowledge the two real uploads **before** clicking Start Full E2E. The pipeline never offers public or unlisted uploads in this mode, and the existing producer and Studio upload gates remain authoritative.

## Operator UI: Media → Full E2E (attach or replay)

The private operator UI exposes **Media → Full E2E** (`#media/e2e`). This screen also starts a fresh test or attaches to an existing run. The CLI below remains useful in unattended environments.

1. Select **Fresh full game** (Red, starter, seed, registered strategist deployment) or **Existing run** to attach to an already completed Champion run.
2. Keep **Upload to YouTube privately** unchecked for the first test. Start the test and follow the stage-level progress.
3. The interface checks the semantic Hall of Fame completion, requests the broadcast replay, producer assessment, edited episode and Short, selected thumbnail and both private publication candidates using the existing servers.
4. Review the two generated formats in Studio. To make **real private uploads** on a later run, tick the upload option **and** its explicit acknowledgment before pressing Start.
5. Use **Export report** to record run ID, stage results, content hashes, publication candidate IDs and provider video IDs. Links open the source run, Media Library, Studio or produced videos.

**Coordination lifetime:** This first UI slice drives existing backend APIs from an open operator browser page. Background game / renderer / already-dispatched upload jobs are server-owned. **Switching operator tabs does not stop coordination**, but reloading, closing the browser or clicking **Stop coordinating** halts the coordinator; it does not cancel the existing game, render or upload. Use **Existing run** to reattach and repeat idempotent preparation against durable run artifacts. A persisted server-side E2E orchestration queue is the next enhancement tracked in #2794.

The UI does not allow public or unlisted upload from this mode. It follows Studio's producer verdict and quality rules and never overrides `archive_only` / `do_not_publish`. A successful private-upload receipt means a provider ID was recorded for a candidate submitted as private; it is *not yet independent provider-side visibility or MP4 playback verification*.



Tracking issue: [#2794](https://github.com/maestroi/PokePilot/issues/2794).

This operator smoke test connects the **existing** farm, verified replay, Studio producer/editor, episode/Short packaging, quality-gated publication queue, and resumable YouTube uploader. It creates **no second gameplay runner, renderer, or uploader**. The CLI is a separate non-UI option; unlike the app-level UI coordinator, the CLI process itself must remain running. Neither coordinator is yet a durable server-owned pipeline.

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
