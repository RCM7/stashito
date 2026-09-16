package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
	domainrepo "github.com/RCM7/stashito/server/internal/app/server/domain/repository"
)

// CacheRepository implements repository.CacheRepository using the filesystem.
//
// Layout:
//
//	<root>/docker/<registryHost>/<repository>/manifests/tags/<tag>.body
//	<root>/docker/<registryHost>/<repository>/manifests/tags/<tag>.meta
//	<root>/docker/<registryHost>/<repository>/manifests/digests/sha256/<hash>.body
//	<root>/docker/<registryHost>/<repository>/manifests/digests/sha256/<hash>.meta
//	<root>/docker/<registryHost>/<repository>/blobs/sha256/<hash>
type CacheRepository struct {
	root string
	mu   sync.Map // keyed by "host/repo/ref" to prevent concurrent fetches
}

func NewCacheRepository(root string) *CacheRepository {
	return &CacheRepository{root: root}
}

// Lock acquires a per-key mutex and returns an unlock function.
func (c *CacheRepository) Lock(key string) func() {
	val, _ := c.mu.LoadOrStore(key, &sync.Mutex{})
	mtx := val.(*sync.Mutex)
	mtx.Lock()
	return mtx.Unlock
}

func (c *CacheRepository) manifestDir(registryHost string, repository string, reference string) (dir string, baseName string) {
	if isDigest(reference) {
		// sha256:abc123 → digests/sha256/abc123
		algo, hash := splitDigest(reference)
		dir = filepath.Join(c.root, "docker", registryHost, repository, "manifests", "digests", algo)
		return dir, hash
	}
	dir = filepath.Join(c.root, "docker", registryHost, repository, "manifests", "tags")
	return dir, reference
}

func (c *CacheRepository) blobDir(registryHost string, repository string, digest string) (dir string, fileName string) {
	algo, hash := splitDigest(digest)
	dir = filepath.Join(c.root, "docker", registryHost, repository, "blobs", algo)
	return dir, hash
}

func (c *CacheRepository) GetManifest(registryHost string, repository string, reference string) (*entity.CachedManifest, error) {
	dir, base := c.manifestDir(registryHost, repository, reference)

	metaBytes, err := os.ReadFile(filepath.Join(dir, base+".meta"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest metadata: %w", err)
	}

	var meta entity.ManifestMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("parsing manifest metadata: %w", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, base+".body"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest body: %w", err)
	}

	return &entity.CachedManifest{
		Metadata: meta,
		Body:     body,
	}, nil
}

func (c *CacheRepository) PutManifest(registryHost string, repository string, reference string, manifest *entity.CachedManifest) error {
	dir, base := c.manifestDir(registryHost, repository, reference)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	metaBytes, err := json.Marshal(manifest.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling manifest metadata: %w", err)
	}

	// Write metadata atomically
	if err := atomicWrite(filepath.Join(dir, base+".meta"), metaBytes); err != nil {
		return fmt.Errorf("writing manifest metadata: %w", err)
	}

	// Write body atomically
	if err := atomicWrite(filepath.Join(dir, base+".body"), manifest.Body); err != nil {
		return fmt.Errorf("writing manifest body: %w", err)
	}

	// If this is a tag, also store under the digest key
	if !isDigest(reference) && manifest.Metadata.Digest != "" {
		digestDir, digestBase := c.manifestDir(registryHost, repository, manifest.Metadata.Digest)
		if err := os.MkdirAll(digestDir, 0755); err != nil {
			return fmt.Errorf("creating digest manifest directory: %w", err)
		}
		if err := atomicWrite(filepath.Join(digestDir, digestBase+".meta"), metaBytes); err != nil {
			return fmt.Errorf("writing digest manifest metadata: %w", err)
		}
		if err := atomicWrite(filepath.Join(digestDir, digestBase+".body"), manifest.Body); err != nil {
			return fmt.Errorf("writing digest manifest body: %w", err)
		}
	}

	return nil
}

func (c *CacheRepository) ManifestExists(registryHost string, repository string, reference string) (*entity.ManifestMetadata, bool) {
	dir, base := c.manifestDir(registryHost, repository, reference)

	metaBytes, err := os.ReadFile(filepath.Join(dir, base+".meta"))
	if err != nil {
		return nil, false
	}

	var meta entity.ManifestMetadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, false
	}

	// Also verify the body file exists
	if _, err := os.Stat(filepath.Join(dir, base+".body")); err != nil {
		return nil, false
	}

	return &meta, true
}

func (c *CacheRepository) BlobExists(registryHost string, repository string, digest string) (int64, bool) {
	dir, fileName := c.blobDir(registryHost, repository, digest)
	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

func (c *CacheRepository) GetBlobPath(registryHost string, repository string, digest string) (string, error) {
	dir, fileName := c.blobDir(registryHost, repository, digest)
	path := filepath.Join(dir, fileName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("blob not found: %w", err)
	}
	return path, nil
}

// atomicBlobWriter writes to a temp file, then renames to the final path on Close.
type atomicBlobWriter struct {
	tmpFile   *os.File
	finalPath string
}

func (w *atomicBlobWriter) Write(p []byte) (int, error) {
	return w.tmpFile.Write(p)
}

func (w *atomicBlobWriter) Close() error {
	if err := w.tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(w.tmpFile.Name(), w.finalPath)
}

// Abort discards the pending blob without committing it to the final path.
func (w *atomicBlobWriter) Abort() error {
	closeErr := w.tmpFile.Close()
	removeErr := os.Remove(w.tmpFile.Name())
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (c *CacheRepository) PutBlob(registryHost string, repository string, digest string) (domainrepo.BlobWriter, error) {
	dir, fileName := c.blobDir(registryHost, repository, digest)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating blob directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-blob-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}

	return &atomicBlobWriter{
		tmpFile:   tmpFile,
		finalPath: filepath.Join(dir, fileName),
	}, nil
}

// atomicWrite writes data to a file atomically using a temp file + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return err
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return err
	}

	return os.Rename(tmpFile.Name(), path)
}

func isDigest(ref string) bool {
	return strings.Contains(ref, ":")
}

func splitDigest(digest string) (algo string, hash string) {
	parts := strings.SplitN(digest, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "sha256", digest
}
