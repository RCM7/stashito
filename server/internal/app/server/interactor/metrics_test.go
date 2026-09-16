package interactor

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

func counterValue(t *testing.T, contentType, result string) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.CacheEvents.WithLabelValues(contentType, result))
}

func TestGetBlobCountsCacheMissThenHit(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	digest := "sha256:metricslayer"
	up := &fakeUpstream{blobData: []byte("layer-data")}

	hitsBefore := counterValue(t, "blob", "hit")
	missesBefore := counterValue(t, "blob", "miss")
	cacheBytesBefore := testutil.ToFloat64(metrics.BlobBytes.WithLabelValues("cache"))
	upstreamBytesBefore := testutil.ToFloat64(metrics.BlobBytes.WithLabelValues("upstream"))

	if _, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest); err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if got := counterValue(t, "blob", "miss") - missesBefore; got != 1 {
		t.Errorf("miss delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.BlobBytes.WithLabelValues("upstream")) - upstreamBytesBefore; got != float64(len("layer-data")) {
		t.Errorf("upstream bytes delta = %v, want %d", got, len("layer-data"))
	}

	if _, err := GetBlob(cache, up, testRegistries)(context.Background(), imageName, digest); err != nil {
		t.Fatalf("GetBlob (cached): %v", err)
	}
	if got := counterValue(t, "blob", "hit") - hitsBefore; got != 1 {
		t.Errorf("hit delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.BlobBytes.WithLabelValues("cache")) - cacheBytesBefore; got != float64(len("layer-data")) {
		t.Errorf("cache bytes delta = %v, want %d", got, len("layer-data"))
	}
}

func TestGetManifestCountsCacheMissThenHit(t *testing.T) {
	cache := fs.NewCacheRepository(t.TempDir())
	up := &fakeUpstream{manifest: newManifest("sha256:metricsmanifest", "fresh-body")}

	hitsBefore := counterValue(t, "manifest", "hit")
	missesBefore := counterValue(t, "manifest", "miss")

	get := GetManifest(cache, up, testRegistries, time.Minute)
	if _, err := get(context.Background(), imageName, "sha256:metricsmanifest"); err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if got := counterValue(t, "manifest", "miss") - missesBefore; got != 1 {
		t.Errorf("miss delta = %v, want 1", got)
	}

	if _, err := get(context.Background(), imageName, "sha256:metricsmanifest"); err != nil {
		t.Fatalf("GetManifest (cached): %v", err)
	}
	if got := counterValue(t, "manifest", "hit") - hitsBefore; got != 1 {
		t.Errorf("hit delta = %v, want 1", got)
	}
}
