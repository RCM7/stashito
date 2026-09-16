package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DispatchHandler parses the catch-all /v2/*path and routes to
// the appropriate manifest or blob handler.
type DispatchHandler struct {
	manifest *ManifestHandler
	blob     *BlobHandler
}

func NewDispatchHandler(manifest *ManifestHandler, blob *BlobHandler) *DispatchHandler {
	return &DispatchHandler{manifest: manifest, blob: blob}
}

func (d *DispatchHandler) Handle(ctx *gin.Context) {
	path := strings.TrimPrefix(ctx.Param("path"), "/")

	// Handle /v2/ root — OCI version check
	if path == "" || path == "/" {
		V2Check(ctx)
		return
	}

	if idx := strings.LastIndex(path, "/manifests/"); idx != -1 {
		name := path[:idx]
		reference := path[idx+len("/manifests/"):]
		if name == "" || reference == "" {
			ctx.JSON(http.StatusBadRequest, ociError("NAME_INVALID", "invalid manifest path"))
			return
		}
		if ctx.Request.Method == http.MethodHead {
			d.manifest.Head(ctx, name, reference)
		} else {
			d.manifest.Get(ctx, name, reference)
		}
		return
	}

	if idx := strings.LastIndex(path, "/blobs/"); idx != -1 {
		name := path[:idx]
		digest := path[idx+len("/blobs/"):]
		if name == "" || digest == "" {
			ctx.JSON(http.StatusBadRequest, ociError("DIGEST_INVALID", "invalid blob path"))
			return
		}
		if ctx.Request.Method == http.MethodHead {
			d.blob.Head(ctx, name, digest)
		} else {
			d.blob.Get(ctx, name, digest)
		}
		return
	}

	ctx.JSON(http.StatusNotFound, ociError("NAME_UNKNOWN", "unknown path"))
}

func ociError(code string, message string) gin.H {
	return gin.H{
		"errors": []gin.H{
			{"code": code, "message": message},
		},
	}
}
