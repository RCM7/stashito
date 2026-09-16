package interactor

import (
	"context"
	"log/slog"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
	"github.com/RCM7/stashito/server/internal/app/server/domain/gateway"
	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

type GetBlobInteractor func(ctx context.Context, name string, digest string) (*entity.CachedBlob, error)
type HeadBlobInteractor func(ctx context.Context, name string, digest string) (int64, error)

func GetBlob(cache *fs.CacheRepository, upstream gateway.UpstreamGateway, registries entity.Registries) GetBlobInteractor {
	return func(ctx context.Context, name string, digest string) (*entity.CachedBlob, error) {
		alias, repo, err := entity.ParseImageName(name)
		if err != nil {
			return nil, err
		}

		up, err := registries.Resolve(alias)
		if err != nil {
			return nil, err
		}
		registryHost := up.Host

		lockKey := registryHost + "/" + repo + "/blob/" + digest
		unlock := cache.Lock(lockKey)
		defer unlock()

		// Check cache first — blobs are content-addressable and immutable
		if size, ok := cache.BlobExists(registryHost, repo, digest); ok {
			path, err := cache.GetBlobPath(registryHost, repo, digest)
			if err == nil {
				slog.Info("serving blob from cache", "repository", repo, "digest", digest, "size", size)
				metrics.CacheHit("blob")
				metrics.BlobBytes.WithLabelValues("cache").Add(float64(size))
				return &entity.CachedBlob{
					Digest: digest,
					Size:   size,
					Path:   path,
				}, nil
			}
		}

		// Not cached — fetch from upstream and store
		metrics.CacheMiss("blob")
		writer, err := cache.PutBlob(registryHost, repo, digest)
		if err != nil {
			return nil, err
		}

		n, err := upstream.GetBlob(ctx, registryHost, repo, digest, writer)
		if err != nil {
			// Discard the partial download — Close would commit it to the cache
			if abortErr := writer.Abort(); abortErr != nil {
				slog.Error("failed to abort blob write", "error", abortErr, "repository", repo, "digest", digest)
			}
			return nil, err
		}

		if err := writer.Close(); err != nil {
			return nil, err
		}

		path, err := cache.GetBlobPath(registryHost, repo, digest)
		if err != nil {
			return nil, err
		}

		metrics.BlobBytes.WithLabelValues("upstream").Add(float64(n))
		return &entity.CachedBlob{
			Digest: digest,
			Size:   n,
			Path:   path,
		}, nil
	}
}

func HeadBlob(cache *fs.CacheRepository, upstream gateway.UpstreamGateway, registries entity.Registries) HeadBlobInteractor {
	return func(ctx context.Context, name string, digest string) (int64, error) {
		alias, repo, err := entity.ParseImageName(name)
		if err != nil {
			return 0, err
		}

		up, err := registries.Resolve(alias)
		if err != nil {
			return 0, err
		}
		registryHost := up.Host

		if size, ok := cache.BlobExists(registryHost, repo, digest); ok {
			return size, nil
		}

		// For HEAD, we don't need to download — just check upstream
		// But for simplicity in MVP, download and cache it
		blob, err := GetBlob(cache, upstream, registries)(ctx, name, digest)
		if err != nil {
			return 0, err
		}
		return blob.Size, nil
	}
}
