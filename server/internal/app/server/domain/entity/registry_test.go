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
