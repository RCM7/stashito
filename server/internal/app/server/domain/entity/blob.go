package entity

import "io"

// CachedBlob is a cached blob opened for streaming. The caller must Close Content.
type CachedBlob struct {
	Digest  string
	Size    int64
	Content io.ReadSeekCloser
}
