# registries

Each upstream registry is one `UPSTREAM_<ALIAS>_HOST` variable. The
lowercased alias becomes the image-path prefix:

```sh
UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io
UPSTREAM_GHCR_HOST=ghcr.io

docker pull localhost:8080/dockerhub/library/redis:7
docker pull localhost:8080/ghcr/acme/api:1.4
```

For private upstreams add `UPSTREAM_<ALIAS>_USERNAME` and
`UPSTREAM_<ALIAS>_PASSWORD`.

## credentials per registry

| Registry | Username / password |
|---|---|
| Docker Hub | username + PAT |
| GHCR | username + PAT |
| Quay | username + robot token |
| Google Artifact Registry | `_json_key` + service account JSON |
| Azure ACR | service principal id + secret |

AWS ECR is not supported yet (rotating credentials).
