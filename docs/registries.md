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
