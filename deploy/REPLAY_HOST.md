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
channel. Never put ROM or S3 credentials in Git or the image. Install
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
