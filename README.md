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
    image: rcm7/stashito:0
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

The image is published for `linux/amd64`, `linux/arm64`, `linux/arm/v7`,
`linux/arm/v6`, `linux/386`, `linux/ppc64le`, `linux/s390x` and
`linux/riscv64`; docker picks the one matching the host.

Every merge to master is a release. `0` and `latest` track the newest one,
`0.3` and `0.3.1` pin one. Releases are listed at
https://github.com/RCM7/stashito/releases.

Or a single command:

```sh
docker run -d --name stashito -p 8080:8080 \
  -v stashito_data:/data/stashito \
  -e PORT=8080 -e STORAGE_PATH=/data/stashito \
  -e LOG_LEVEL=info -e LOG_FORMAT=text -e TAG_TTL=60s \
  -e UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io \
  rcm7/stashito:0
```

On Kubernetes, with the Helm chart:

```sh
helm repo add stashito https://stashito.com/helm-charts
helm install stashito stashito/stashito
```

Values and node mirror setup: https://stashito.com/docs/kubernetes.

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
Set `MIRROR_UPSTREAM=dockerhub` and image names need no prefix: point the
Docker daemon at Stashito with `"registry-mirrors": ["http://stashito:8080"]`
in `daemon.json` (or containerd `hosts.toml`, or a podman mirror) and every
`docker pull postgres` is served from the cache with no image reference
changed. Full examples at https://stashito.com/docs/registries#mirror-mode.
`LOG_FORMAT` is `text` or `json`. `GET /healthz` reports status, configured
upstreams and storage writability as JSON.

The cache grows without bound unless you set `MAX_STORAGE` (e.g. `50GB`,
`2TB`). With a cap, the least recently pulled manifests and blobs are
evicted inline as new ones are written, so disk use never exceeds it. Each
eviction frees `MAX_STORAGE_HEADROOM` (default `10%`) below the cap so a
full cache evicts in batches rather than on every write.
Either way a blob that does not fit in the filesystem's free space is
refused with `507 Insufficient Storage` instead of filling the disk.

## Metrics

Set `METRICS_ENABLED=true` to expose Prometheus metrics at `GET /metrics`.
By default they are served on the main port; set `METRICS_PORT` to serve
them on a separate port instead — recommended when the pull endpoint is
reachable by clients you don't want reading operational data.

Everything is prefixed `stashito_`: cache hits and misses per content type
(`stashito_cache_events_total`), blob bytes served from cache vs upstream
(`stashito_blob_bytes_served_total`), request counts and latency histograms,
upstream registry requests by host and status code, storage gauges
(bytes, blobs, manifests — rescanned at most once a minute), and eviction
counters (`stashito_evictions_total`, `stashito_evicted_bytes_total`) next
to the configured cap (`stashito_storage_max_bytes`).

A ready-made Grafana dashboard lives at
[`grafana/dashboard.json`](grafana/dashboard.json): hit rate, bandwidth
saved, traffic, upstream health, latency and storage growth against the cap.

## Project policy

Free. Open source. Contributions welcome.

Bug reports and feature requests go through issues. Pull requests are
accepted. No timelines, no support commitments.

## License

[GNU AGPL-3.0](LICENSE). Free to use, copy, modify, and distribute. If you
run a modified version as a network service, you must offer its source to
the users of that service.
