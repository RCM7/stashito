# configuration

Configuration is env vars only. All variables are required unless marked
optional; there are no other defaults.

| Variable | Meaning |
|---|---|
| `PORT` | HTTP listen port |
| `STORAGE_PATH` | Filesystem root for cached data. Grows without bound unless `MAX_STORAGE` is set. |
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `text` or `json` (structured logs for machine parsing) |
| `TAG_TTL` | Go duration (e.g. `60s`). Tag manifests are served from cache without upstream revalidation within this window; after it expires the next request revalidates via upstream HEAD (digest compare). Digest manifests and blobs are immutable and never revalidated. |
| `UPSTREAM_<ALIAS>_HOST` | One per upstream registry host (e.g. `registry-1.docker.io`). At least one required. See [registries](/registries). |
| `UPSTREAM_<ALIAS>_USERNAME` / `_PASSWORD` | Optional basic-auth pair for private upstreams. For ECR hosts: AWS access key id + secret access key (unset = AWS default credential chain). |
| `MIRROR_UPSTREAM` | Optional. Alias that serves image names with no alias prefix, so `docker pull postgres` works with Stashito configured as a registry mirror. See [mirror mode](/registries#mirror-mode). |
| `METRICS_ENABLED` | Optional, default `false`. `true` exposes Prometheus metrics at `GET /metrics`. |
| `METRICS_PORT` | Optional. Serve `/metrics` on this separate port; 0/unset serves it on `PORT`. |
| `MAX_STORAGE` | Optional. Cap on cached bytes, e.g. `500MB`, `50GB` or `2TB` (1024-based, as `df -h` reports; a bare number is bytes). Unset means unlimited. When a write would push the cache past the cap, the least recently pulled manifests and blobs are deleted first, inline, so the cap holds. |

## behavior

- Cache hits are served from disk without contacting upstream.
- If upstream is unreachable, cached content is served stale.
- Layers stay cached even if the image is later deleted upstream.
- With `MAX_STORAGE` set, eviction runs inline on writes and counts bytes
  still downloading, so concurrent pulls do not overshoot the cap. Last
  access survives restarts (persisted to the file mtime at most once a
  minute per entry). A blob larger than the cap on its own is still served
  but is the first thing the next write evicts.
- A blob that does not fit in the free space of the storage filesystem is
  refused with `507 Insufficient Storage` before any byte is written, so a
  pull can never fill the disk. With `MAX_STORAGE` set, eviction runs first
  and the blob is refused only if evicting everything would still not make
  room.
