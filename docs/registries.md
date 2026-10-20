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

## mirror mode

Set `MIRROR_UPSTREAM` to one of the aliases and image names without an
alias prefix go to that upstream. That is the request shape docker,
containerd and podman send when Stashito is their registry mirror, so every
`docker pull postgres` in every Dockerfile, compose file and CI job is
served from the cache with no image reference changed.

```sh
UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io
MIRROR_UPSTREAM=dockerhub
```

Docker daemon, in `/etc/docker/daemon.json` (Docker mirrors Docker Hub
only, so the mirrored alias is the Docker Hub one):

```json
{ "registry-mirrors": ["http://stashito.internal:8080"] }
```

containerd, including Kubernetes, k3s and Kind, in
`/etc/containerd/certs.d/docker.io/hosts.toml`:

```toml
server = "https://registry-1.docker.io"

[host."http://stashito.internal:8080"]
  capabilities = ["pull", "resolve"]
```

podman, in `/etc/containers/registries.conf.d/stashito.conf`:

```toml
[[registry]]
location = "docker.io"

[[registry.mirror]]
location = "stashito.internal:8080"
insecure = true
```

A name with no prefix carries no registry, so one upstream is mirrored.
Other upstreams keep their alias prefix. A configured alias wins over the
mirror: `dockerhub/library/postgres` always means the `dockerhub` upstream,
so pick aliases that do not collide with namespaces on the mirrored
upstream. Both forms share one cache: a layer pulled as
`localhost:8080/dockerhub/library/postgres` is a hit for `postgres` through
the mirror.

## credentials per registry

| Registry | Username / password |
|---|---|
| Docker Hub | username + PAT |
| GHCR | username + PAT |
| Quay | username + robot token |
| Google Artifact Registry | `_json_key` + service account JSON |
| Azure ACR | service principal id + secret |
| AWS ECR | AWS access key id + secret access key, or leave both unset |

## AWS ECR

Hosts matching `<account>.dkr.ecr.<region>.amazonaws.com` are detected as
ECR automatically. Stashito calls `ecr:GetAuthorizationToken` itself and
refreshes the 12-hour token before it expires — no `docker login` cron, no
manually rotated secrets.

```sh
UPSTREAM_ECR_HOST=123456789012.dkr.ecr.eu-west-1.amazonaws.com
UPSTREAM_ECR_USERNAME=<aws access key id>
UPSTREAM_ECR_PASSWORD=<aws secret access key>

docker pull localhost:8080/ecr/my-team/api:1.4
```

With username and password unset, the AWS default credential chain is used
instead: `AWS_*` environment variables, shared config files, or the
instance/task role (EC2, ECS, EKS IRSA). The credentials need the
`ecr:GetAuthorizationToken` and `ecr:BatchGetImage`/`ecr:GetDownloadUrlForLayer`
permissions. FIPS (`dkr.ecr-fips`) and China (`.amazonaws.com.cn`) hosts are
detected too. Public ECR (`public.ecr.aws`) needs no credentials at all —
configure it like any anonymous upstream.
