package fs

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	root    string
	mu      sync.Map // keyed by "host/repo/ref" to prevent concurrent fetches
	evictor *evictor
}

func NewCacheRepository(root string) *CacheRepository {
	return &CacheRepository{root: root}
}

// SetMaxStorage caps the bytes kept under root. It indexes what is already on
// disk, evicts down to max if needed, and from then on evicts least recently
// used entries inline whenever a write would push the cache past max.
func (c *CacheRepository) SetMaxStorage(max int64) error {
	ev, err := newEvictor(c.root, max)
	if err != nil {
		return fmt.Errorf("indexing storage: %w", err)
	}
	c.evictor = ev
	return nil
}

// UsedBytes reports the bytes tracked by the eviction index; 0 when no cap is set.
func (c *CacheRepository) UsedBytes() int64 {
	return c.evictor.usedBytes()
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

	c.evictor.touch(filepath.Join(dir, base))
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
	c.evictor.add(filepath.Join(dir, base), kindManifest, int64(len(metaBytes)+len(manifest.Body)), false)

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
		c.evictor.add(filepath.Join(digestDir, digestBase), kindManifest, int64(len(metaBytes)+len(manifest.Body)), false)
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

	c.evictor.touch(filepath.Join(dir, base))
	return &meta, true
}

func (c *CacheRepository) BlobExists(registryHost string, repository string, digest string) (int64, bool) {
	dir, fileName := c.blobDir(registryHost, repository, digest)
	path := filepath.Join(dir, fileName)
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	c.evictor.touch(path)
	return info.Size(), true
}

// OpenBlob opens a cached blob for streaming. The open handle keeps the data
// readable even if eviction unlinks the file mid-stream.
func (c *CacheRepository) OpenBlob(registryHost string, repository string, digest string) (io.ReadSeekCloser, int64, error) {
	dir, fileName := c.blobDir(registryHost, repository, digest)
	path := filepath.Join(dir, fileName)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("blob not found: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("blob not found: %w", err)
	}
	c.evictor.touch(path)
	return f, info.Size(), nil
}

// atomicBlobWriter writes to a temp file, then renames to the final path on Close.
type atomicBlobWriter struct {
	tmpFile   *os.File
	finalPath string
	evictor   *evictor
	written   int64
	oversized bool
}

var diskFree = statfsFree

// ReserveSize runs before the first byte of a download: evicts to make room
// when a cap is set, then refuses the blob if the filesystem still cannot
// hold it, so a pull can never fill the disk.
func (w *atomicBlobWriter) ReserveSize(size int64) error {
	dir := filepath.Dir(w.finalPath)
	w.evictor.makeRoom(size)
	free, ok := diskFree(dir)
	if !ok {
		return nil
	}
	if free < size {
		free = w.evictor.freeDisk(dir, size)
	}
	if free < size {
		return fmt.Errorf("%w: blob needs %d bytes, %d bytes free under %s", entity.ErrInsufficientStorage, size, free, dir)
	}
	return nil
}

func (w *atomicBlobWriter) Write(p []byte) (int, error) {
	n, err := w.tmpFile.Write(p)
	if n > 0 && !w.oversized {
		w.written += int64(n)
		if !w.evictor.reserve(int64(n), w.written) {
			w.oversized = true
			slog.Warn("blob is larger than MAX_STORAGE, it will be evicted by the next write", "path", w.finalPath, "max_bytes", w.evictor.max)
		}
	}
	return n, err
}

func (w *atomicBlobWriter) Close() error {
	if err := w.tmpFile.Close(); err != nil {
		w.evictor.release(w.written)
		return err
	}
	info, err := os.Stat(w.tmpFile.Name())
	if err != nil {
		w.evictor.release(w.written)
		return err
	}
	if err := os.Rename(w.tmpFile.Name(), w.finalPath); err != nil {
		w.evictor.release(w.written)
		return err
	}
	if !w.oversized {
		w.evictor.release(w.written)
	}
	w.evictor.add(w.finalPath, kindBlob, info.Size(), w.oversized)
	return nil
}

// Abort discards the pending blob without committing it to the final path.
func (w *atomicBlobWriter) Abort() error {
	if !w.oversized {
		w.evictor.release(w.written)
	}
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
		evictor:   c.evictor,
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
