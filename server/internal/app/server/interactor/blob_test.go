package interactor

import (
	"context"
	"io"
	"testing"

	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
)

func TestGetBlobFetchesAndCaches(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:layer1"
	up := &fakeUpstream{blobData: []byte("layer-data")}

	blob, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if blob.Size != int64(len("layer-data")) {
		t.Errorf("size = %d, want %d", blob.Size, len("layer-data"))
	}
	defer blob.Content.Close()
	data, err := io.ReadAll(blob.Content)
	if err != nil {
		t.Fatalf("reading blob content: %v", err)
	}
	if string(data) != "layer-data" {
		t.Errorf("blob content = %q, want %q", data, "layer-data")
	}

	// Second call must hit the cache, not upstream
	if _, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest); err != nil {
		t.Fatalf("GetBlob (cached): %v", err)
	}
	if up.getBlobCalls != 1 {
		t.Errorf("getBlobCalls = %d, want 1", up.getBlobCalls)
	}
}

func TestGetBlobUpstreamErrorLeavesNoPartialBlob(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:broken"
	up := &fakeUpstream{getBlobErr: errUpstreamDown}

	if _, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest); err == nil {
		t.Fatal("GetBlob with failing upstream = nil error, want error")
	}

	// The partial download must not be committed to the cache
	if size, ok := cache.BlobExists(testHost, testRepo, digest); ok {
		t.Errorf("partial blob committed to cache (size %d), want nothing cached", size)
	}

	// A retry with a healthy upstream must succeed and serve the full blob
	up.getBlobErr = nil
	up.blobData = []byte("layer-data")
	blob, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("GetBlob retry: %v", err)
	}
	defer blob.Content.Close()
	data, err := io.ReadAll(blob.Content)
	if err != nil {
		t.Fatalf("reading blob content: %v", err)
	}
	if string(data) != "layer-data" {
		t.Errorf("blob content after retry = %q, want %q", data, "layer-data")
	}
}

func TestHeadBlobCached(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:layer1"
	w, err := cache.PutBlob(testHost, testRepo, digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("layer-data")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{}

	size, err := HeadBlob(cache, up, testRegistries)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("HeadBlob: %v", err)
	}
	if size != int64(len("layer-data")) {
		t.Errorf("size = %d, want %d", size, len("layer-data"))
	}
	if up.getBlobCalls != 0 {
		t.Errorf("getBlobCalls = %d, want 0", up.getBlobCalls)
	}
}

func TestGetBlobMirrorPathSharesCacheWithPrefixedPath(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:layer1"
	up := &fakeUpstream{blobData: []byte("layer-data")}
	mirror := testRegistries
	mirror.Mirror = "dockerhub"

	blob, err := GetBlob(cache, up, mirror)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("GetBlob (prefixed): %v", err)
	}
	blob.Content.Close()

	up.blobData = []byte("changed-upstream")
	blob, err = GetBlob(cache, up, mirror)(context.Background(), "library/alpine", digest)
	if err != nil {
		t.Fatalf("GetBlob (mirror): %v", err)
	}
	defer blob.Content.Close()
	data, err := io.ReadAll(blob.Content)
	if err != nil {
		t.Fatalf("reading blob content: %v", err)
	}
	if string(data) != "layer-data" {
		t.Errorf("mirror path content = %q, want cached %q", data, "layer-data")
	}

	if _, err := GetBlob(cache, up, testRegistries)(context.Background(), "library/alpine", digest); err == nil {
		t.Error("GetBlob without MIRROR_UPSTREAM accepted an un-prefixed name")
	}
}
