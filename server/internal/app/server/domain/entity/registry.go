package entity

import (
	"fmt"
	"regexp"
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
type Registries map[string]Upstream

// Resolve maps a registry alias to its upstream.
func (r Registries) Resolve(alias string) (Upstream, error) {
	up, ok := r[alias]
	if !ok {
		return Upstream{}, fmt.Errorf("unknown registry alias: %s", alias)
	}
	return up, nil
}

// ByHost returns the configured upstreams keyed by host, for credential lookup.
func (r Registries) ByHost() map[string]Upstream {
	byHost := make(map[string]Upstream, len(r))
	for _, up := range r {
		byHost[up.Host] = up
	}
	return byHost
}

// ParseImageName splits a full image name like "dockerhub/library/postgres"
// into the registry alias and the repository path.
// Returns alias="dockerhub", repository="library/postgres".
func ParseImageName(name string) (alias string, repository string, err error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", "", fmt.Errorf("invalid image name: %s (expected <registry>/<repository>)", name)
	}
	return parts[0], parts[1], nil
}
