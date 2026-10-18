package fs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

func putBlob(t *testing.T, cache *CacheRepository, digest string, size int) {
	t.Helper()
	w, err := cache.PutBlob(testHost, testRepo, digest)
	if err != nil {
		t.Fatalf("PutBlob(%s): %v", digest, err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("x"), size)); err != nil {
		t.Fatalf("Write(%s): %v", digest, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close(%s): %v", digest, err)
	}
}

func cappedCache(t *testing.T, max int64) *CacheRepository {
	t.Helper()
	cache := NewCacheRepository(t.TempDir())
	if err := cache.SetMaxStorage(max); err != nil {
		t.Fatalf("SetMaxStorage: %v", err)
	}
	return cache
}

func assertBlob(t *testing.T, cache *CacheRepository, digest string, want bool) {
	t.Helper()
	if _, ok := cache.BlobExists(testHost, testRepo, digest); ok != want {
		t.Errorf("BlobExists(%s) = %v, want %v", digest, ok, want)
	}
}

func TestEvictionKeepsUsedUnderMax(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)
	putBlob(t, cache, "sha256:b", 100)
	putBlob(t, cache, "sha256:c", 100)

	assertBlob(t, cache, "sha256:a", false)
	assertBlob(t, cache, "sha256:b", true)
	assertBlob(t, cache, "sha256:c", true)
	if used := cache.UsedBytes(); used != 200 {
		t.Errorf("UsedBytes = %d, want 200", used)
	}
}

func TestEvictionIsLeastRecentlyUsed(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)
	putBlob(t, cache, "sha256:b", 100)

	content, _, err := cache.OpenBlob(testHost, testRepo, "sha256:a")
	if err != nil {
		t.Fatalf("OpenBlob: %v", err)
	}
	content.Close()

	putBlob(t, cache, "sha256:c", 100)
	assertBlob(t, cache, "sha256:a", true)
	assertBlob(t, cache, "sha256:b", false)
	assertBlob(t, cache, "sha256:c", true)
}

func TestEvictionHappensDuringDownload(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)
	putBlob(t, cache, "sha256:b", 100)

	w, err := cache.PutBlob(testHost, testRepo, "sha256:c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("x"), 100)); err != nil {
		t.Fatal(err)
	}
	assertBlob(t, cache, "sha256:a", false)
	assertBlob(t, cache, "sha256:b", true)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if used := cache.UsedBytes(); used != 200 {
		t.Errorf("UsedBytes = %d, want 200", used)
	}
}

func TestAbortReleasesReservedBytes(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)

	w, err := cache.PutBlob(testHost, testRepo, "sha256:partial")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("x"), 100)); err != nil {
		t.Fatal(err)
	}
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}

	putBlob(t, cache, "sha256:b", 100)
	assertBlob(t, cache, "sha256:a", true)
	assertBlob(t, cache, "sha256:b", true)
}

func TestOversizedBlobDoesNotEmptyCacheAndGoesFirst(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:small", 100)
	putBlob(t, cache, "sha256:huge", 300)

	assertBlob(t, cache, "sha256:small", true)
	assertBlob(t, cache, "sha256:huge", true)

	putBlob(t, cache, "sha256:next", 100)
	assertBlob(t, cache, "sha256:huge", false)
	assertBlob(t, cache, "sha256:small", true)
	assertBlob(t, cache, "sha256:next", true)
}

func TestEvictionRemovesManifestPairs(t *testing.T) {
	cache := cappedCache(t, 250)
	if err := cache.PutManifest(testHost, testRepo, "sha256:m1", testManifest("sha256:m1")); err != nil {
		t.Fatal(err)
	}
	putBlob(t, cache, "sha256:a", 200)
	putBlob(t, cache, "sha256:b", 50)

	if _, ok := cache.ManifestExists(testHost, testRepo, "sha256:m1"); ok {
		t.Error("manifest still present after eviction")
	}
	dir, base := cache.manifestDir(testHost, testRepo, "sha256:m1")
	for _, suffix := range []string{".meta", ".body"} {
		if _, err := os.Stat(filepath.Join(dir, base+suffix)); !os.IsNotExist(err) {
			t.Errorf("%s%s still on disk", base, suffix)
		}
	}
}

func TestOpenHandleSurvivesEviction(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)

	content, _, err := cache.OpenBlob(testHost, testRepo, "sha256:a")
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()

	putBlob(t, cache, "sha256:b", 100)
	putBlob(t, cache, "sha256:c", 100)
	assertBlob(t, cache, "sha256:a", false)

	data, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("reading evicted blob through open handle: %v", err)
	}
	if len(data) != 100 {
		t.Errorf("read %d bytes, want 100", len(data))
	}
}

func TestStartupIndexEvictsOldestByMtime(t *testing.T) {
	root := t.TempDir()
	cache := NewCacheRepository(root)
	putBlob(t, cache, "sha256:old", 100)
	putBlob(t, cache, "sha256:new", 100)
	if err := cache.PutManifest(testHost, testRepo, "latest", testManifest("sha256:d")); err != nil {
		t.Fatal(err)
	}

	oldDir, oldName := cache.blobDir(testHost, testRepo, "sha256:old")
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(oldDir, oldName), twoHoursAgo, twoHoursAgo); err != nil {
		t.Fatal(err)
	}
	hourAgo := time.Now().Add(-time.Hour)
	for _, ref := range []string{"latest", "sha256:d"} {
		dir, base := cache.manifestDir(testHost, testRepo, ref)
		for _, suffix := range []string{".meta", ".body"} {
			if err := os.Chtimes(filepath.Join(dir, base+suffix), hourAgo, hourAgo); err != nil {
				t.Fatal(err)
			}
		}
	}

	reopened := NewCacheRepository(root)
	if err := reopened.SetMaxStorage(150); err != nil {
		t.Fatalf("SetMaxStorage: %v", err)
	}
	assertBlob(t, reopened, "sha256:old", false)
	assertBlob(t, reopened, "sha256:new", true)
	if _, ok := reopened.ManifestExists(testHost, testRepo, "latest"); ok {
		t.Error("manifest kept, want evicted before the newer blob")
	}
	if used := reopened.UsedBytes(); used != 100 {
		t.Errorf("UsedBytes = %d, want 100", used)
	}
}

func TestStartupIndexOnMissingRoot(t *testing.T) {
	cache := NewCacheRepository(filepath.Join(t.TempDir(), "missing"))
	if err := cache.SetMaxStorage(100); err != nil {
		t.Fatalf("SetMaxStorage on missing root: %v", err)
	}
	putBlob(t, cache, "sha256:a", 50)
	assertBlob(t, cache, "sha256:a", true)
}

func TestNoCapMeansNoEviction(t *testing.T) {
	cache := NewCacheRepository(t.TempDir())
	for _, d := range []string{"sha256:a", "sha256:b", "sha256:c"} {
		putBlob(t, cache, d, 1000)
	}
	for _, d := range []string{"sha256:a", "sha256:b", "sha256:c"} {
		assertBlob(t, cache, d, true)
	}
	if used := cache.UsedBytes(); used != 0 {
		t.Errorf("UsedBytes without cap = %d, want 0", used)
	}
}

func bytesOnDisk(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func stubDiskFree(t *testing.T, root string, capacity int64) {
	t.Helper()
	orig := diskFree
	diskFree = func(string) (int64, bool) { return capacity - bytesOnDisk(t, root), true }
	t.Cleanup(func() { diskFree = orig })
}

func reserve(t *testing.T, cache *CacheRepository, digest string, size int64) error {
	t.Helper()
	w, err := cache.PutBlob(testHost, testRepo, digest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Abort() })
	return w.(*atomicBlobWriter).ReserveSize(size)
}

func TestReserveSizeRefusesBlobThatDoesNotFitOnDisk(t *testing.T) {
	root := t.TempDir()
	cache := NewCacheRepository(root)
	putBlob(t, cache, "sha256:a", 100)
	stubDiskFree(t, root, 150)

	err := reserve(t, cache, "sha256:big", 100)
	if !errors.Is(err, entity.ErrInsufficientStorage) {
		t.Fatalf("ReserveSize = %v, want ErrInsufficientStorage", err)
	}
	assertBlob(t, cache, "sha256:a", true)
}

func TestReserveSizeEvictsToFreeDiskWhenCapped(t *testing.T) {
	root := t.TempDir()
	cache := NewCacheRepository(root)
	if err := cache.SetMaxStorage(1000); err != nil {
		t.Fatal(err)
	}
	putBlob(t, cache, "sha256:a", 100)
	putBlob(t, cache, "sha256:b", 100)
	stubDiskFree(t, root, 250)

	if err := reserve(t, cache, "sha256:c", 100); err != nil {
		t.Fatalf("ReserveSize = %v, want nil after eviction", err)
	}
	assertBlob(t, cache, "sha256:a", false)
	assertBlob(t, cache, "sha256:b", true)
}

func TestReserveSizeRefusesWhenEvictionCannotFreeEnough(t *testing.T) {
	root := t.TempDir()
	cache := NewCacheRepository(root)
	if err := cache.SetMaxStorage(1000); err != nil {
		t.Fatal(err)
	}
	putBlob(t, cache, "sha256:a", 100)
	stubDiskFree(t, root, 150)

	err := reserve(t, cache, "sha256:huge", 500)
	if !errors.Is(err, entity.ErrInsufficientStorage) {
		t.Fatalf("ReserveSize = %v, want ErrInsufficientStorage", err)
	}
}

func TestReserveSizeMakesRoomUnderCapBeforeWriting(t *testing.T) {
	cache := cappedCache(t, 250)
	putBlob(t, cache, "sha256:a", 100)
	putBlob(t, cache, "sha256:b", 100)

	if err := reserve(t, cache, "sha256:c", 100); err != nil {
		t.Fatal(err)
	}
	assertBlob(t, cache, "sha256:a", false)
	assertBlob(t, cache, "sha256:b", true)
}
