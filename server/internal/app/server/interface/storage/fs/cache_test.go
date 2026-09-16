package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

const (
	testHost = "registry-1.docker.io"
	testRepo = "library/alpine"
)

func testManifest(digest string) *entity.CachedManifest {
	return &entity.CachedManifest{
		Metadata: entity.ManifestMetadata{
			ContentType: "application/vnd.oci.image.index.v1+json",
			Digest:      digest,
			Size:        4,
		},
		Body: []byte(`{ }`),
	}
}

func TestManifestRoundTripByTag(t *testing.T) {
	cache := NewCacheRepository(t.TempDir())
	digest := "sha256:abc123"

	if err := cache.PutManifest(testHost, testRepo, "latest", testManifest(digest)); err != nil {
		t.Fatalf("PutManifest: %v", err)
	}

	got, err := cache.GetManifest(testHost, testRepo, "latest")
	if err != nil {
		t.Fatalf("GetManifest by tag: %v", err)
	}
	if got.Metadata.Digest != digest {
		t.Errorf("digest = %q, want %q", got.Metadata.Digest, digest)
	}
	if string(got.Body) != `{ }` {
		t.Errorf("body = %q, want %q", got.Body, `{ }`)
	}

	// A tag write must also be retrievable by its digest
	byDigest, err := cache.GetManifest(testHost, testRepo, digest)
	if err != nil {
		t.Fatalf("GetManifest by digest after tag put: %v", err)
	}
	if string(byDigest.Body) != `{ }` {
		t.Errorf("body by digest = %q, want %q", byDigest.Body, `{ }`)
	}

	meta, ok := cache.ManifestExists(testHost, testRepo, "latest")
	if !ok {
		t.Fatal("ManifestExists = false, want true")
	}
	if meta.Digest != digest {
		t.Errorf("ManifestExists digest = %q, want %q", meta.Digest, digest)
	}
}

func TestManifestMissing(t *testing.T) {
	cache := NewCacheRepository(t.TempDir())

	if _, err := cache.GetManifest(testHost, testRepo, "nope"); err == nil {
		t.Error("GetManifest on empty cache = nil error, want error")
	}
	if _, ok := cache.ManifestExists(testHost, testRepo, "nope"); ok {
		t.Error("ManifestExists on empty cache = true, want false")
	}
}

func TestBlobPutCommit(t *testing.T) {
	cache := NewCacheRepository(t.TempDir())
	digest := "sha256:def456"

	w, err := cache.PutBlob(testHost, testRepo, digest)
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if _, err := w.Write([]byte("layer-data")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Blob must not be visible before Close
	if _, ok := cache.BlobExists(testHost, testRepo, digest); ok {
		t.Error("BlobExists before Close = true, want false")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	size, ok := cache.BlobExists(testHost, testRepo, digest)
	if !ok {
		t.Fatal("BlobExists after Close = false, want true")
	}
	if size != int64(len("layer-data")) {
		t.Errorf("size = %d, want %d", size, len("layer-data"))
	}

	path, err := cache.GetBlobPath(testHost, testRepo, digest)
	if err != nil {
		t.Fatalf("GetBlobPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading blob file: %v", err)
	}
	if string(data) != "layer-data" {
		t.Errorf("blob content = %q, want %q", data, "layer-data")
	}
}

func TestBlobAbort(t *testing.T) {
	root := t.TempDir()
	cache := NewCacheRepository(root)
	digest := "sha256:aborted"

	w, err := cache.PutBlob(testHost, testRepo, digest)
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if _, err := w.Write([]byte("partial")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	if _, ok := cache.BlobExists(testHost, testRepo, digest); ok {
		t.Error("BlobExists after Abort = true, want false")
	}

	// No temp files may remain
	blobDir := filepath.Join(root, "docker", testHost, testRepo, "blobs", "sha256")
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		t.Fatalf("reading blob dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("blob dir has %d leftover files, want 0", len(entries))
	}
}

func TestGetBlobPathMissing(t *testing.T) {
	cache := NewCacheRepository(t.TempDir())
	if _, err := cache.GetBlobPath(testHost, testRepo, "sha256:missing"); err == nil {
		t.Error("GetBlobPath on empty cache = nil error, want error")
	}
}
