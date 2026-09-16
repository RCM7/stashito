package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/RCM7/stashito/server/internal/app/server/interactor"
)

type ManifestHandler struct {
	getInteractor  interactor.GetManifestInteractor
	headInteractor interactor.HeadManifestInteractor
}

func NewManifestHandler(get interactor.GetManifestInteractor, head interactor.HeadManifestInteractor) *ManifestHandler {
	return &ManifestHandler{getInteractor: get, headInteractor: head}
}

func (h *ManifestHandler) Get(ctx *gin.Context, name string, reference string) {
	manifest, err := h.getInteractor(ctx.Request.Context(), name, reference)
	if err != nil {
		slog.Error("failed to get manifest", "name", name, "reference", reference, "error", err)
		status := http.StatusBadGateway
		code := "MANIFEST_UNKNOWN"
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "unknown registry") || strings.Contains(err.Error(), "invalid image") {
			status = http.StatusNotFound
			code = "NAME_UNKNOWN"
		}
		ctx.JSON(status, ociError(code, err.Error()))
		return
	}

	ctx.Header("Docker-Content-Digest", manifest.Metadata.Digest)
	ctx.Header("Content-Type", manifest.Metadata.ContentType)
	ctx.Header("Docker-Distribution-API-Version", "registry/2.0")
	ctx.Data(http.StatusOK, manifest.Metadata.ContentType, manifest.Body)
}

func (h *ManifestHandler) Head(ctx *gin.Context, name string, reference string) {
	meta, err := h.headInteractor(ctx.Request.Context(), name, reference)
	if err != nil {
		slog.Error("failed to HEAD manifest", "name", name, "reference", reference, "error", err)
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "unknown registry") || strings.Contains(err.Error(), "invalid image") {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ociError("MANIFEST_UNKNOWN", err.Error()))
		return
	}

	ctx.Header("Docker-Content-Digest", meta.Digest)
	ctx.Header("Content-Type", meta.ContentType)
	ctx.Header("Docker-Distribution-API-Version", "registry/2.0")
	if meta.Size > 0 {
		ctx.Header("Content-Length", fmt.Sprintf("%d", meta.Size))
	}
	ctx.Status(http.StatusOK)
}
