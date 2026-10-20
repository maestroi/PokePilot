# Full-game media E2E smoke test

## Server-owned production (no browser required)

`pokereplay` runs production for every finished run tagged `purpose: "full_e2e"`. Once a minute it lists those runs from wall. For each run that ends `reason=done` with the adapter's `stats.goal_complete=true`, it moves through: broadcast render → producer assessment → episode and Short edits → packages and thumbnail → **private** publication candidates. It calls its own Studio handlers in-process, so the quality gates and validation are the same as the operator path. Renders remain wall-leased jobs. Progress is stored in S3 at `studio/<sha256(run)>/production/v1.json`, which pins the LLM-backed producer assessment so it is never repeated, and survives replay restarts. Read it with `GET /v1/runs/{id}/production`; Media → Full E2E shows it as **Production**. A stage that fails five passes in a row stops as `failed_<stage>` with its error.

Without the private-upload opt-in, production stops at `ready_for_review` and candidates wait in Studio. With it, production also approves and uploads both candidates as private videos (see below).

## Durable E2E badge and database filtering

**New Full E2E launches are explicitly tagged `purpose: "full_e2e"`** at farm run creation, independently of the goal, play style and model. This is saved with the run's authoritative record and materialized as an indexed `runs.purpose` column by PostgreSQL schema migration 007 (local SQLite catalogs receive the same column). It is returned from both `GET /v1/runs/{id}` and `GET /v1/dashboard`.

Operators can filter all completed E2E runs through **Runs → E2E only** or query `GET /v1/dashboard?status=done&purpose=full_e2e&limit=25&offset=0`. Active tagged runs are available at `GET /v1/dashboard?active=1&purpose=full_e2e`. Runs and Media Library show an **E2E** badge only for the explicit database tag. Media → Full E2E loads tagged active/finished runs using this database filter, rather than depending on the last few generic runs.

The badge means **launched as E2E**, not **E2E passed**. Outcome and media upload completion come from separate authoritative game/replay/publication records. Launches created before this change remain untagged (even if a previous UI workflow started them), because goal text or ID prefixes are not trustworthy evidence of E2E intent. The compatibility section can still surface them as possible Champion source runs.

## Finding a previous E2E run and checking whether it worked

Go to **Media → Full E2E**. It lists tagged E2E runs from the farm database and recent Pokémon Red Champion-goal runs, which are marked **Champion candidate**, not claimed as E2E. Enter an older run's ID under **Inspect a run**.

Select a run to see the server's record, refreshed about every 10 seconds. **Both private upload receipts recorded** requires durable `published` candidates with private intent and provider video IDs; it does **not** independently prove YouTube-side visibility. **Episode + Short in publication queue** only proves both private candidates exist.

## Operator UI: New run → Full E2E (recommended)

Open the **private operator UI → New run**, choose **Pokémon Red** and **Play the game** as the LLM planner, then select **Run workflow → Full E2E · complete game → episode + Short**. The workflow locks the goal to *Beat the Elite Four and Champion*, makes the campaign finite, and uses resilient recovery while retaining the selected model, starter, seed, play style and FPS.

Press **Start Full E2E** **once**. The New run screen queues exactly one farm run tagged `purpose=full_e2e` and opens **Media → Full E2E** for it. pokereplay's production loop (`cmd/pokereplay/production.go`) owns everything after that, so no browser needs to stay open: once the run finishes with the goal flag it renders the broadcast replay, asks the producer, renders the episode and portrait Short, packages them with a thumbnail, and creates private publication candidates. Progress is `GET /v1/runs/{id}/production`; a run that exhausted its retries shows **Retry production** (`POST /v1/runs/{id}/production/retry`).

Uploads are **off by default**. Tick **Also upload … as private videos** and its acknowledgment before **Start Full E2E** to record `POST /v1/runs/{id}/production {"upload_private":true}` (refused when the replay server has no uploader, before any gameplay is queued). The loop then approves and publishes each private candidate through the normal Studio publish action, one at a time, waiting on channel pacing, and records the video links. A candidate blocked by the quality gate ends production as `failed_<format>_upload`; public or unlisted is never used. The same request on a run sitting in `ready_for_review` uploads it after the fact.

## Operator UI: Media → Full E2E (inspect)

**Media → Full E2E** (`#media/e2e`) lists tagged E2E runs (and older untagged Champion runs) and shows the server's record for one: farm outcome, Champion proof, production stage, replay, publication candidates, edit previews and uploaded video links. It coordinates nothing itself. From there an operator can **Retry production** after a `failed_*` stage, or **Upload privately…** (with an explicit acknowledgment) on a `full_e2e` run that was launched without the upload opt-in.

Tracking issue: [#2794](https://github.com/maestroi/PokePilot/issues/2794).

This smoke test connects the **existing** farm, verified replay, Studio producer/editor, episode/Short packaging, quality-gated publication queue, and resumable YouTube uploader. It creates **no second gameplay runner, renderer, or uploader**. The CLI below is a separate non-UI option whose process must remain running.

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
