# api

Stashito speaks the OCI Distribution Spec. `<name>` is like
`dockerhub/library/postgres` — upstream alias first, then the ordinary
image path. With `MIRROR_UPSTREAM` set, a name with no alias prefix goes to
that upstream, which is what a Docker daemon configured with
`registry-mirrors` sends.

| Endpoint | Meaning |
|---|---|
| `GET /healthz` | JSON health check: `status` (`ok` or `degraded`), configured `upstreams`, `mirror_upstream`, `storage_path`, `storage_writable`; `503` when degraded |
| `GET /v2/` | OCI version check — `200` is the minimal liveness signal |
| `GET/HEAD /v2/<name>/manifests/<reference>` | Manifest by tag or digest |
| `GET/HEAD /v2/<name>/blobs/<digest>` | Blob (layer/config data); `507` when the blob does not fit in the storage filesystem's free space |
| `GET /metrics` | Prometheus metrics; only when `METRICS_ENABLED=true`, on `METRICS_PORT` when set |
