package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// V2Check handles GET /v2/ — the OCI version check endpoint.
// Docker clients call this first to verify the registry speaks V2.
func V2Check(ctx *gin.Context) {
	ctx.Header("Docker-Distribution-API-Version", "registry/2.0")
	ctx.JSON(http.StatusOK, gin.H{})
}
