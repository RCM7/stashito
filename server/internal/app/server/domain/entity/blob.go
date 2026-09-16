package entity

// CachedBlob represents a cached blob on the filesystem.
type CachedBlob struct {
	Digest string
	Size   int64
	Path   string // filesystem path for streaming
}
