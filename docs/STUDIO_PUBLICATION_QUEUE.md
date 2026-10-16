# Media Studio publication queue (#803)

The publication queue is **review-first**. It consumes the producer's evidence-backed outcome (#797), a declarative episode/Short edit plan and fully rendered MP4 (#799), title/chapters/description from #801, and optionally the selected #802 JPEG thumbnail. It hands video upload to the **existing resumable YouTube uploader** (#178) and uses that uploader's checkpoints rather than inventing a second upload implementation.

## Operator workflow

Open **Media → Studio** and select a completed run. In **Publication queue**, choose **Full episode** or **Short / clip** and click **Prepare edited output + package**. The service creates a typed edit plan, requests its render from already-recorded broadcast footage, and creates a versioned factual content package. If the full broadcast MP4 is missing, render it in Media Library; editing never replays gameplay.

Review the package and generated options. For episodes, generate and select a JPEG in **Episode package & thumbnails** (#802). Then in **Publication queue**, choose a verified title, privacy level (private by default), and optional **not-before** date. Click **Queue for review**.

The candidate is durable under a run-scoped studio publication index. It can be:

- **queued**: meets quality criteria but no publication has been approved
- **blocked**: a hard evidence/asset/quality gate failed
- **rendering**: validated edited media is not available yet
- **approved**: explicit human approval, but still no upload
- **publishing**: the existing resumable YouTube uploader is active
- **published**: YouTube video ID and URL recorded
- **failed**: resumable uploader can be retried without regenerating gameplay
- **skipped**: producer's archive/do-not-publish decision, operator archive choice, or operator rejection

**Approve after gates** reevaluates the current source assets, checks title provenance and that an episode's selected thumbnail still matches the latest image manifest. The **Publish to YouTube** button is a *separate* explicit operator action. **Skip** does not delete gameplay or remove its recording. An archive-only or do-not-publish assessment produces a durable skipped candidate without reaching YouTube.

Do not use an edited package without an attached plan: earlier #801 packaging may be valid for thumbnails but insufficient for publication, by design. Short packages need their own portrait edit plan. One finished run may have both an episode and a Short candidate.

## Quality and safety boundaries

Hard rules in `media/publication`: verified source run/assessment identity, producer's permitted publication class, minimum two narrative beats, minimum average editorial score of 40 across novelty/drama/challenge/failure-recovery, maximum recent similarity of 85 (when present), verified title IDs, matching plan profile, ready nonempty edited MP4, plus a selected current thumbnail for episodes. The uploader always rechecks gates and source fingerprints immediately before upload. No input can override the producer classification. The server currently owns the policy; a future version can expose **admin-only**, audited configurable thresholds.

Uploads use the actual edition MP4's immutable S3 object key, **not** an ordinary raw replay. The packaging title and text are sourced from the verified #801 candidate. Short metadata is taken from the verified Shorts candidate, and relevant chapter timestamps are included in the video description. The selected JPEG is submitted using YouTube's standard thumbnails/set API **after** the existing video upload succeeds. A failed thumbnail transfer does not cause the video to be uploaded again; it is recorded as an advisory failure for review.

Idempotency: each candidate has a stable hash over the run, assessment, package, edit plan, format, title, thumbnail, privacy, and schedule. The existing #178 resumable uploader saves each candidate's upload session and provider ID under a unique `studio-{candidate-hash}` identifier. On retry, the existing session is resumed. GET reconciles publishing state with the uploader after a crash. Two different candidates **of the same format** cannot simultaneously publish (or both become published) for the same run. The private UI does not expose publishing to the spectator.

The existing direct replay publisher at `/v1/runs/{id}/publish/youtube` is retained for backwards compatibility and is explicitly labeled **legacy direct upload** in the operator UI. It is outside the Studio quality/approval queue and must not be used as a substitute for reviewed episodic publication. Migrating or disabling that independent legacy endpoint is a separate rollout decision.

## Scheduling, throughput and recovery

`not_before_unix` is a hard earliest-dispatch gate. It does **not** automatically start a video when that time arrives: default rollout requires an operator to press Publish after approval and the not-before time. A later automation can call the same protected dispatch route.

The publishing host allows one active YouTube upload at a time and stores a channel-start checkpoint in S3. `POKEPILOT_STUDIO_MIN_UPLOAD_INTERVAL` controls minimum spacing between distinct uploads; default `15m`, e.g. `1h` or `0s`. Retries of the same candidate are exempt. Rate-limiting and index updates are serialized by the replay worker's process mutex and durable across restarts, but **not an atomic S3 lease across distinct replicas**. Deploy only **one active replay publication worker** for this release. No service credentials are accepted in browser/API request bodies.

Server-side YouTube credentials reuse `POKEPILOT_YOUTUBE_CLIENT_ID`, `POKEPILOT_YOUTUBE_CLIENT_SECRET`, `POKEPILOT_YOUTUBE_REFRESH_TOKEN`; optionally `POKEPILOT_YOUTUBE_THUMBNAIL_UPLOAD_URL` overrides thumbnails/set for integration tests or trusted deployments. Provider session URI and OAuth access/refresh tokens are never present in the operator response.

## Private API

```http
GET  /v1/runs/{run}/studio/publications
POST /v1/runs/{run}/studio/publications
POST /v1/runs/{run}/studio/publications/{candidate}/approve
POST /v1/runs/{run}/studio/publications/{candidate}/reject
POST /v1/runs/{run}/studio/publications/{candidate}/publish
```

Request for creating an episode candidate:

```json
{
  "assessment_hash":"<sha-256>",
  "package_hash":"<sha-256>",
  "format":"episode",
  "title_id":"title-1",
  "thumbnail_id":"<current-selected-thumbnail-candidate-sha>",
  "privacy":"private",
  "not_before_unix":0
}
```

`plan_hash` is server-derived from the validated package, never arbitrary browser-supplied S3 storage. For an intentional archive outcome, specify `format:"archive"` and only `assessment_hash`. Non-public classes cannot become published, even if the operator asks. All endpoints belong to the **private operator relay**, not spectator routes.

The queue candidate and index are under `publications/studio/{sanitized-run}/`; the #178 uploader's existing resumable checkpoints remain under `publications/youtube/studio-{candidate}.json`. Outbound publication failures do not change a run's status, source footage, semantic timeline, content package, or edit plan.

## Tests

```bash
go test ./media/publication ./cmd/pokereplay ./cmd/pokeui
cd web && npm run typecheck && npm test
```

Coverage: review/skip and do-not-publish classifications, score/evidence/similarity gates, Short/episode plan profiles, missing thumbnail/media, identity hashing, approval/rejection and guarded retry transitions, due-date gating, durable indexed archive candidates, channel pacing, existing YouTube resumable uploader, and JPEG thumbnails/set handoff.
