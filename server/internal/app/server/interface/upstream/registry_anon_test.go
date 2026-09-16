package upstream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

// anonGateway returns a gateway pointed at an httptest server that never
// challenges for auth. Returns the server host to use as registryHost.
func anonGateway(srv *httptest.Server) (*RegistryGateway, string) {
	host := strings.TrimPrefix(srv.URL, "http://")
	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "test", Host: host},
	})
	g.httpClient = srv.Client()
	return g, host
}

func TestGetManifest(t *testing.T) {
	body := `{"schemaVersion":2}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/library/alpine/manifests/latest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if accept := r.Header.Get("Accept"); !strings.Contains(accept, "application/vnd.oci.image.index.v1+json") {
			t.Errorf("Accept = %q, missing OCI index type", accept)
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		w.Header().Set("Docker-Content-Digest", "sha256:deadbeef")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	m, err := g.GetManifest(context.Background(), host, "library/alpine", "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if string(m.Body) != body {
		t.Errorf("body = %q, want %q", m.Body, body)
	}
	if m.Metadata.Digest != "sha256:deadbeef" {
		t.Errorf("digest = %q, want sha256:deadbeef", m.Metadata.Digest)
	}
	if m.Metadata.ContentType != "application/vnd.oci.image.index.v1+json" {
		t.Errorf("contentType = %q", m.Metadata.ContentType)
	}
	if m.Metadata.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", m.Metadata.Size, len(body))
	}
}

func TestGetManifestComputesMissingDigest(t *testing.T) {
	body := `{"schemaVersion":2}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Docker-Content-Digest header
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	m, err := g.GetManifest(context.Background(), host, "library/alpine", "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body)))
	if m.Metadata.Digest != want {
		t.Errorf("digest = %q, want computed %q", m.Metadata.Digest, want)
	}
}

func TestGetManifestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	if _, err := g.GetManifest(context.Background(), host, "library/alpine", "nope"); err == nil {
		t.Error("404 = nil error, want error")
	}
}

func TestHeadManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %q, want HEAD", r.Method)
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		w.Header().Set("Docker-Content-Digest", "sha256:deadbeef")
		w.Header().Set("Content-Length", "42")
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	meta, err := g.HeadManifest(context.Background(), host, "library/alpine", "latest")
	if err != nil {
		t.Fatalf("HeadManifest: %v", err)
	}
	if meta.Digest != "sha256:deadbeef" {
		t.Errorf("digest = %q", meta.Digest)
	}
	if meta.Size != 42 {
		t.Errorf("size = %d, want 42", meta.Size)
	}
}

func TestGetBlobStreams(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/library/alpine/blobs/sha256:abc" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	var buf bytes.Buffer
	n, err := g.GetBlob(context.Background(), host, "library/alpine", "sha256:abc", &buf)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("n = %d, want %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("streamed data does not match")
	}
}

func TestGetBlobUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g, host := anonGateway(srv)
	var buf bytes.Buffer
	if _, err := g.GetBlob(context.Background(), host, "library/alpine", "sha256:abc", &buf); err == nil {
		t.Error("500 = nil error, want error")
	}
}
