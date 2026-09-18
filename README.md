# Stashito

Pull-through cache for Docker images. Pull once — every later pull is served
from your own disk. No rate limits, no wasted bandwidth, no waiting on the
internet.

Works with any OCI registry: Docker Hub, GHCR, Quay, GAR, ACR, ECR.

Stashito is a caching registry proxy — a pull-through cache, or registry
mirror. It implements the OCI Distribution Spec, so docker, containerd and
podman pull through it unchanged. Use it to dodge Docker Hub rate limits
(upstream sees one pull per image), get LAN-speed pulls in CI and homelabs,
keep deploys working through registry outages or a dead uplink — including
air-gapped stretches — and keep images deployable after they vanish upstream:
a yanked tag, a deleted repo, a left-pad moment.

https://stashito.com

## Run it

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

Or a single command:

```sh
docker run -d --name stashito -p 8080:8080 \
  -v stashito_data:/data/stashito \
  -e PORT=8080 -e STORAGE_PATH=/data/stashito \
  -e LOG_LEVEL=info -e LOG_FORMAT=text -e TAG_TTL=60s \
  -e UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io \
  rcm7/stashito
```

```sh
docker pull localhost:8080/dockerhub/library/postgres:16
```

The first path segment names an upstream you configured
(`UPSTREAM_<ALIAS>_HOST`, plus `_USERNAME` / `_PASSWORD` for private
registries). Everything after it is the ordinary image path.
AWS ECR hosts are detected automatically; stashito fetches and rotates the
ECR authorization token itself, using `_USERNAME` / `_PASSWORD` as AWS
access keys or, when unset, the AWS default credential chain (IAM role,
IRSA).
`LOG_FORMAT` is `text` or `json`. `GET /healthz` reports status, configured
upstreams and storage writability as JSON.

## Metrics

Set `METRICS_ENABLED=true` to expose Prometheus metrics at `GET /metrics`.
By default they are served on the main port; set `METRICS_PORT` to serve
them on a separate port instead — recommended when the pull endpoint is
reachable by clients you don't want reading operational data.

Everything is prefixed `stashito_`: cache hits and misses per content type
(`stashito_cache_events_total`), blob bytes served from cache vs upstream
(`stashito_blob_bytes_served_total`), request counts and latency histograms,
upstream registry requests by host and status code, and storage gauges
(bytes, blobs, manifests — rescanned at most once a minute).

A ready-made Grafana dashboard lives at
[`grafana/dashboard.json`](grafana/dashboard.json): hit rate, bandwidth
saved, traffic, upstream health, latency and storage growth.

## Project policy

Free. Open source. Contributions welcome.

Bug reports and feature requests go through issues. Pull requests are
accepted. No timelines, no support commitments.

## License

[GNU AGPL-3.0](LICENSE). Free to use, copy, modify, and distribute. If you
run a modified version as a network service, you must offer its source to
the users of that service.
