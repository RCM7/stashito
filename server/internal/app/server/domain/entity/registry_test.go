package entity

import "testing"

func testRegistries(mirrorAlias string) Registries {
	return Registries{
		Upstreams: map[string]Upstream{
			"dockerhub": {Alias: "dockerhub", Host: "registry-1.docker.io"},
			"ghcr":      {Alias: "ghcr", Host: "ghcr.io"},
		},
		Mirror: mirrorAlias,
	}
}

func TestResolveImagePrefixed(t *testing.T) {
	for _, r := range []Registries{testRegistries(""), testRegistries("dockerhub")} {
		up, repo, err := r.ResolveImage("ghcr/acme/api")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if up.Host != "ghcr.io" {
			t.Errorf("host = %q, want %q", up.Host, "ghcr.io")
		}
		if repo != "acme/api" {
			t.Errorf("repository = %q, want %q", repo, "acme/api")
		}
	}
}

func TestResolveImageNoDefaultRejectsUnprefixed(t *testing.T) {
	r := testRegistries("")
	for _, name := range []string{"library/postgres", "postgres", "dockerhub/", ""} {
		if _, _, err := r.ResolveImage(name); err == nil {
			t.Errorf("ResolveImage(%q) = nil error, want error", name)
		}
	}
}

func TestResolveImageDefaultServesUnprefixed(t *testing.T) {
	r := testRegistries("dockerhub")
	for _, name := range []string{"library/postgres", "postgres", "acme/api"} {
		up, repo, err := r.ResolveImage(name)
		if err != nil {
			t.Fatalf("ResolveImage(%q): %v", name, err)
		}
		if up.Host != "registry-1.docker.io" {
			t.Errorf("ResolveImage(%q) host = %q, want registry-1.docker.io", name, up.Host)
		}
		if repo != name {
			t.Errorf("ResolveImage(%q) repository = %q, want %q", name, repo, name)
		}
	}
}

func TestResolveImageDefaultStillRejectsEmpty(t *testing.T) {
	r := testRegistries("dockerhub")
	for _, name := range []string{"", "dockerhub/", "foo/"} {
		if _, _, err := r.ResolveImage(name); err == nil {
			t.Errorf("ResolveImage(%q) = nil error, want error", name)
		}
	}
}

func TestResolveImageAliasWinsOverDefault(t *testing.T) {
	r := testRegistries("dockerhub")
	up, repo, err := r.ResolveImage("ghcr/library/postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.Host != "ghcr.io" || repo != "library/postgres" {
		t.Errorf("got host %q repo %q, want ghcr.io library/postgres", up.Host, repo)
	}
}

func TestRegistriesResolve(t *testing.T) {
	r := testRegistries("")
	up, err := r.Resolve("dockerhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.Host != "registry-1.docker.io" {
		t.Errorf("host = %q, want %q", up.Host, "registry-1.docker.io")
	}
	if _, err := r.Resolve("quay"); err == nil {
		t.Error("Resolve(\"quay\") = nil error, want error")
	}
}

func TestRegistriesAliasesSorted(t *testing.T) {
	got := testRegistries("").Aliases()
	if len(got) != 2 || got[0] != "dockerhub" || got[1] != "ghcr" {
		t.Errorf("Aliases() = %v, want [dockerhub ghcr]", got)
	}
}

func TestUpstreamIsECR(t *testing.T) {
	cases := []struct {
		host   string
		isECR  bool
		region string
	}{
		{"123456789012.dkr.ecr.eu-west-1.amazonaws.com", true, "eu-west-1"},
		{"123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com", true, "us-gov-west-1"},
		{"123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", true, "cn-north-1"},
		{"registry-1.docker.io", false, ""},
		{"public.ecr.aws", false, ""},
		{"ghcr.io", false, ""},
		{"123456789012.dkr.ecr.eu-west-1.amazonaws.com.evil.example", false, ""},
	}
	for _, tc := range cases {
		u := Upstream{Host: tc.host}
		if got := u.IsECR(); got != tc.isECR {
			t.Errorf("IsECR(%q) = %v, want %v", tc.host, got, tc.isECR)
		}
		if got := u.ECRRegion(); got != tc.region {
			t.Errorf("ECRRegion(%q) = %q, want %q", tc.host, got, tc.region)
		}
	}
}
