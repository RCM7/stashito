package rest

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/RCM7/stashito/server/internal/app/server/config"
	"github.com/RCM7/stashito/server/internal/app/server/interactor"
	"github.com/RCM7/stashito/server/internal/app/server/interface/handler"
	"github.com/RCM7/stashito/server/internal/app/server/interface/storage/fs"
	"github.com/RCM7/stashito/server/internal/app/server/interface/upstream"
	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

// RunRouter starts the HTTP server
func RunRouter(c *config.Config) error {
	// Initialize storage and upstream
	cacheRepo := fs.NewCacheRepository(c.StoragePath)
	registry := upstream.NewRegistryGateway(c.Upstreams.ByHost())

	// Build interactors
	getManifest := interactor.GetManifest(cacheRepo, registry, c.Upstreams, c.TagTTL)
	headManifest := interactor.HeadManifest(cacheRepo, registry, c.Upstreams, c.TagTTL)
	getBlob := interactor.GetBlob(cacheRepo, registry, c.Upstreams)
	headBlob := interactor.HeadBlob(cacheRepo, registry, c.Upstreams)

	// Build handlers
	manifestHandler := handler.NewManifestHandler(getManifest, headManifest)
	blobHandler := handler.NewBlobHandler(getBlob, headBlob)
	dispatch := handler.NewDispatchHandler(manifestHandler, blobHandler)

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	upstreams := make([]string, 0, len(c.Upstreams))
	for alias := range c.Upstreams {
		upstreams = append(upstreams, alias)
	}
	sort.Strings(upstreams)

	router.GET("/healthz", func(ctx *gin.Context) {
		resp := gin.H{
			"status":           "ok",
			"upstreams":        upstreams,
			"storage_path":     c.StoragePath,
			"storage_writable": true,
		}
		probe, err := os.CreateTemp(c.StoragePath, ".healthz-*")
		if err != nil {
			resp["status"] = "degraded"
			resp["storage_writable"] = false
			resp["error"] = err.Error()
			ctx.JSON(http.StatusServiceUnavailable, resp)
			return
		}
		probe.Close()
		os.Remove(probe.Name())
		ctx.JSON(http.StatusOK, resp)
	})

	// OCI Distribution endpoints — single catch-all handles both /v2/ and /v2/<name>/...
	router.GET("/v2/*path", httpMetrics, dispatch.Handle)
	router.HEAD("/v2/*path", httpMetrics, dispatch.Handle)

	if c.MetricsEnabled {
		prometheus.MustRegister(metrics.NewStorageCollector(c.StoragePath))
		metricsHandler := promhttp.Handler()
		if c.MetricsPort != 0 && c.MetricsPort != c.Port {
			go func() {
				mux := http.NewServeMux()
				mux.Handle("/metrics", metricsHandler)
				if err := http.ListenAndServe(fmt.Sprintf(":%d", c.MetricsPort), mux); err != nil {
					fmt.Fprintf(os.Stderr, "Metrics server error: %v\n", err)
					os.Exit(1)
				}
			}()
		} else {
			router.GET("/metrics", gin.WrapH(metricsHandler))
		}
	}

	return router.Run(fmt.Sprintf(":%d", c.Port))
}

func httpMetrics(ctx *gin.Context) {
	start := time.Now()
	ctx.Next()

	path := ctx.Param("path")
	requestType := "v2check"
	switch {
	case strings.Contains(path, "/manifests/"):
		requestType = "manifest"
	case strings.Contains(path, "/blobs/"):
		requestType = "blob"
	}

	method := ctx.Request.Method
	metrics.HTTPRequests.WithLabelValues(requestType, method, strconv.Itoa(ctx.Writer.Status())).Inc()
	metrics.HTTPDuration.WithLabelValues(requestType, method).Observe(time.Since(start).Seconds())
}
