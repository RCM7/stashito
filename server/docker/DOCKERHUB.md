# Stashito

Free, open-source pull-through cache for Docker images. Pull once — every
later pull is served from your own disk. No rate limits, no wasted
bandwidth, no waiting on the internet.

Works with any OCI registry: Docker Hub, GHCR, Quay, GAR, ACR, ECR.

Stashito is a caching registry proxy — a pull-through cache, or registry
mirror. It implements the OCI Distribution Spec, so docker, containerd and
podman pull through it unchanged. Use it to dodge Docker Hub rate limits
(upstream sees one pull per image), get LAN-speed pulls in CI and homelabs,
keep deploys working through registry outages or a dead uplink, and keep
images deployable after they vanish upstream: a yanked tag, a deleted repo,
a left-pad moment.

https://stashito.com — full docs at https://stashito.com/llms.txt

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

## Metrics

Optional Prometheus metrics: set `METRICS_ENABLED=true` to expose
`GET /metrics` — cache hits and misses, upstream requests, HTTP latency,
storage size. `METRICS_PORT` serves them on a separate port. A ready-made
Grafana dashboard ships in the repo.

## License

GNU AGPL-3.0. Free to use, copy, modify, and distribute. If you run a
modified version as a network service, you must offer its source to the
users of that service.
