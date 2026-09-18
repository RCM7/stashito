# configuration

Configuration is env vars only. All variables are required unless marked
optional; there are no other defaults.

| Variable | Meaning |
|---|---|
| `PORT` | HTTP listen port |
| `STORAGE_PATH` | Filesystem root for cached data. Point it at a volume you're happy to let grow. |
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `text` or `json` (structured logs for machine parsing) |
| `TAG_TTL` | Go duration (e.g. `60s`). Tag manifests are served from cache without upstream revalidation within this window; after it expires the next request revalidates via upstream HEAD (digest compare). Digest manifests and blobs are immutable and never revalidated. |
| `UPSTREAM_<ALIAS>_HOST` | One per upstream registry host (e.g. `registry-1.docker.io`). At least one required. See [registries](/registries). |
| `UPSTREAM_<ALIAS>_USERNAME` / `_PASSWORD` | Optional basic-auth pair for private upstreams. For ECR hosts: AWS access key id + secret access key (unset = AWS default credential chain). |
| `METRICS_ENABLED` | Optional, default `false`. `true` exposes Prometheus metrics at `GET /metrics`. |
| `METRICS_PORT` | Optional. Serve `/metrics` on this separate port; 0/unset serves it on `PORT`. |

## behavior

- Cache hits are served from disk without contacting upstream.
- If upstream is unreachable, cached content is served stale.
- Layers stay cached even if the image is later deleted upstream.
