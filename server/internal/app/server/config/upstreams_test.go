package config

import (
	"strings"
	"testing"
)

func TestParseUpstreams(t *testing.T) {
	environ := []string{
		"PORT=8080",
		"UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io",
		"UPSTREAM_GCP_HOST=europe-docker.pkg.dev",
		"UPSTREAM_GCP_USERNAME=_json_key",
		`UPSTREAM_GCP_PASSWORD={"type":"service_account","key":"a=b,c"}`,
		"UPSTREAM_MY_ACR_HOST=myregistry.azurecr.io",
		"UPSTREAM_MY_ACR_USERNAME=sp-app-id",
		"UPSTREAM_MY_ACR_PASSWORD=sp-secret",
	}

	registries, err := ParseUpstreams(environ)
	if err != nil {
		t.Fatalf("ParseUpstreams: %v", err)
	}

	if len(registries.Upstreams) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(registries.Upstreams))
	}
	if registries.Mirror != "" {
		t.Errorf("Default = %q, want empty", registries.Mirror)
	}

	hub, err := registries.Resolve("dockerhub")
	if err != nil {
		t.Fatalf("Resolve(dockerhub): %v", err)
	}
	if hub.Host != "registry-1.docker.io" {
		t.Errorf("dockerhub host = %q", hub.Host)
	}
	if hub.HasCredentials() {
		t.Error("dockerhub should be anonymous")
	}

	gcp, err := registries.Resolve("gcp")
	if err != nil {
		t.Fatalf("Resolve(gcp): %v", err)
	}
	if gcp.Username != "_json_key" {
		t.Errorf("gcp username = %q", gcp.Username)
	}
	if !strings.Contains(gcp.Password, `"key":"a=b,c"`) {
		t.Errorf("gcp password mangled: %q", gcp.Password)
	}

	// Alias with underscore
	acr, err := registries.Resolve("my_acr")
	if err != nil {
		t.Fatalf("Resolve(my_acr): %v", err)
	}
	if acr.Host != "myregistry.azurecr.io" || acr.Username != "sp-app-id" {
		t.Errorf("my_acr = %+v", acr)
	}

	if _, err := registries.Resolve("unknown"); err == nil {
		t.Error("Resolve(unknown) should fail")
	}
}

func TestParseUpstreamsDefault(t *testing.T) {
	registries, err := ParseUpstreams([]string{
		"UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io",
		"UPSTREAM_GHCR_HOST=ghcr.io",
		"MIRROR_UPSTREAM=DockerHub",
	})
	if err != nil {
		t.Fatalf("ParseUpstreams: %v", err)
	}
	if registries.Mirror != "dockerhub" {
		t.Errorf("Default = %q, want dockerhub", registries.Mirror)
	}
}

func TestParseUpstreamsErrors(t *testing.T) {
	cases := []struct {
		name    string
		environ []string
		wantErr string
	}{
		{
			name:    "no upstreams",
			environ: []string{"PORT=8080"},
			wantErr: "no upstreams configured",
		},
		{
			name: "default names unknown alias",
			environ: []string{
				"UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io",
				"MIRROR_UPSTREAM=ghcr",
			},
			wantErr: "MIRROR_UPSTREAM",
		},
		{
			name: "credentials without host",
			environ: []string{
				"UPSTREAM_GCP_USERNAME=_json_key",
				"UPSTREAM_GCP_PASSWORD=secret",
			},
			wantErr: "no UPSTREAM_GCP_HOST",
		},
		{
			name: "username without password",
			environ: []string{
				"UPSTREAM_ACR_HOST=x.azurecr.io",
				"UPSTREAM_ACR_USERNAME=user",
			},
			wantErr: "both",
		},
		{
			name: "password without username",
			environ: []string{
				"UPSTREAM_ACR_HOST=x.azurecr.io",
				"UPSTREAM_ACR_PASSWORD=secret",
			},
			wantErr: "both",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseUpstreams(tc.environ)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}
