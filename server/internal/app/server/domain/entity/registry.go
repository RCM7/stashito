package entity

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Upstream describes a configured upstream registry.
// Username/Password are optional; empty means anonymous access.
type Upstream struct {
	Alias    string
	Host     string
	Username string
	Password string
}

// HasCredentials reports whether this upstream has basic credentials configured.
func (u Upstream) HasCredentials() bool {
	return u.Username != ""
}

var ecrHostPattern = regexp.MustCompile(`^\d+\.dkr\.ecr(-fips)?\.([a-z0-9-]+)\.amazonaws\.com(\.cn)?$`)

// IsECR reports whether this upstream is a private AWS ECR registry, detected
// from its host.
func (u Upstream) IsECR() bool {
	return ecrHostPattern.MatchString(u.Host)
}

// ECRRegion returns the AWS region embedded in an ECR host, or "" when the
// host is not an ECR registry.
func (u Upstream) ECRRegion() string {
	m := ecrHostPattern.FindStringSubmatch(u.Host)
	if m == nil {
		return ""
	}
	return m[2]
}

// Registries maps a registry alias (e.g. "dockerhub") to its upstream config.
type Registries struct {
	Upstreams map[string]Upstream
	Mirror    string
}

// Resolve maps a registry alias to its upstream.
func (r Registries) Resolve(alias string) (Upstream, error) {
	up, ok := r.Upstreams[alias]
	if !ok {
		return Upstream{}, fmt.Errorf("unknown registry alias: %s", alias)
	}
	return up, nil
}

// ResolveImage maps a full image name like "dockerhub/library/postgres" to
// its upstream and repository path.
func (r Registries) ResolveImage(name string) (Upstream, string, error) {
	alias, repository, ok := strings.Cut(name, "/")
	if ok && repository != "" {
		if up, found := r.Upstreams[alias]; found {
			return up, repository, nil
		}
	}
	if r.Mirror != "" && name != "" && !strings.HasSuffix(name, "/") {
		return r.Upstreams[r.Mirror], name, nil
	}
	if !ok || repository == "" {
		return Upstream{}, "", fmt.Errorf("invalid image name: %s (expected <registry>/<repository>)", name)
	}
	return Upstream{}, "", fmt.Errorf("unknown registry alias: %s", alias)
}

// Aliases returns the configured aliases, sorted.
func (r Registries) Aliases() []string {
	aliases := make([]string, 0, len(r.Upstreams))
	for alias := range r.Upstreams {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// ByHost returns the configured upstreams keyed by host, for credential lookup.
func (r Registries) ByHost() map[string]Upstream {
	byHost := make(map[string]Upstream, len(r.Upstreams))
	for _, up := range r.Upstreams {
		byHost[up.Host] = up
	}
	return byHost
}
