# PostgreSQL control plane

PokePilot uses PostgreSQL as the sole production store for durable structured control-plane state while leaving large recordings, states and debug bundles in the existing S3-compatible artifact store.

The production rule is intentionally strict: **PostgreSQL owns one durable filesystem and may run on only one designated node/VM.** Do not use a Swarm named volume for database data.

## Option A: dedicated storage node inside the Swarm

Choose the node (a dedicated VM is fine) that owns the database disk and label it once:

```sh
docker node update --label-add pokepilot.db=true <node-name>
```

Make sure no other node has that label:

```sh
docker node ls -q | xargs -n1 docker node inspect --format '{{.Description.Hostname}} {{index .Spec.Labels "pokepilot.db"}}'
```

Mount a dedicated persistent data disk at `/data` on that labeled node, then create the PostgreSQL directory there. Confirm `/data` is a different filesystem from `/` before deploying:

```sh
findmnt -T /data
findmnt -T /

# postgres:18-alpine runs as uid/gid 70. The Debian image uses 999.
sudo install -d -o 70 -g 70 /data/pokepilot/postgres
```

Put the database settings in `.env` or `~/.config/pokepilot/env`. Use a URL-safe password or URL-encode it in the DSN:

```sh
FARM_DB_DIR=/data/pokepilot/postgres
POKEPILOT_DB_MEMORY_LIMIT=4G
POKEPILOT_DB_NAME=pokepilot
POKEPILOT_DB_USER=pokepilot
POKEPILOT_DB_PASSWORD=<strong-password>
POKEPILOT_DATABASE_URL=postgres://pokepilot:<url-encoded-password>@postgres:5432/pokepilot?sslmode=disable
```

Deploy the normal farm plus the database overlay:

```sh
set -a
[ -f "$HOME/.config/pokepilot/env" ] && . "$HOME/.config/pokepilot/env"
[ -f ./.env ] && . ./.env
set +a

docker stack deploy --resolve-image never \
  -c deploy/farm.yml \
  -c deploy/postgres.yml \
  pokefarm
```

`deploy/postgres.yml` enforces `replicas: 1`, the placement constraint `node.labels.pokepilot.db == true`, and a PostgreSQL memory limit that defaults to 4 GiB. Its bind mount is `${FARM_DB_DIR}:/var/lib/postgresql`; PostgreSQL 18 stores its version-specific `PGDATA` beneath that persistent root. Production should keep `FARM_DB_DIR=/data/pokepilot/postgres` so database growth cannot consume the VM root filesystem.

The database port is not published to the host. Other services reach it over the Swarm overlay network as `postgres:5432`.

PokéWall receives `POKEPILOT_MODEL_REGISTRY=postgres-env://POKEPILOT_DATABASE_URL`. That indirection is deliberate: the existing startup logger may print the registry source on an error, so the source contains only an environment-variable name and never database credentials.

## Option B: separate PostgreSQL VM outside the Swarm

Run PostgreSQL 14+ on a dedicated VM with its own persistent disk/backups and apply `deploy/postgres/001_control_plane.sql`. Configure PokéWall with the same secret-safe indirection:

```sh
POKEPILOT_MODEL_REGISTRY=postgres-env://POKEPILOT_DATABASE_URL
POKEPILOT_DATABASE_URL=postgres://pokepilot:<url-encoded-password>@<db-host>:5432/pokepilot?sslmode=require
```

In that topology you do **not** deploy `deploy/postgres.yml`; the farm only connects to the external database. Prefer a private network plus TLS/firewall rules that allow the PokéWall host and nothing else.

## What is stored there now

PostgreSQL is the production authority for run lifecycle/history, attempts,
experiment definitions and per-run immutable model/comparable identity, model
deployments/hosts, LLM exchanges, objective failures, issue
fingerprints/occurrences/links/outbox state, checkpoints/artifact metadata,
spectator controls, and dataset manifests.

`farm.LoadModelRegistry` still accepts a JSON file for development/backward
compatibility, but the production wall uses the PostgreSQL registry. The schema
seeds the deployments that were previously represented by
`deploy/models.example.json`; update revision, quantization and engine-version
fields to the exact deployed artifacts before treating results as reproducible
benchmarks.

SQLite, `state.json`, and `model-experiments.json` are legacy/local
compatibility paths only. They are not configured by the PostgreSQL production
overlay. Local finish/checkpoint files are disposable caches and may be absent
or read-only without changing run settlement or historical inspection.
Large immutable payload bytes remain in S3 rather than PostgreSQL.

## Backup

At minimum, back up both the Postgres database and the S3-compatible artifact bucket. A simple logical backup is:

```sh
pg_dump --format=custom --file=pokepilot.dump "$POKEPILOT_DATABASE_URL"
```

A database backup alone is not sufficient to recreate recordings or debug bundles because those intentionally remain in object storage.
