// Package testutil provides shared test infrastructure for image handler tests.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/opencontainers/go-digest"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"
)

// FakeManifest holds raw manifest bytes and their media type.
type FakeManifest struct {
	Bytes     []byte
	MediaType string
}

// FakeRepo implements [image.Repository] for testing. It returns canned
// manifests and blobs keyed by digest, and tracks call counts for
// cache verification tests.
type FakeRepo struct {
	Manifests map[string]FakeManifest
	Blobs     map[string]func() io.ReadCloser
	Named_    reference.Named

	// ResolveDesc and ResolveErr control the return values of Resolve.
	ResolveDesc ocispecv1.Descriptor
	ResolveErr  error

	// CloseErr controls the return value of Close.
	CloseErr error

	// Counters track how many times each method was called.
	ResolveCount      atomic.Int32
	FetchManifestCount atomic.Int32
	FetchBlobCount     atomic.Int32
	CloseCount         atomic.Int32
}

// NewFakeRepo creates a FakeRepo with a default reference.
func NewFakeRepo() *FakeRepo {
	ref, _ := reference.ParseNormalizedNamed("example.com/test:latest")
	return &FakeRepo{
		Manifests: make(map[string]FakeManifest),
		Blobs:     make(map[string]func() io.ReadCloser),
		Named_:    ref,
	}
}

func (r *FakeRepo) Named() reference.Named { return r.Named_ }

func (r *FakeRepo) Close() error {
	r.CloseCount.Add(1)
	return r.CloseErr
}

func (r *FakeRepo) Resolve(_ context.Context) (ocispecv1.Descriptor, error) {
	r.ResolveCount.Add(1)
	return r.ResolveDesc, r.ResolveErr
}

func (r *FakeRepo) FetchManifest(_ context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	r.FetchManifestCount.Add(1)
	m, ok := r.Manifests[desc.Digest.String()]
	if !ok {
		return nil, "", fmt.Errorf("manifest not found: %s", desc.Digest)
	}
	return m.Bytes, m.MediaType, nil
}

func (r *FakeRepo) FetchBlob(_ context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	r.FetchBlobCount.Add(1)
	factory, ok := r.Blobs[desc.Digest.String()]
	if !ok {
		return nil, fmt.Errorf("blob not found: %s", desc.Digest)
	}
	return factory(), nil
}

// AddManifest stores a manifest and returns its descriptor.
func (r *FakeRepo) AddManifest(data []byte, mediaType string) ocispecv1.Descriptor {
	d := digest.FromBytes(data)
	r.Manifests[d.String()] = FakeManifest{Bytes: data, MediaType: mediaType}
	return ocispecv1.Descriptor{
		MediaType: mediaType,
		Digest:    d,
		Size:      int64(len(data)),
	}
}

// AddBlob stores a blob and returns its descriptor.
func (r *FakeRepo) AddBlob(data []byte, mediaType string) ocispecv1.Descriptor {
	d := digest.FromBytes(data)
	r.Blobs[d.String()] = func() io.ReadCloser {
		return io.NopCloser(bytes.NewReader(data))
	}
	return ocispecv1.Descriptor{
		MediaType: mediaType,
		Digest:    d,
		Size:      int64(len(data)),
	}
}

// AddErrorBlob registers a blob that returns an error on read.
func (r *FakeRepo) AddErrorBlob(desc ocispecv1.Descriptor, readErr error) {
	r.Blobs[desc.Digest.String()] = func() io.ReadCloser {
		return io.NopCloser(&errorReader{err: readErr})
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// MustJSON marshals v to JSON and panics on error.
func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

