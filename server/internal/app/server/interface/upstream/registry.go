package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

// Accepted manifest media types for the Accept header
var acceptedManifestTypes = strings.Join([]string{
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.image.index.v1+json",
}, ", ")

type tokenEntry struct {
	token     string
	expiresAt time.Time
}

type tokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// RegistryGateway implements UpstreamGateway for any OCI Distribution
// registry. Authentication is discovered per registry via the
// WWW-Authenticate challenge on a 401 response: Bearer challenges go through
// the token endpoint (with basic credentials when configured), Basic
// challenges send the credentials directly.
type RegistryGateway struct {
	httpClient *http.Client
	upstreams  map[string]entity.Upstream // keyed by registry host
	scheme     string                     // "https"; overridable in tests
	tokenCache map[string]tokenEntry      // host+"/"+repository -> bearer token
	basicHosts map[string]bool            // hosts that use Basic auth directly
	mu         sync.RWMutex
}

func NewRegistryGateway(upstreams map[string]entity.Upstream) *RegistryGateway {
	return &RegistryGateway{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // large layers can take a while
		},
		upstreams:  upstreams,
		scheme:     "https",
		tokenCache: make(map[string]tokenEntry),
		basicHosts: make(map[string]bool),
	}
}

// cachedAuth returns the auth to apply proactively, if any is known for this
// host/repository: a valid bearer token, or basic credentials for hosts that
// answered a Basic challenge before.
func (g *RegistryGateway) applyCachedAuth(req *http.Request, registryHost string, repository string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if entry, ok := g.tokenCache[registryHost+"/"+repository]; ok && time.Now().Before(entry.expiresAt) {
		req.Header.Set("Authorization", "Bearer "+entry.token)
		return
	}
	if g.basicHosts[registryHost] {
		if up, ok := g.upstreams[registryHost]; ok && up.HasCredentials() {
			req.SetBasicAuth(up.Username, up.Password)
		}
	}
}

// challenge holds a parsed WWW-Authenticate header.
type challenge struct {
	scheme  string // "bearer" or "basic"
	realm   string
	service string
	scope   string
}

// parseChallenge parses a WWW-Authenticate header like:
//
//	Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/postgres:pull"
func parseChallenge(header string) (challenge, error) {
	scheme, params, _ := strings.Cut(strings.TrimSpace(header), " ")
	c := challenge{scheme: strings.ToLower(scheme)}
	if c.scheme != "bearer" && c.scheme != "basic" {
		return c, fmt.Errorf("unsupported auth scheme in challenge: %q", header)
	}

	for _, part := range splitChallengeParams(params) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "realm":
			c.realm = value
		case "service":
			c.service = value
		case "scope":
			c.scope = value
		}
	}

	if c.scheme == "bearer" && c.realm == "" {
		return c, fmt.Errorf("bearer challenge without realm: %q", header)
	}
	return c, nil
}

// splitChallengeParams splits on commas that are outside quoted values.
func splitChallengeParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}
	return parts
}

// fetchToken exchanges a bearer challenge for a token, sending basic
// credentials to the token endpoint when the upstream has them configured.
func (g *RegistryGateway) fetchToken(ctx context.Context, c challenge, registryHost string, repository string) (string, error) {
	tokenURL, err := url.Parse(c.realm)
	if err != nil {
		return "", fmt.Errorf("parsing challenge realm %q: %w", c.realm, err)
	}

	q := tokenURL.Query()
	if c.service != "" {
		q.Set("service", c.service)
	}
	scope := c.scope
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", repository)
	}
	q.Set("scope", scope)
	tokenURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}

	if up, ok := g.upstreams[registryHost]; ok && up.HasCredentials() {
		req.SetBasicAuth(up.Username, up.Password)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching auth token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth token request for %s returned %d", registryHost, resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding auth token response: %w", err)
	}

	token := tokenResp.Token
	if token == "" {
		token = tokenResp.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("auth token response from %s contained no token", registryHost)
	}

	ttl := tokenResp.ExpiresIn - 30
	if ttl < 30 {
		ttl = 30
	}

	g.mu.Lock()
	g.tokenCache[registryHost+"/"+repository] = tokenEntry{
		token:     token,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}
	g.mu.Unlock()

	return token, nil
}

// doRegistryRequest performs an authenticated request against the upstream,
// handling the WWW-Authenticate challenge flow on a 401.
func (g *RegistryGateway) doRegistryRequest(ctx context.Context, method string, registryHost string, path string, repository string, accept string) (*http.Response, error) {
	newRequest := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s://%s%s", g.scheme, registryHost, path), nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		return req, nil
	}

	req, err := newRequest()
	if err != nil {
		return nil, err
	}
	g.applyCachedAuth(req, registryHost, repository)

	resp, err := g.doMeasured(req, registryHost)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	header := resp.Header.Get("WWW-Authenticate")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	c, err := parseChallenge(header)
	if err != nil {
		return nil, fmt.Errorf("upstream %s returned 401: %w", registryHost, err)
	}

	retry, err := newRequest()
	if err != nil {
		return nil, err
	}

	switch c.scheme {
	case "bearer":
		token, err := g.fetchToken(ctx, c, registryHost, repository)
		if err != nil {
			return nil, err
		}
		retry.Header.Set("Authorization", "Bearer "+token)
	case "basic":
		up, ok := g.upstreams[registryHost]
		if !ok || !up.HasCredentials() {
			return nil, fmt.Errorf("upstream %s requires basic auth but no credentials are configured", registryHost)
		}
		retry.SetBasicAuth(up.Username, up.Password)
		g.mu.Lock()
		g.basicHosts[registryHost] = true
		g.mu.Unlock()
	}

	return g.doMeasured(retry, registryHost)
}

func (g *RegistryGateway) doMeasured(req *http.Request, registryHost string) (*http.Response, error) {
	resp, err := g.httpClient.Do(req)
	if err != nil {
		metrics.UpstreamErrors.WithLabelValues(registryHost).Inc()
		return nil, err
	}
	metrics.UpstreamRequests.WithLabelValues(registryHost, req.Method, strconv.Itoa(resp.StatusCode)).Inc()
	return resp, nil
}

func (g *RegistryGateway) GetManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.CachedManifest, error) {
	path := fmt.Sprintf("/v2/%s/manifests/%s", repository, reference)

	resp, err := g.doRegistryRequest(ctx, http.MethodGet, registryHost, path, repository, acceptedManifestTypes)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("manifest not found: %s/%s:%s", registryHost, repository, reference)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d for manifest %s/%s:%s", resp.StatusCode, registryHost, repository, reference)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading manifest body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		// Compute digest if header is missing
		hash := sha256.Sum256(body)
		digest = fmt.Sprintf("sha256:%x", hash)
	}

	slog.Info("fetched manifest from upstream",
		"registry", registryHost,
		"repository", repository,
		"reference", reference,
		"digest", digest,
		"contentType", contentType,
		"size", len(body),
	)

	return &entity.CachedManifest{
		Metadata: entity.ManifestMetadata{
			ContentType: contentType,
			Digest:      digest,
			Size:        int64(len(body)),
		},
		Body: body,
	}, nil
}

func (g *RegistryGateway) HeadManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.ManifestMetadata, error) {
	path := fmt.Sprintf("/v2/%s/manifests/%s", repository, reference)

	resp, err := g.doRegistryRequest(ctx, http.MethodHead, registryHost, path, repository, acceptedManifestTypes)
	if err != nil {
		return nil, fmt.Errorf("HEAD manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("manifest not found: %s/%s:%s", registryHost, repository, reference)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d for HEAD manifest %s/%s:%s", resp.StatusCode, registryHost, repository, reference)
	}

	return &entity.ManifestMetadata{
		ContentType: resp.Header.Get("Content-Type"),
		Digest:      resp.Header.Get("Docker-Content-Digest"),
		Size:        resp.ContentLength,
	}, nil
}

func (g *RegistryGateway) GetBlob(ctx context.Context, registryHost string, repository string, digest string, dest io.Writer) (int64, error) {
	path := fmt.Sprintf("/v2/%s/blobs/%s", repository, digest)

	resp, err := g.doRegistryRequest(ctx, http.MethodGet, registryHost, path, repository, "")
	if err != nil {
		return 0, fmt.Errorf("fetching blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("blob not found: %s/%s@%s", registryHost, repository, digest)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("upstream returned %d for blob %s/%s@%s", resp.StatusCode, registryHost, repository, digest)
	}

	n, err := io.Copy(dest, resp.Body)
	if err != nil {
		return n, fmt.Errorf("streaming blob: %w", err)
	}

	slog.Info("fetched blob from upstream",
		"registry", registryHost,
		"repository", repository,
		"digest", digest,
		"size", n,
	)

	return n, nil
}
