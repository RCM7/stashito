package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

func manifestStatus(t *testing.T, method string, err error) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(method, "/v2/dockerhub/library/alpine/manifests/sha256-abc", nil)
	h := NewManifestHandler(
		func(context.Context, string, string) (*entity.CachedManifest, error) { return nil, err },
		func(context.Context, string, string) (*entity.ManifestMetadata, error) { return nil, err },
	)
	if method == http.MethodHead {
		h.Head(ctx, "dockerhub/library/alpine", "sha256-abc")
	} else {
		h.Get(ctx, "dockerhub/library/alpine", "sha256-abc")
	}
	return rec.Code
}

func TestManifestNotFoundIs404(t *testing.T) {
	err := fmt.Errorf("%w: registry-1.docker.io/library/alpine:sha256-abc", entity.ErrManifestNotFound)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if got := manifestStatus(t, method, err); got != http.StatusNotFound {
			t.Errorf("%s not found = %d, want 404", method, got)
		}
	}
}

func TestManifestUpstreamFailureIs502(t *testing.T) {
	err := errors.New("fetching manifest: dial tcp: connection refused")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if got := manifestStatus(t, method, err); got != http.StatusBadGateway {
			t.Errorf("%s upstream failure = %d, want 502", method, got)
		}
	}
}
