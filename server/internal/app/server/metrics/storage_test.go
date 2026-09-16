package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	root := t.TempDir()
	blobDir := filepath.Join(root, "docker", "registry-1.docker.io", "library", "postgres", "blobs", "sha256")
	manifestDir := filepath.Join(root, "docker", "registry-1.docker.io", "library", "postgres", "manifests", "tags")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blobDir, "abc123"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blobDir, "def456"), make([]byte, 50), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "latest.body"), make([]byte, 20), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "latest.meta"), make([]byte, 10), 0644); err != nil {
		t.Fatal(err)
	}

	bytes, blobs, manifests := scan(root)
	if bytes != 180 {
		t.Errorf("bytes = %v, want 180", bytes)
	}
	if blobs != 2 {
		t.Errorf("blobs = %v, want 2", blobs)
	}
	if manifests != 1 {
		t.Errorf("manifests = %v, want 1", manifests)
	}
}
