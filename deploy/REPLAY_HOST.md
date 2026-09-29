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

Replay remains a standalone Docker container because the Swarm service API
does not pass through `/dev/dri` in this deployment. The VM joins Swarm as a
worker so its container can use the attachable `pokefarm_gpu` overlay and the
`replay` DNS alias expected by the UI and spectator. Keep this node **active**:
pausing or draining it removes the overlay anchor after a reboot. The anchor
is deployed from `deploy/replay-overlay-anchor.yml` as the `pokefarm-render`
stack. It uses no GPU and only keeps the network present. Verify it is running
before starting the sidecar:

```sh
docker stack deploy -c deploy/replay-overlay-anchor.yml pokefarm-render
docker service ps pokefarm-render_overlay-anchor
# On the render VM:
docker network inspect pokefarm_gpu --format '{{.Name}} {{.Attachable}}'
```

Provision `/opt/pokefarm/roms/pokemon_red.gb` and root-readable-only
`/opt/pokefarm/replay.env` on the VM through the existing private operations
channel. Never put ROM or S3 credentials in Git or the image. Install
`deploy/replay-pull.sh` as `/usr/local/sbin/pokefarm-replay-pull` and the
`deploy/pokefarm-replay-pull.{service,timer}` units under `/etc/systemd/system`.
Enable the timer and run the service once. The bootstrap pulls the published
image digest, extracts its matching sidecar definition, and reconciles the
container. Disable the old worker's replay timer before leaving the new alias
in production.

```sh
systemctl daemon-reload
systemctl enable --now pokefarm-replay-pull.timer
systemctl start pokefarm-replay-pull.service
docker exec pokefarm-replay wget -qO- http://127.0.0.1:8080/healthz
```

`/healthz` must report `status: ok`, `s3_configured: true`, `vaapi: true`, and
`encoder: h264_vaapi`. From an operator UI container, request
`http://replay:8080/healthz` to verify overlay DNS and the full network path.
After a reboot, also verify the anchor task, sidecar container state, and timer.
If Docker starts replay before the overlay exists, the timer must recreate the
exited container once the anchor has restored the network.


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
