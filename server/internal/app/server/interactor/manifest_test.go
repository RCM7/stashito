package interactor

import (
	"context"
	"testing"
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
)

const (
	imageName = "dockerhub/library/alpine"
	testHost  = "registry-1.docker.io"
	testRepo  = "library/alpine"
)

func TestGetManifestInvalidName(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	up := &fakeUpstream{}

	if _, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), "noalias", "latest"); err == nil {
		t.Error("invalid name = nil error, want error")
	}
	if _, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), "ghcr/foo/bar", "latest"); err == nil {
		t.Error("unknown alias = nil error, want error")
	}
}

func TestGetManifestByDigestCached(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	if err := cache.PutManifest(testHost, testRepo, digest, newManifest(digest, "cached-body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{}

	got, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "cached-body" {
		t.Errorf("body = %q, want %q", got.Body, "cached-body")
	}
	if up.getManifestCalls != 0 || up.headManifestCalls != 0 {
		t.Errorf("upstream called (%d get, %d head), want 0 — digests are immutable", up.getManifestCalls, up.headManifestCalls)
	}
}

func TestGetManifestByDigestUncachedFetchesAndCaches(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	up := &fakeUpstream{manifest: newManifest(digest, "fresh-body")}

	got, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "fresh-body" {
		t.Errorf("body = %q, want %q", got.Body, "fresh-body")
	}
	if up.getManifestCalls != 1 {
		t.Errorf("getManifestCalls = %d, want 1", up.getManifestCalls)
	}
	if _, ok := cache.ManifestExists(testHost, testRepo, digest); !ok {
		t.Error("manifest not cached after fetch")
	}
}

func TestGetManifestByTagFreshServedFromCache(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	if err := cache.PutManifest(testHost, testRepo, "latest", newManifest(digest, "cached-body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{headMeta: &newManifest(digest, "cached-body").Metadata}

	got, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "cached-body" {
		t.Errorf("body = %q, want %q", got.Body, "cached-body")
	}
	if up.headManifestCalls != 1 {
		t.Errorf("headManifestCalls = %d, want 1 (freshness check)", up.headManifestCalls)
	}
	if up.getManifestCalls != 0 {
		t.Errorf("getManifestCalls = %d, want 0 (tag is fresh)", up.getManifestCalls)
	}
}

func TestGetManifestByTagStaleRefetches(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	oldDigest := "sha256:old"
	newDigest := "sha256:new"
	if err := cache.PutManifest(testHost, testRepo, "latest", newManifest(oldDigest, "old-body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{
		headMeta: &newManifest(newDigest, "new-body").Metadata,
		manifest: newManifest(newDigest, "new-body"),
	}

	got, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "new-body" {
		t.Errorf("body = %q, want %q — stale tag must refetch", got.Body, "new-body")
	}
	if up.getManifestCalls != 1 {
		t.Errorf("getManifestCalls = %d, want 1", up.getManifestCalls)
	}

	meta, ok := cache.ManifestExists(testHost, testRepo, "latest")
	if !ok || meta.Digest != newDigest {
		t.Errorf("cached tag digest = %v, want %q", meta, newDigest)
	}
}

func TestGetManifestByTagUpstreamDownServesStale(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	if err := cache.PutManifest(testHost, testRepo, "latest", newManifest(digest, "stale-body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{headManifestErr: errUpstreamDown, getManifestErr: errUpstreamDown}

	got, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v — upstream down must serve stale cache", err)
	}
	if string(got.Body) != "stale-body" {
		t.Errorf("body = %q, want %q", got.Body, "stale-body")
	}
}

func TestGetManifestByTagUpstreamDownNoCacheErrors(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	up := &fakeUpstream{headManifestErr: errUpstreamDown, getManifestErr: errUpstreamDown}

	if _, err := GetManifest(cache, up, testRegistries, 0)(context.Background(), imageName, "latest"); err == nil {
		t.Error("upstream down with empty cache = nil error, want error")
	}
}

func TestGetManifestByTagWithinTTLSkipsUpstream(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	m := newManifest(digest, "cached-body")
	m.Metadata.FetchedAt = time.Now()
	if err := cache.PutManifest(testHost, testRepo, "latest", m); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{}

	got, err := GetManifest(cache, up, testRegistries, time.Hour)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "cached-body" {
		t.Errorf("body = %q, want %q", got.Body, "cached-body")
	}
	if up.headManifestCalls != 0 || up.getManifestCalls != 0 {
		t.Errorf("upstream called (%d head, %d get), want 0 — tag within TTL", up.headManifestCalls, up.getManifestCalls)
	}
}

func TestGetManifestByTagExpiredTTLRevalidatesAndRefreshes(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	m := newManifest(digest, "cached-body")
	m.Metadata.FetchedAt = time.Now().Add(-2 * time.Hour)
	if err := cache.PutManifest(testHost, testRepo, "latest", m); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{headMeta: &newManifest(digest, "cached-body").Metadata}

	got, err := GetManifest(cache, up, testRegistries, time.Hour)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(got.Body) != "cached-body" {
		t.Errorf("body = %q, want %q", got.Body, "cached-body")
	}
	if up.headManifestCalls != 1 {
		t.Errorf("headManifestCalls = %d, want 1 (TTL expired, revalidate)", up.headManifestCalls)
	}
	if up.getManifestCalls != 0 {
		t.Errorf("getManifestCalls = %d, want 0 (digest unchanged)", up.getManifestCalls)
	}

	// Successful revalidation must restart the TTL window
	meta, ok := cache.ManifestExists(testHost, testRepo, "latest")
	if !ok {
		t.Fatal("manifest gone from cache")
	}
	if time.Since(meta.FetchedAt) > time.Minute {
		t.Errorf("FetchedAt = %v, want refreshed to ~now", meta.FetchedAt)
	}
}

func TestHeadManifestByTagWithinTTLSkipsUpstream(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	m := newManifest(digest, "body")
	m.Metadata.FetchedAt = time.Now()
	if err := cache.PutManifest(testHost, testRepo, "latest", m); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{}

	meta, err := HeadManifest(cache, up, testRegistries, time.Hour)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("HeadManifest: %v", err)
	}
	if meta.Digest != digest {
		t.Errorf("digest = %q, want %q", meta.Digest, digest)
	}
	if up.headManifestCalls != 0 {
		t.Errorf("headManifestCalls = %d, want 0 — tag within TTL", up.headManifestCalls)
	}
}

func TestHeadManifestByDigestCached(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	if err := cache.PutManifest(testHost, testRepo, digest, newManifest(digest, "body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{}

	meta, err := HeadManifest(cache, up, testRegistries, 0)(context.Background(), imageName, digest)
	if err != nil {
		t.Fatalf("HeadManifest: %v", err)
	}
	if meta.Digest != digest {
		t.Errorf("digest = %q, want %q", meta.Digest, digest)
	}
	if up.headManifestCalls != 0 {
		t.Errorf("headManifestCalls = %d, want 0", up.headManifestCalls)
	}
}

func TestHeadManifestByTagUpstreamDownServesStale(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:abc"
	if err := cache.PutManifest(testHost, testRepo, "latest", newManifest(digest, "body")); err != nil {
		t.Fatal(err)
	}
	up := &fakeUpstream{headManifestErr: errUpstreamDown}

	meta, err := HeadManifest(cache, up, testRegistries, 0)(context.Background(), imageName, "latest")
	if err != nil {
		t.Fatalf("HeadManifest: %v — upstream down must serve stale cache", err)
	}
	if meta.Digest != digest {
		t.Errorf("digest = %q, want %q", meta.Digest, digest)
	}
}
