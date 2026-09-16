package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

func TestParseChallenge(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   challenge
		hasErr bool
	}{
		{
			name:   "docker hub bearer",
			header: `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/postgres:pull"`,
			want: challenge{
				scheme:  "bearer",
				realm:   "https://auth.docker.io/token",
				service: "registry.docker.io",
				scope:   "repository:library/postgres:pull",
			},
		},
		{
			name:   "scope with comma inside quotes",
			header: `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:a/b:pull,push"`,
			want: challenge{
				scheme:  "bearer",
				realm:   "https://ghcr.io/token",
				service: "ghcr.io",
				scope:   "repository:a/b:pull,push",
			},
		},
		{
			name:   "basic",
			header: `Basic realm="Registry Realm"`,
			want:   challenge{scheme: "basic", realm: "Registry Realm"},
		},
		{
			name:   "bearer without realm",
			header: `Bearer service="x"`,
			hasErr: true,
		},
		{
			name:   "unsupported scheme",
			header: `Digest realm="x"`,
			hasErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseChallenge(tc.header)
			if tc.hasErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChallenge: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// newTestGateway points the gateway at an httptest server over plain HTTP.
func newTestGateway(upstreams map[string]entity.Upstream) *RegistryGateway {
	g := NewRegistryGateway(upstreams)
	g.scheme = "http"
	return g
}

// bearerRegistry simulates a registry + token endpoint in one server.
func bearerRegistry(t *testing.T, expectBasicUser, expectBasicPass string) (*httptest.Server, *int) {
	t.Helper()
	tokenRequests := 0
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		if expectBasicUser != "" {
			user, pass, ok := r.BasicAuth()
			if !ok || user != expectBasicUser || pass != expectBasicPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if got := r.URL.Query().Get("scope"); got != "repository:library/postgres:pull" {
			t.Errorf("token scope = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok123", "expires_in": 300})
	})

	mux.HandleFunc("/v2/library/postgres/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			u, _ := url.Parse(srv.URL)
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="http://%s/token",service="test-registry"`, u.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", "sha256:abc")
		_, _ = w.Write([]byte(`{"schemaVersion":2}`))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &tokenRequests
}

func TestBearerFlowAnonymous(t *testing.T) {
	srv, tokenRequests := bearerRegistry(t, "", "")
	host := strings.TrimPrefix(srv.URL, "http://")

	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "test", Host: host},
	})

	m, err := g.GetManifest(context.Background(), host, "library/postgres", "latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if m.Metadata.Digest != "sha256:abc" {
		t.Errorf("digest = %q", m.Metadata.Digest)
	}

	// Second request reuses cached token
	if _, err := g.GetManifest(context.Background(), host, "library/postgres", "latest"); err != nil {
		t.Fatalf("GetManifest (cached token): %v", err)
	}
	if *tokenRequests != 1 {
		t.Errorf("token endpoint hit %d times, want 1", *tokenRequests)
	}
}

func TestBearerFlowWithCredentials(t *testing.T) {
	srv, _ := bearerRegistry(t, "_json_key", `{"type":"service_account"}`)
	host := strings.TrimPrefix(srv.URL, "http://")

	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "gcp", Host: host, Username: "_json_key", Password: `{"type":"service_account"}`},
	})

	if _, err := g.GetManifest(context.Background(), host, "library/postgres", "latest"); err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
}

func TestBearerFlowMissingCredentials(t *testing.T) {
	srv, _ := bearerRegistry(t, "user", "pass")
	host := strings.TrimPrefix(srv.URL, "http://")

	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "test", Host: host},
	})

	if _, err := g.GetManifest(context.Background(), host, "library/postgres", "latest"); err == nil {
		t.Fatal("expected error when token endpoint rejects anonymous request")
	}
}

func TestBasicAuthFlow(t *testing.T) {
	authAttempts := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/team/app/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "sp-app-id" || pass != "sp-secret" {
			authAttempts++
			w.Header().Set("WWW-Authenticate", `Basic realm="Registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:def")
		_, _ = w.Write([]byte(`{"schemaVersion":2}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "acr", Host: host, Username: "sp-app-id", Password: "sp-secret"},
	})

	if _, err := g.GetManifest(context.Background(), host, "team/app", "v1"); err != nil {
		t.Fatalf("GetManifest: %v", err)
	}

	// Host remembered as basic — second request authenticates proactively
	if _, err := g.GetManifest(context.Background(), host, "team/app", "v1"); err != nil {
		t.Fatalf("GetManifest (remembered basic): %v", err)
	}
	if authAttempts != 1 {
		t.Errorf("unauthenticated attempts = %d, want 1", authAttempts)
	}
}

func TestBasicAuthMissingCredentials(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	g := newTestGateway(map[string]entity.Upstream{
		host: {Alias: "test", Host: host},
	})

	_, err := g.GetManifest(context.Background(), host, "team/app", "v1")
	if err == nil || !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("expected missing-credentials error, got: %v", err)
	}
}
