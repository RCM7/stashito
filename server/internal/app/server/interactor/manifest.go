package interactor

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
	"github.com/RCM7/stashito/server/internal/app/server/domain/gateway"
	"github.com/RCM7/stashito/server/internal/app/server/domain/repository"
	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

type GetManifestInteractor func(ctx context.Context, name string, reference string) (*entity.CachedManifest, error)
type HeadManifestInteractor func(ctx context.Context, name string, reference string) (*entity.ManifestMetadata, error)

func GetManifest(cache *fs.CacheRepository, upstream gateway.UpstreamGateway, registries entity.Registries, tagTTL time.Duration) GetManifestInteractor {
	return func(ctx context.Context, name string, reference string) (*entity.CachedManifest, error) {
		alias, repo, err := entity.ParseImageName(name)
		if err != nil {
			return nil, err
		}

		up, err := registries.Resolve(alias)
		if err != nil {
			return nil, err
		}
		registryHost := up.Host

		lockKey := registryHost + "/" + repo + "/" + reference
		unlock := cache.Lock(lockKey)
		defer unlock()

		return getManifestCacheThrough(ctx, cache, upstream, registryHost, repo, reference, tagTTL)
	}
}

func HeadManifest(cache *fs.CacheRepository, upstream gateway.UpstreamGateway, registries entity.Registries, tagTTL time.Duration) HeadManifestInteractor {
	return func(ctx context.Context, name string, reference string) (*entity.ManifestMetadata, error) {
		alias, repo, err := entity.ParseImageName(name)
		if err != nil {
			return nil, err
		}

		up, err := registries.Resolve(alias)
		if err != nil {
			return nil, err
		}
		registryHost := up.Host

		lockKey := registryHost + "/" + repo + "/" + reference
		unlock := cache.Lock(lockKey)
		defer unlock()

		// For digest references, just check cache
		if isDigest(reference) {
			if meta, ok := cache.ManifestExists(registryHost, repo, reference); ok {
				return meta, nil
			}
		}

		// For tags, serve from cache while within the TTL window
		if !isDigest(reference) {
			if cached, ok := cache.ManifestExists(registryHost, repo, reference); ok && withinTTL(cached, tagTTL) {
				return cached, nil
			}

			// TTL expired — check upstream freshness
			upstreamMeta, err := upstream.HeadManifest(ctx, registryHost, repo, reference)
			if err == nil {
				// Check if our cached tag points to the same digest
				if cached, ok := cache.ManifestExists(registryHost, repo, reference); ok {
					if cached.Digest == upstreamMeta.Digest {
						refreshManifestTTL(cache, registryHost, repo, reference)
						return cached, nil
					}
				}
				return upstreamMeta, nil
			}
			// Upstream failed — fall through to serve stale cache
			slog.Warn("upstream HEAD failed, checking stale cache", "error", err)
		}

		if meta, ok := cache.ManifestExists(registryHost, repo, reference); ok {
			return meta, nil
		}

		// Nothing cached and upstream failed
		return upstream.HeadManifest(ctx, registryHost, repo, reference)
	}
}

func getManifestCacheThrough(
	ctx context.Context,
	cache repository.CacheRepository,
	upstream gateway.UpstreamGateway,
	registryHost string,
	repository string,
	reference string,
	tagTTL time.Duration,
) (*entity.CachedManifest, error) {
	// Digest references are immutable — serve from cache if available
	if isDigest(reference) {
		if manifest, err := cache.GetManifest(registryHost, repository, reference); err == nil {
			slog.Info("serving manifest from cache (digest)", "repository", repository, "reference", reference)
			metrics.CacheHit("manifest")
			return manifest, nil
		}
		// Not cached — fetch from upstream
		return fetchAndCacheManifest(ctx, cache, upstream, registryHost, repository, reference)
	}

	// Tag reference — serve from cache while within the TTL window, no upstream traffic
	if cachedMeta, ok := cache.ManifestExists(registryHost, repository, reference); ok && withinTTL(cachedMeta, tagTTL) {
		if manifest, err := cache.GetManifest(registryHost, repository, reference); err == nil {
			slog.Info("serving manifest from cache (tag within TTL)", "repository", repository, "tag", reference, "digest", cachedMeta.Digest)
			metrics.CacheHit("manifest")
			return manifest, nil
		}
	}

	// TTL expired or not cached — revalidate against upstream
	upstreamMeta, headErr := upstream.HeadManifest(ctx, registryHost, repository, reference)
	if headErr == nil {
		// We know the current digest for this tag — check if our cache matches
		if cachedMeta, ok := cache.ManifestExists(registryHost, repository, reference); ok {
			if cachedMeta.Digest == upstreamMeta.Digest {
				slog.Info("serving manifest from cache (tag revalidated)", "repository", repository, "tag", reference, "digest", upstreamMeta.Digest)
				manifest, err := cache.GetManifest(registryHost, repository, reference)
				if err == nil {
					metrics.CacheHit("manifest")
					// Tag unchanged upstream — restart the TTL window
					manifest.Metadata.FetchedAt = time.Now()
					if putErr := cache.PutManifest(registryHost, repository, reference, manifest); putErr != nil {
						slog.Error("failed to refresh manifest TTL", "error", putErr, "repository", repository, "tag", reference)
					}
					return manifest, nil
				}
			}
		}
		// Cache miss or stale tag — fetch from upstream
		return fetchAndCacheManifest(ctx, cache, upstream, registryHost, repository, reference)
	}

	// Upstream HEAD failed — try stale cache
	slog.Warn("upstream HEAD failed, trying stale cache", "error", headErr, "repository", repository, "tag", reference)
	if manifest, err := cache.GetManifest(registryHost, repository, reference); err == nil {
		slog.Warn("serving stale manifest from cache", "repository", repository, "tag", reference)
		metrics.CacheHit("manifest")
		return manifest, nil
	}

	// No cache at all — must go upstream (will fail again, but gives a clear error)
	return fetchAndCacheManifest(ctx, cache, upstream, registryHost, repository, reference)
}

func fetchAndCacheManifest(
	ctx context.Context,
	cache repository.CacheRepository,
	upstream gateway.UpstreamGateway,
	registryHost string,
	repository string,
	reference string,
) (*entity.CachedManifest, error) {
	metrics.CacheMiss("manifest")
	manifest, err := upstream.GetManifest(ctx, registryHost, repository, reference)
	if err != nil {
		return nil, err
	}

	manifest.Metadata.FetchedAt = time.Now()
	if cacheErr := cache.PutManifest(registryHost, repository, reference, manifest); cacheErr != nil {
		slog.Error("failed to cache manifest", "error", cacheErr, "repository", repository, "reference", reference)
		// Don't fail the request — just serve uncached
	}

	return manifest, nil
}

// withinTTL reports whether a cached tag manifest is still trusted without revalidation.
// Entries written before FetchedAt existed have a zero time and always revalidate.
func withinTTL(meta *entity.ManifestMetadata, ttl time.Duration) bool {
	return time.Since(meta.FetchedAt) < ttl
}

// refreshManifestTTL restarts the TTL window on a cached tag after upstream
// confirmed the digest is unchanged.
func refreshManifestTTL(cache repository.CacheRepository, registryHost string, repository string, reference string) {
	manifest, err := cache.GetManifest(registryHost, repository, reference)
	if err != nil {
		return
	}
	manifest.Metadata.FetchedAt = time.Now()
	if err := cache.PutManifest(registryHost, repository, reference, manifest); err != nil {
		slog.Error("failed to refresh manifest TTL", "error", err, "repository", repository, "reference", reference)
	}
}

func isDigest(ref string) bool {
	return strings.Contains(ref, ":")
}
