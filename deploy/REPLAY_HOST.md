# Dedicated replay renderer

The production replay renderer runs in VM 9001 (`vm-pokefarm-render-01`,
`192.168.50.203`) on pve02. It has 4 vCPUs, 6 GiB RAM, and a 50 GiB disk. The
Intel Iris Xe iGPU (`0000:00:02.0`) is passed through; the VM is set to start on
host boot. Standard VGA remains available for the Proxmox console. A console
left showing kernel messages does not establish a boot failure: check SSH,
`systemctl is-system-running`, and the render device inside the guest.

The guest must show `i915` bound to the Intel GPU and `/dev/dri/renderD128`.
The farm image contains FFmpeg and the Intel media driver; a host desktop or
host FFmpeg installation is unnecessary. A hardware encode is the positive GPU
check:

```sh
docker run --rm --device /dev/dri/renderD128 -e LIBVA_DRIVER_NAME=iHD \
  ghcr.io/maestroi/pokepilot:latest ffmpeg -hide_banner -loglevel error \
  -init_hw_device vaapi=va:/dev/dri/renderD128 \
  -f lavfi -i color=c=black:s=320x240:r=30 \
  -vf format=nv12,hwupload -c:v h264_vaapi -frames:v 30 -f null -
```

Replay is a standalone Docker container because the Swarm service API does not
pass through `/dev/dri` in this deployment. The VM is a dedicated render box and
is **not** a Swarm node: as a worker it attracted farm tasks (runners, ui, …)
that starved the encoder. If it was joined before, remove it:

```sh
# On the render VM:
docker swarm leave
# On a manager:
docker node rm vm-pokefarm-render-01
docker stack rm pokefarm-render   # the old overlay anchor, no longer needed
```

The sidecar publishes `8080` on the VM. The farm stack's `ui` and `spectator`
use `-replay http://192.168.50.203:8080`, and the stack publishes pokewall on
`18080` so replay can reach it. pokewall has no authentication of its own, so
that port must stay on the private LAN.

Provision `/opt/pokefarm/roms/pokemon_red.gb` and root-readable-only
`/opt/pokefarm/replay.env` on the VM through the existing private operations
channel. Never put ROM or S3 credentials in Git or the image. Use
[`deploy/replay.env.example`](./replay.env.example) as the canonical variable
reference and starting capacity envelope; replace the placeholder S3 values and
keep the host copy mode `0600`. Install
`deploy/replay-pull.sh` as `/usr/local/sbin/pokefarm-replay-pull` and the
`deploy/pokefarm-replay-pull.{service,timer}` units under `/etc/systemd/system`.
Write `/etc/default/pokefarm-replay` with
`FARM_REPLAY_WALL=http://192.168.50.100:18080` (any Swarm node works through the
ingress mesh). Enable the timer and run the service once. The bootstrap pulls the published
image digest, extracts its matching sidecar definition, and reconciles the
container.

```sh
systemctl daemon-reload
systemctl enable --now pokefarm-replay-pull.timer
systemctl start pokefarm-replay-pull.service
docker exec pokefarm-replay wget -qO- http://127.0.0.1:8080/healthz
```

`/healthz` must report `status: ok`, `s3_configured: true`, `vaapi: true`, and
`encoder: h264_vaapi`.

Updates are not Swarm-driven: the timer pulls `:latest` every two minutes, but
it only replaces a healthy container when `/healthz` reports
`active_renders: 0`. A new image published mid-encode is logged as `deferring
update` and applied on the first idle tick. A stopped or unhealthy container is
replaced immediately. From an operator UI container, request
`http://192.168.50.203:8080/healthz` to verify the full network path. After a
reboot, verify the sidecar container state and timer; the timer recreates an
exited container.


## Long replay scratch and segment cache

Completed replays are rendered as deterministic frame segments. The default
segment window is five minutes and can be changed with
`POKEPILOT_REPLAY_SEGMENT_SECONDS` (10–3600 seconds). Each completed segment is
validated with `ffprobe`, uploaded to S3, and removed from local scratch before
the next missing segment is encoded. On restart, cached segments are probed
again; a zero-length, truncated, or otherwise invalid MP4 is treated as missing
and regenerated from the authoritative `.gbrun`.

Final assembly does **not** download every cached segment to the replay VM.
FFmpeg reads the ordered segment list directly from short-lived presigned S3 GET
URLs and stream-copies them into one local final MP4, which is validated before
upload. Presigned URLs are written only to the temporary concat manifest and
must never be logged.

The expected scratch envelope for a bounded render is therefore approximately:

- the source `.gbrun` and replay ROM working files for each attempt;
- at most one actively encoded segment (plus one raw segment while the broadcast
  compositor is producing its corresponding composed segment);
- one final assembled MP4 during the assembly/upload stage;
- small manifests/overlay PNGs.

Scratch no longer scales as "all rendered segments + final MP4". Segment
artifacts are durable derived cache entries in S3 and can be regenerated from
the source recording if an individual object fails validation.


## Render capacity and admission control

The copy-paste configuration reference is
[`deploy/replay.env.example`](./replay.env.example). The table below explains
what each capacity knob controls and when to change it.

| Variable | Default / recommended start | What it bounds | Raise it when | Lower it when |
| --- | --- | --- | --- | --- |
| `POKEPILOT_REPLAY_MAX_JOBS` | `2` | Claimed offline render jobs | Jobs queue while CPU/RAM/scratch still have headroom | Multiple jobs push host memory/disk pressure too high |
| `POKEPILOT_REPLAY_WORKERS` | `3` | Concurrent emulator + encoder segment pipelines | GPU/CPU is underutilized during long renders | RSS, FDs, encoder load, or contention rises |
| `POKEPILOT_REPLAY_MIN_SCRATCH_BYTES` | `1073741824` (1 GiB) | Minimum free scratch before claiming new work | Only after measuring worst-case assembly size and preserving safety margin | Increase this value—not decrease it—when final MP4s or temp work get larger |
| `POKEPILOT_REPLAY_SCRATCH_DIR` | `/tmp/pokepilot-replay` | Location of transient job directories | Move to a larger/faster dedicated volume | N/A; choose a filesystem with enough free space |
| `POKEPILOT_REPLAY_JOB_TIMEOUT` | `8h` | Whole claimed job lifetime | Legitimate very long jobs hit timeout despite healthy progress | Hung/stuck jobs occupy admission slots too long |
| `POKEPILOT_REPLAY_SEGMENT_TIMEOUT` | `2h` | One segment encoder/compositor lifetime | Legitimate slow software renders exceed the bound | A stuck segment holds an encoder worker too long |
| `POKEPILOT_REPLAY_SEGMENT_SECONDS` | `300` | Source duration per resumable segment | Segment overhead dominates and retries are cheap | Failures/restarts redo too much work per segment |

Offline replay work is admitted separately from live spectator media. Live MJPEG
sessions keep their bounded subscriber queues and frame dropping behavior and do
not consume offline render-job slots.

The replay worker uses these capacity knobs:

- `POKEPILOT_REPLAY_MAX_JOBS` — maximum claimed offline render jobs at once
  (default 2). Excess durable jobs remain `queued` in pokewall.
- `POKEPILOT_REPLAY_WORKERS` — maximum attempt/segment workers that can hold an
  emulator + encoder pipeline at once (default 3).
- `POKEPILOT_REPLAY_MIN_SCRATCH_BYTES` — free-space floor required before a
  new offline job is claimed (default 1 GiB). Low space leaves work queued and
  does not affect ready artifacts or API health.
- `POKEPILOT_REPLAY_SCRATCH_DIR` — dedicated temp root (default
  `$TMPDIR/pokepilot-replay`). Orphan `job-*` directories are removed when
  the replay worker starts.
- `POKEPILOT_REPLAY_JOB_TIMEOUT` — maximum lifetime of one claimed render job
  (Go duration, default `8h`).
- `POKEPILOT_REPLAY_SEGMENT_TIMEOUT` — maximum lifetime of an individual
  segment encoder/compositor pipeline (Go duration, default `2h`).

For the current 4-vCPU/6-GiB render VM, start conservatively with the defaults.
Raise `POKEPILOT_REPLAY_WORKERS` or `POKEPILOT_REPLAY_MAX_JOBS` only after
watching memory, file descriptors, scratch free space, and encoder utilization
under long multi-attempt renders.

`/healthz` exposes the worker envelope without credentials: offline job limit
and active count, queued depth/reasons, active segment workers, active encoder
processes, scratch free/use/floor, oldest render age, failures/retries/completed
renders, rendered bytes and average throughput. A healthy but saturated worker
continues to return `status: ok`; saturation is represented by queued jobs
rather than by spawning additional work.

Cancellation is durable in pokewall. Active workers heartbeat frequently and
cancel their render context when the durable job is cancelled; the replay API
also exposes `POST /v1/runs/{id}/replay/cancel?mode=...` for immediate local
propagation. A cancelled or timed-out subprocess cannot publish a ready artifact
because assembly/upload only runs after the segment has completed and passed
validation.


## Event-driven highlight reels

Completed runs with a `media-timeline.json` can produce a short deterministic
highlight reel without a second event-detection system. The highlight planner
selects versioned semantic events, expands them with pre/post-roll, merges
overlapping or nearby windows, ranks them deterministically, and keeps the
stitched reel under a duration budget.

The built-in policy currently recognizes run completion/failure, Champion and
Elite Four events, badges and gym battles, rival battles, rare/important
catches, evolutions, blackouts, planner/failure recovery, milestones, and
checkpoints. Event types that are not emitted by a game yet are harmless and
become active automatically once that game's timeline starts producing them.

Configuration lives in `deploy/replay.env.example`:

- `POKEPILOT_HIGHLIGHT_TARGET_SECONDS` defaults to 480 seconds.
- `POKEPILOT_HIGHLIGHT_MERGE_GAP_SECONDS` defaults to 3 seconds.
- `POKEPILOT_HIGHLIGHT_POLICY_JSON` can replace the complete policy for
  experiments; production policy changes should increment the policy version.

Each selected window is rendered through the normal broadcast compositor and is
cached as an independently resumable derived clip. The final reel is assembled
from those cached clips, then a JSON manifest is persisted beside it with the
plan hash, source frame ranges, event references, and clip artifact keys.
Changing the source recording, media timeline, renderer/profile, or highlight
plan produces a different canonical artifact identity.

Operator endpoints can inspect or request generation under
`/v1/runs/{id}/highlights/...`. The public spectator exposes only read-only
status/video/manifest routes for runs already admitted to the public replay
archive; it cannot trigger a render.
