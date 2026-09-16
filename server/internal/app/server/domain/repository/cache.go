package repository

import (
	"io"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

// BlobWriter writes a blob to the cache. Close commits the blob atomically;
// Abort discards it, leaving nothing at the final path.
type BlobWriter interface {
	io.WriteCloser

	// Abort discards the pending blob. Safe to call instead of Close on error.
	Abort() error
}

// CacheRepository defines the interface for the local filesystem cache.
type CacheRepository interface {
	// GetManifest retrieves a cached manifest by registry host, repository, and reference.
	GetManifest(registryHost string, repository string, reference string) (*entity.CachedManifest, error)

	// PutManifest stores a manifest in the cache.
	PutManifest(registryHost string, repository string, reference string, manifest *entity.CachedManifest) error

	// ManifestExists checks if a manifest exists in the cache and returns its metadata.
	ManifestExists(registryHost string, repository string, reference string) (*entity.ManifestMetadata, bool)

	// BlobExists checks if a blob exists in the cache. Returns size and existence.
	BlobExists(registryHost string, repository string, digest string) (int64, bool)

	// GetBlobPath returns the filesystem path for a cached blob.
	GetBlobPath(registryHost string, repository string, digest string) (string, error)

	// PutBlob opens a writer for storing a blob. The caller must Close (commit)
	// or Abort (discard) the writer. The write is atomic — the blob appears at
	// its final path only after Close.
	PutBlob(registryHost string, repository string, digest string) (BlobWriter, error)
}
