//go:build !unix

package fs

func statfsFree(dir string) (int64, bool) {
	return 0, false
}
