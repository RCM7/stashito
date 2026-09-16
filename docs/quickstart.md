# quickstart

One container, a volume, and at least one upstream.

## start stashito

```yaml
# docker-compose.yml
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

Or as a single command:

```sh
docker run -d --name stashito -p 8080:8080 \
  -v stashito_data:/data/stashito \
  -e PORT=8080 -e STORAGE_PATH=/data/stashito \
  -e LOG_LEVEL=info -e LOG_FORMAT=text -e TAG_TTL=60s \
  -e UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io \
  rcm7/stashito
```

All variables are required unless marked optional in
[configuration](/configuration) — if something is missing or mistyped,
Stashito tells you at startup instead of guessing.

## pull through it

```sh
docker pull localhost:8080/dockerhub/library/postgres:16
```

The first path segment names an upstream you configured: `dockerhub` is the
lowercased `<ALIAS>` from `UPSTREAM_DOCKERHUB_HOST`. Everything after it is
the ordinary image path on that upstream.

## verify

```sh
curl -f http://localhost:8080/healthz
```

Returns `status`, configured `upstreams`, `storage_path` and
`storage_writable` as JSON; `503` when degraded. Then pull the same image
twice: the first pull is a miss (fetched and cached), the second is served
from cache.
