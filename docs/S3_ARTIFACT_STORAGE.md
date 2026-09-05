# S3-compatible farm artifact storage

PokéFarm can store large durable artifacts such as GomeBoy `.gbrun` recordings in any path-style S3-compatible object store. This is intended for self-hosted RustFS/MinIO as well as S3-compatible hosted services.

The object store is optional. With no S3 variables set, the runner preserves the older behavior and attempts to carry `run.gbrun` inline in the finish report. Once any S3 variable is set, the runner treats object storage as intentionally configured: a bad configuration or failed upload is logged and the recording is omitted rather than silently embedding a large blob back into the finish JSON.

## Runner configuration

Set the following values in `.env` or `~/.config/pokepilot/env`; `make farm-up` already sources those files before interpolating `deploy/farm.yml`.

```sh
POKEPILOT_S3_ENDPOINT=http://nas.example.lan:9000
POKEPILOT_S3_BUCKET=pokepilot
POKEPILOT_S3_REGION=us-east-1
POKEPILOT_S3_ACCESS_KEY=replace-me
POKEPILOT_S3_SECRET_KEY=replace-me
POKEPILOT_S3_TIMEOUT=60s
```

The bucket must already exist. PokePilot deliberately does not create or administer buckets.

Recordings are written under browsable keys shaped like:

```text
runs/<sanitized-run-id>-<short-id-hash>/attempt-<n>/run.gbrun
```

The wall's finish dump then contains only metadata:

```json
{
  "name": "run.gbrun",
  "media_type": "application/octet-stream",
  "sha256": "...",
  "store": "s3",
  "bucket": "pokepilot",
  "object_key": "runs/.../attempt-1/run.gbrun",
  "size": 1234567
}
```

No S3 endpoint or credentials are written into the run artifact reference.

## Synology + RustFS

For a Synology NAS, let DSM/SHR/RAID own disk redundancy and mount one shared folder into RustFS. Do not build a second disk topology inside PokePilot.

A minimal Container Manager/Compose shape is:

```yaml
services:
  rustfs:
    image: rustfs/rustfs:latest
    restart: unless-stopped
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      RUSTFS_ACCESS_KEY: ${RUSTFS_ACCESS_KEY}
      RUSTFS_SECRET_KEY: ${RUSTFS_SECRET_KEY}
      RUSTFS_ADDRESS: ":9000"
      RUSTFS_CONSOLE_ADDRESS: ":9001"
    volumes:
      - /volume1/pokepilot-objects:/data
    command: /data
```

Create the `pokepilot` bucket once, then point the farm runners at the NAS endpoint. HDD-backed storage is appropriate for this workload: `.gbrun` files are written once and later read sequentially for deterministic replay or media rendering.

## Storage contract

Inline and remote artifacts share the same `farm.Artifact` wire type:

- inline: `data` is present and `store` is empty;
- remote: `data` is absent, `store` is `s3`, and bucket/key/size identify the object;
- `sha256` always describes the original artifact bytes;
- only inline bytes count toward the 24 MiB finish-report artifact budget.

The current S3 integration uploads `run.gbrun`. Existing periodic checkpoint and objective-failure artifacts remain inline for compatibility; they can move to the same store later without another wire-format change.

## Streaming and replay

`.gbrun` is the canonical deterministic recording, not a browser video stream. A later replay worker can read the object, verify it with GomeBoy, and derive MP4 or HLS objects in the same bucket. Live spectator HLS should use an asynchronous encoder/uploader so NAS or network latency never backpressures emulator execution.
