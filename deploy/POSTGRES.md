# PostgreSQL control plane

PokePilot can keep operator-controlled model deployment configuration in PostgreSQL while leaving large recordings, screenshots and debug bundles in the existing S3-compatible artifact store.

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

The first migration moves the selectable model deployment registry into PostgreSQL. `farm.LoadModelRegistry` accepts a JSON file (development/backward compatibility), a PostgreSQL DSN, or the recommended `postgres-env://ENV_NAME` source, so the existing `/v1/models` and experiment code does not need a second configuration format.

The initial schema seeds the deployments that were previously represented by `deploy/models.example.json`. Update their revision, quantization and engine-version fields to the exact deployed artifacts before using results as reproducible benchmarks.

The current SQLite run-history catalog and `model-experiments.json` remain in the existing PokéWall bind mount in this first migration. They should be migrated next, after the Postgres deployment is proven stable; large artifacts should remain in S3 rather than moving into PostgreSQL.

## Backup

At minimum, back up both the Postgres database and the S3-compatible artifact bucket. A simple logical backup is:

```sh
pg_dump --format=custom --file=pokepilot.dump "$POKEPILOT_DATABASE_URL"
```

A database backup alone is not sufficient to recreate recordings or debug bundles because those intentionally remain in object storage.
