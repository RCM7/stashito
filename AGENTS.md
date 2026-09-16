# Stashito

Pull-through cache for Docker/OCI images. A caching registry proxy: implements
the OCI Distribution Spec, sits between clients and upstream registries
(Docker Hub, GHCR, Quay, Google Artifact Registry, Azure ACR), stores every
manifest and blob it fetches on local disk, and serves later pulls from that
cache. Ships as the Docker image `rcm7/stashito`.

## Run

```yaml
services:
  stashito:
    image: rcm7/stashito
    environment:
      PORT: "8080"
      STORAGE_PATH: "/data/stashito"
      LOG_LEVEL: "info"
      LOG_FORMAT: "text"
      TAG_TTL: "60s"
      UPSTREAM_DOCKERHUB_HOST: "registry-1.docker.io"
    volumes:
      - stashito_data:/data/stashito
    ports:
      - "8080:8080"

volumes:
  stashito_data:
```

## Pull through it

```sh
docker pull localhost:8080/dockerhub/library/postgres:16
```

The first path segment is the lowercased `<ALIAS>` from an
`UPSTREAM_<ALIAS>_HOST` variable. Everything after it is the ordinary image
path on that upstream.

## Configuration

All env vars are required unless marked optional. There are no other defaults.

| Variable | Meaning |
|---|---|
| `PORT` | HTTP listen port |
| `STORAGE_PATH` | Filesystem root for cached data |
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `text` or `json` (structured logs for machine parsing) |
| `TAG_TTL` | Go duration (e.g. `60s`). Tag manifests are served from cache without upstream revalidation within this window; after it expires the next request revalidates via upstream HEAD (digest compare). Digest manifests and blobs are immutable and never revalidated. |
| `UPSTREAM_<ALIAS>_HOST` | One per upstream registry host (e.g. `registry-1.docker.io`). At least one required. |
| `UPSTREAM_<ALIAS>_USERNAME` / `_PASSWORD` | Optional basic-auth pair for private upstreams. Docker Hub / GHCR / Quay: username + PAT or robot token. GAR: `_json_key` + service account JSON. ACR: service principal id + secret. |
| `METRICS_ENABLED` | Optional, default `false`. `true` exposes Prometheus metrics at `GET /metrics` (`stashito_` prefix). |
| `METRICS_PORT` | Optional. Serve `/metrics` on this separate port; 0/unset serves it on `PORT`. |

AWS ECR is not supported yet (rotating credentials).

## Behavior

- Cache hits are served from disk without contacting upstream.
- If upstream is unreachable, cached content is served stale.
- Layers stay cached even if the image is later deleted upstream.

## Verify a running instance

```sh
curl -f http://localhost:8080/healthz
```

Returns JSON: `status` (`ok` or `degraded`), configured `upstreams`,
`storage_path` and `storage_writable`; `503` when degraded. `GET /v2/`
returning `200` is the minimal OCI liveness check. Pull the same image twice:
the first pull is a miss (fetched and cached), the second is served from
cache.

## Endpoints

- `GET /healthz` — JSON health check as above
- `GET /v2/` — OCI version check
- `GET/HEAD /v2/<name>/manifests/<reference>` — manifest by tag or digest
- `GET/HEAD /v2/<name>/blobs/<digest>` — blob (layer/config data)
- `GET /metrics` — Prometheus metrics; only when `METRICS_ENABLED=true`, on `METRICS_PORT` when set

Where `<name>` is like `dockerhub/library/postgres`.

## Development

Go server, Clean Architecture. Always use Makefile targets from the repo root:

```sh
make build-local     # Build server binary
make run-local       # Run server (with .env)
make test            # Run all tests
make build           # Docker build
make push            # Push Docker image
```

Clean Architecture, under `server/`:

- `cmd/` — Cobra CLI
- `internal/app/server/config/` — Config via go-envconfig
- `internal/app/server/domain/` — Entities + interfaces (gateway, repository)
- `internal/app/server/interactor/` — Cache-through business logic
- `internal/app/server/interface/handler/` — Gin HTTP handlers for OCI endpoints
- `internal/app/server/interface/storage/fs/` — Filesystem cache
- `internal/app/server/interface/upstream/` — Generic OCI registry client (challenge auth)
- `internal/app/server/http/rest/` — Router wiring
- `internal/app/server/metrics/` — Prometheus instrumentation: cache hit/miss in the interactors, upstream requests in interface/upstream, HTTP totals/latency via Gin middleware, storage gauges from a custom collector that walks `STORAGE_PATH` at most once a minute. Grafana dashboard: `grafana/dashboard.json`.

## Git commits

Do NOT include "Co-Authored-By" lines or attribution in commit messages.

## Shipping features

Every new feature ships with a pass over the public surfaces in the same
change: docs/ (the VitePress site served at stashito.com/docs), README.md,
and the Docker Hub description (source in server/docker/DOCKERHUB.md, pushed
to hub.docker.com via the API). The landing page and llms.txt are maintained
outside this repo; automated CI builds docs/ from here and deploys the full
site (landing plus /docs) to stashito.com.

## License

GNU AGPL-3.0. Free to use, copy, modify, and distribute. Running a modified
version as a network service requires offering its source to that service's
users.
