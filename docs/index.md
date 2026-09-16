# what is stashito

Stashito is a pull-through cache for Docker/OCI images: a caching registry
proxy that sits between your machines and upstream registries (Docker Hub,
GHCR, Quay, Google Artifact Registry, Azure ACR). It stores every manifest
and blob it fetches on local disk and serves later pulls from that cache.

It implements the OCI Distribution Spec — the same HTTP API every registry
speaks — so docker, containerd and podman pull through it unchanged. It ships
as a single Go binary, distributed as the Docker image `rcm7/stashito`.

## why run it

- **Rate limits** — Docker Hub caps anonymous pulls. Behind Stashito,
  upstream only ever sees one pull per image, however often your machines ask.
- **Speed** — cached layers come off local disk over your own network, at
  whatever speed your LAN moves.
- **Outages** — if upstream is unreachable, cached content is served stale.
  Deploys keep working through registry outages and dead uplinks.
- **Supply chain** — layers stay cached even if the image is later deleted
  upstream. What you pulled stays yours.

## how it works

A pull asks for a manifest; the manifest lists layers; the layers are blobs
on disk. Digest manifests and blobs are immutable and never revalidated.
Tags are the one thing that move: within `TAG_TTL` a cached tag is served
without checking upstream; after it expires the next request revalidates via
an upstream HEAD (digest compare) and only fetches what actually changed.

Next: [quickstart](/quickstart).
