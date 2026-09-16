package gateway

import (
	"context"
	"io"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

// UpstreamGateway defines the interface for fetching content from an upstream registry.
type UpstreamGateway interface {
	// GetManifest fetches a manifest by tag or digest from the upstream registry.
	GetManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.CachedManifest, error)

	// HeadManifest checks if a manifest exists upstream and returns metadata only.
	HeadManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.ManifestMetadata, error)

	// GetBlob fetches a blob and streams it to the provided writer.
	// Returns the number of bytes written.
	GetBlob(ctx context.Context, registryHost string, repository string, digest string, dest io.Writer) (int64, error)
}
