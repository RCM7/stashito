package metrics

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	storageBytesDesc = prometheus.NewDesc(
		"stashito_storage_bytes",
		"Total bytes on disk under the storage path.",
		nil, nil,
	)
	storageBlobsDesc = prometheus.NewDesc(
		"stashito_storage_blobs",
		"Number of cached blobs on disk.",
		nil, nil,
	)
	storageManifestsDesc = prometheus.NewDesc(
		"stashito_storage_manifests",
		"Number of cached manifests on disk (tag and digest entries).",
		nil, nil,
	)
)

type StorageCollector struct {
	root     string
	interval time.Duration

	mu        sync.Mutex
	scannedAt time.Time
	bytes     float64
	blobs     float64
	manifests float64
}

func NewStorageCollector(root string) *StorageCollector {
	return &StorageCollector{root: root, interval: time.Minute}
}

func (c *StorageCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- storageBytesDesc
	ch <- storageBlobsDesc
	ch <- storageManifestsDesc
}

func (c *StorageCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	if time.Since(c.scannedAt) >= c.interval {
		c.bytes, c.blobs, c.manifests = scan(c.root)
		c.scannedAt = time.Now()
	}
	bytes, blobs, manifests := c.bytes, c.blobs, c.manifests
	c.mu.Unlock()

	ch <- prometheus.MustNewConstMetric(storageBytesDesc, prometheus.GaugeValue, bytes)
	ch <- prometheus.MustNewConstMetric(storageBlobsDesc, prometheus.GaugeValue, blobs)
	ch <- prometheus.MustNewConstMetric(storageManifestsDesc, prometheus.GaugeValue, manifests)
}

func scan(root string) (bytes, blobs, manifests float64) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		bytes += float64(info.Size())
		sep := string(filepath.Separator)
		switch {
		case strings.Contains(path, sep+"blobs"+sep):
			blobs++
		case strings.HasSuffix(path, ".body"):
			manifests++
		}
		return nil
	})
	return bytes, blobs, manifests
}
