package entity

import "time"

// ManifestMetadata holds metadata about a cached manifest.
type ManifestMetadata struct {
	ContentType string    `json:"contentType"`
	Digest      string    `json:"digest"`
	Size        int64     `json:"size"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

// CachedManifest is a manifest with its metadata and raw body.
type CachedManifest struct {
	Metadata ManifestMetadata
	Body     []byte
}
