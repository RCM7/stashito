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
	disk    diskLedger
}

// diskLedger counts bytes that downloads have announced but not yet written,
// so concurrent ReserveSize calls do not all pass on the same statfs number.
type diskLedger struct {
	mu      sync.Mutex
	pending int64
}

func (l *diskLedger) claim(dir string, size int64, ev *evictor) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	free, ok := diskFree(dir)
	if ok && free-l.pending < size {
		free = ev.freeDisk(dir, size+l.pending)
	}
	if ok && free-l.pending < size {
		return fmt.Errorf("%w: blob needs %d bytes, %d bytes free under %s", entity.ErrInsufficientStorage, size, max(free-l.pending, 0), dir)
	}
	l.pending += size
	return nil
}

func (l *diskLedger) release(n int64) {
	if n == 0 {
		return
	}
	l.mu.Lock()
	l.pending -= n
	l.mu.Unlock()
}

func NewCacheRepository(root string) *CacheRepository {
	return &CacheRepository{root: root}
}

// SetMaxStorage caps the bytes kept under root. It indexes what is already on
// disk, evicts if needed, and from then on evicts least recently used entries
// inline whenever a write would push the cache past max. Each eviction frees
// down to max minus headroom so writes near the cap do not evict one by one.
func (c *CacheRepository) SetMaxStorage(max, headroom int64) error {
	ev, err := newEvictor(c.root, max, headroom)
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

// atomicBlobWriter writes to a temp file, then renames to the final path on
// Close. booked is what it holds in the evictor's in-flight count, pending
// what it holds in the disk ledger; both are given back on Close or Abort.
type atomicBlobWriter struct {
	tmpFile   *os.File
	finalPath string
	evictor   *evictor
	disk      *diskLedger
	written   int64
	booked    int64
	pending   int64
	oversized bool
}

var diskFree = statfsFree

// ReserveSize runs before the first byte of a download: books the announced
// size against the cap (evicting to make room) and against the filesystem's
// free space, then refuses the blob if the disk still cannot hold it, so a
// pull can never fill the disk.
func (w *atomicBlobWriter) ReserveSize(size int64) error {
	w.book(size)
	if err := w.disk.claim(filepath.Dir(w.finalPath), size, w.evictor); err != nil {
		return err
	}
	w.pending = size
	return nil
}

func (w *atomicBlobWriter) book(total int64) {
	if w.oversized || total <= w.booked {
		return
	}
	if !w.evictor.fits(total) {
		w.oversized = true
		w.evictor.release(w.booked)
		w.booked = 0
		slog.Warn("blob is larger than MAX_STORAGE, it will be evicted by the next write", "path", w.finalPath, "max_bytes", w.evictor.max)
		return
	}
	w.evictor.reserve(total - w.booked)
	w.booked = total
}

func (w *atomicBlobWriter) Write(p []byte) (int, error) {
	n, err := w.tmpFile.Write(p)
	if n > 0 {
		w.written += int64(n)
		landed := min(int64(n), w.pending)
		w.pending -= landed
		w.disk.release(landed)
		w.book(w.written)
	}
	return n, err
}

func (w *atomicBlobWriter) settle() {
	w.evictor.release(w.booked)
	w.booked = 0
	w.disk.release(w.pending)
	w.pending = 0
}

func (w *atomicBlobWriter) Close() error {
	defer w.settle()
	if err := w.tmpFile.Close(); err != nil {
		return err
	}
	info, err := os.Stat(w.tmpFile.Name())
	if err != nil {
		return err
	}
	if err := os.Rename(w.tmpFile.Name(), w.finalPath); err != nil {
		return err
	}
	w.settle()
	w.evictor.add(w.finalPath, kindBlob, info.Size(), w.oversized)
	return nil
}

// Abort discards the pending blob without committing it to the final path.
func (w *atomicBlobWriter) Abort() error {
	w.settle()
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
		disk:      &c.disk,
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
