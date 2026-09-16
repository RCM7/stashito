package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/RCM7/stashito/server/internal/app/server/interactor"
)

type BlobHandler struct {
	getInteractor  interactor.GetBlobInteractor
	headInteractor interactor.HeadBlobInteractor
}

func NewBlobHandler(get interactor.GetBlobInteractor, head interactor.HeadBlobInteractor) *BlobHandler {
	return &BlobHandler{getInteractor: get, headInteractor: head}
}

func (h *BlobHandler) Get(ctx *gin.Context, name string, digest string) {
	blob, err := h.getInteractor(ctx.Request.Context(), name, digest)
	if err != nil {
		slog.Error("failed to get blob", "name", name, "digest", digest, "error", err)
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "unknown registry") || strings.Contains(err.Error(), "invalid image") {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ociError("BLOB_UNKNOWN", err.Error()))
		return
	}

	ctx.Header("Docker-Content-Digest", blob.Digest)
	ctx.Header("Content-Length", fmt.Sprintf("%d", blob.Size))
	ctx.Header("Docker-Distribution-API-Version", "registry/2.0")
	ctx.File(blob.Path)
}

func (h *BlobHandler) Head(ctx *gin.Context, name string, digest string) {
	size, err := h.headInteractor(ctx.Request.Context(), name, digest)
	if err != nil {
		slog.Error("failed to HEAD blob", "name", name, "digest", digest, "error", err)
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		ctx.JSON(status, ociError("BLOB_UNKNOWN", err.Error()))
		return
	}

	ctx.Header("Docker-Content-Digest", digest)
	ctx.Header("Content-Length", fmt.Sprintf("%d", size))
	ctx.Header("Docker-Distribution-API-Version", "registry/2.0")
	ctx.Status(http.StatusOK)
}
