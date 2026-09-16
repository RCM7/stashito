# api

Stashito speaks the OCI Distribution Spec. `<name>` is like
`dockerhub/library/postgres` — upstream alias first, then the ordinary
image path.

| Endpoint | Meaning |
|---|---|
| `GET /healthz` | JSON health check: `status` (`ok` or `degraded`), configured `upstreams`, `storage_path`, `storage_writable`; `503` when degraded |
| `GET /v2/` | OCI version check — `200` is the minimal liveness signal |
| `GET/HEAD /v2/<name>/manifests/<reference>` | Manifest by tag or digest |
| `GET/HEAD /v2/<name>/blobs/<digest>` | Blob (layer/config data) |
| `GET /metrics` | Prometheus metrics; only when `METRICS_ENABLED=true`, on `METRICS_PORT` when set |
