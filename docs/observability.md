# observability

## logs

`LOG_FORMAT=text` for humans, `json` for structured logs your tooling (or
your agent) can parse. Levels via `LOG_LEVEL`: `debug`, `info`, `warn`,
`error`.

## metrics

Set `METRICS_ENABLED=true` to expose Prometheus metrics at `GET /metrics`
(`stashito_` prefix): cache hits and misses, upstream requests, HTTP totals
and latency, storage size, and with `MAX_STORAGE` set the cap
(`stashito_storage_max_bytes`) plus eviction counters
(`stashito_evictions_total` by type, `stashito_evicted_bytes_total`). Set `METRICS_PORT` to serve them on a separate
port — keep it off the port you expose to Docker.

A ready-made Grafana dashboard ships in the repo at `grafana/dashboard.json`.

## health

`GET /healthz` reports `status`, configured `upstreams`, `mirror_upstream`,
`storage_path` and `storage_writable` as JSON, and returns `503` when degraded.
