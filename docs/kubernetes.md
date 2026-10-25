# kubernetes

The Helm chart runs one Stashito pod with a persistent volume for the cache.

## install

```sh
helm repo add stashito https://stashito.com/helm-charts
helm install stashito stashito/stashito
```

Each chart version matches a Stashito release and runs that image tag. The
default install caches Docker Hub under the `dockerhub` alias in a `20Gi`
volume.

## values

```yaml
upstreams:
  - alias: dockerhub
    host: registry-1.docker.io
  - alias: ghcr
    host: ghcr.io
    existingSecret: ghcr-creds
  - alias: quay
    host: quay.io
    username: robot
    password: token

mirrorUpstream: dockerhub
tagTTL: 60s
maxStorage: 45GB
maxStorageHeadroom: "10%"

persistence:
  size: 50Gi
  storageClass: ""

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

| Value | Meaning |
|---|---|
| `upstreams` | One entry per upstream registry: `alias` and `host`, plus credentials for private registries. See [registries](/registries). |
| `upstreams[].existingSecret` | Secret with `username` and `password` keys. Takes precedence over inline `username` / `password`. |
| `mirrorUpstream` | Alias served without a prefix. See [mirror mode](/registries#mirror-mode). |
| `tagTTL`, `logLevel`, `logFormat` | `TAG_TTL`, `LOG_LEVEL`, `LOG_FORMAT`. See [configuration](/configuration). |
| `maxStorage`, `maxStorageHeadroom` | `MAX_STORAGE`, `MAX_STORAGE_HEADROOM`. Keep `maxStorage` below `persistence.size` so the cap is hit before the volume fills. |
| `persistence` | `size`, `storageClass`, `accessModes`, or `existingClaim` to bring a volume. `enabled: false` uses an `emptyDir`. |
| `metrics.enabled` | Serves `/metrics` on `metrics.port` (default `9090`); `serviceMonitor.enabled` adds a Prometheus Operator `ServiceMonitor`. |
| `service`, `ingress` | `ClusterIP` on port `8080` by default. Set `service.type: NodePort` for nodes to reach it on a fixed port. |
| `extraEnv` | Extra env vars for the container, e.g. AWS settings for ECR. |

`helm show values stashito/stashito` prints every value with its default.

## nodes pulling through it

To make the kubelet pull through Stashito, every node's containerd needs a
mirror entry pointing at an address the node can reach. A `NodePort` service
works without DNS:

```sh
helm install stashito stashito/stashito \
  --set mirrorUpstream=dockerhub \
  --set service.type=NodePort --set service.nodePort=30500
```

Then on each node, `/etc/containerd/certs.d/docker.io/hosts.toml`:

```toml
server = "https://registry-1.docker.io"

[host."http://10.0.0.11:30500"]
  capabilities = ["pull", "resolve"]
```

`10.0.0.11` stands for any node's IP; a `NodePort` answers on every node.
Pods keep their image names unchanged. A node pulling while Stashito itself
is down falls back to `server`.

## behavior

- The pod runs as non-root with a read-only root filesystem; the volume is
  made writable via `fsGroup`.
- One replica, `Recreate` strategy: the cache volume is `ReadWriteOnce`.
- Readiness is `GET /healthz` (fails when the cache is not writable), liveness
  is `GET /v2/`.
- `helm uninstall` keeps the cache volume. Delete the PVC to drop the cache.
