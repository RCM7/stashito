package interactor

import (
	"context"
	"errors"
	"io"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

var errUpstreamDown = errors.New("upstream unreachable")

// fakeUpstream is a configurable gateway.UpstreamGateway for tests.
type fakeUpstream struct {
	manifest *entity.CachedManifest
	headMeta *entity.ManifestMetadata
	blobData []byte

	getManifestErr  error
	headManifestErr error
	getBlobErr      error

	getManifestCalls  int
	headManifestCalls int
	getBlobCalls      int
}

func (f *fakeUpstream) GetManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.CachedManifest, error) {
	f.getManifestCalls++
	if f.getManifestErr != nil {
		return nil, f.getManifestErr
	}
	return f.manifest, nil
}

func (f *fakeUpstream) HeadManifest(ctx context.Context, registryHost string, repository string, reference string) (*entity.ManifestMetadata, error) {
	f.headManifestCalls++
	if f.headManifestErr != nil {
		return nil, f.headManifestErr
	}
	return f.headMeta, nil
}

func (f *fakeUpstream) GetBlob(ctx context.Context, registryHost string, repository string, digest string, dest io.Writer) (int64, error) {
	f.getBlobCalls++
	if f.getBlobErr != nil {
		// Simulate a partial download before the failure
		_, _ = dest.Write([]byte("partial-"))
		return 0, f.getBlobErr
	}
	n, err := dest.Write(f.blobData)
	return int64(n), err
}

func newManifest(digest string, body string) *entity.CachedManifest {
	return &entity.CachedManifest{
		Metadata: entity.ManifestMetadata{
			ContentType: "application/vnd.oci.image.index.v1+json",
			Digest:      digest,
			Size:        int64(len(body)),
		},
		Body: []byte(body),
	}
}

// testRegistries maps the dockerhub alias used across interactor tests.
var testRegistries = entity.Registries{
	"dockerhub": {Alias: "dockerhub", Host: "registry-1.docker.io"},
}
