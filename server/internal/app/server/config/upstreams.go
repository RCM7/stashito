package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

const upstreamPrefix = "UPSTREAM_"

var upstreamSuffixes = []string{"_HOST", "_USERNAME", "_PASSWORD"}

// ParseUpstreams builds the registry map from environment variables of the form:
//
//	UPSTREAM_<ALIAS>_HOST     — upstream registry host (required per alias)
//	UPSTREAM_<ALIAS>_USERNAME — basic auth username (optional)
//	UPSTREAM_<ALIAS>_PASSWORD — basic auth password (optional)
//
// The alias used in image names is the lowercased <ALIAS> segment, so
// UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io serves dockerhub/library/postgres.
// At least one upstream must be configured.
func ParseUpstreams(environ []string) (entity.Registries, error) {
	type raw struct {
		host, username, password string
	}
	byAlias := make(map[string]*raw)

	get := func(alias string) *raw {
		if r, ok := byAlias[alias]; ok {
			return r
		}
		r := &raw{}
		byAlias[alias] = r
		return r
	}

	for _, kv := range environ {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, upstreamPrefix) {
			continue
		}
		for _, suffix := range upstreamSuffixes {
			aliasPart := strings.TrimSuffix(key, suffix)
			if aliasPart == key || len(aliasPart) <= len(upstreamPrefix) {
				continue
			}
			alias := strings.ToLower(aliasPart[len(upstreamPrefix):])
			switch suffix {
			case "_HOST":
				get(alias).host = value
			case "_USERNAME":
				get(alias).username = value
			case "_PASSWORD":
				get(alias).password = value
			}
			break
		}
	}

	registries := make(entity.Registries, len(byAlias))
	// Sorted for deterministic error reporting
	aliases := make([]string, 0, len(byAlias))
	for alias := range byAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	for _, alias := range aliases {
		r := byAlias[alias]
		if r.host == "" {
			return nil, fmt.Errorf("upstream %q has credentials but no UPSTREAM_%s_HOST", alias, strings.ToUpper(alias))
		}
		if (r.username == "") != (r.password == "") {
			return nil, fmt.Errorf("upstream %q must set both UPSTREAM_%s_USERNAME and UPSTREAM_%s_PASSWORD, or neither", alias, strings.ToUpper(alias), strings.ToUpper(alias))
		}
		registries[alias] = entity.Upstream{
			Alias:    alias,
			Host:     r.host,
			Username: r.username,
			Password: r.password,
		}
	}

	if len(registries) == 0 {
		return nil, fmt.Errorf("no upstreams configured: set at least one UPSTREAM_<ALIAS>_HOST")
	}

	return registries, nil
}
