package entity

import "testing"

func TestParseImageName(t *testing.T) {
	alias, repo, err := ParseImageName("dockerhub/library/postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "dockerhub" {
		t.Errorf("alias = %q, want %q", alias, "dockerhub")
	}
	if repo != "library/postgres" {
		t.Errorf("repository = %q, want %q", repo, "library/postgres")
	}
}

func TestParseImageNameInvalid(t *testing.T) {
	for _, name := range []string{"postgres", "dockerhub/", ""} {
		if _, _, err := ParseImageName(name); err == nil {
			t.Errorf("ParseImageName(%q) = nil error, want error", name)
		}
	}
}

func TestRegistriesResolve(t *testing.T) {
	r := Registries{"dockerhub": {Alias: "dockerhub", Host: "registry-1.docker.io"}}
	up, err := r.Resolve("dockerhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if up.Host != "registry-1.docker.io" {
		t.Errorf("host = %q, want %q", up.Host, "registry-1.docker.io")
	}
}

func TestRegistriesResolveUnknown(t *testing.T) {
	r := Registries{"dockerhub": {Alias: "dockerhub", Host: "registry-1.docker.io"}}
	if _, err := r.Resolve("ghcr"); err == nil {
		t.Error("Resolve(\"ghcr\") = nil error, want error")
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
