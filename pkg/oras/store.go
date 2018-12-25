package oras

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/remotes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// ensure interface
var (
	_ content.Provider = &MemoryStore{}
)

// MemoryStore stores contents in the memory
type MemoryStore struct {
	content map[string][]byte
}

// NewMemoryStore creates a new memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		content: make(map[string][]byte),
	}
}

// FetchHandler returnes a handler that will fetch all content into the memory store
// discovered in a call to Dispath.
// Use with ChildrenHandler to do a full recurisive fetch.
func (s *MemoryStore) FetchHandler(fetcher remotes.Fetcher) images.HandlerFunc {
	return func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		ctx = log.WithLogger(ctx, log.G(ctx).WithFields(logrus.Fields{
			"digest":    desc.Digest,
			"mediatype": desc.MediaType,
			"size":      desc.Size,
		}))

		// if desc.MediaType == images.MediaTypeDockerSchema1Manifest {
		// 	return nil, fmt.Errorf("%v not supported", desc.MediaType)
		// }

		log.G(ctx).Debug("fetch")
		rc, err := fetcher.Fetch(ctx, desc)
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, err
		}
		s.Set(desc, content)
		return nil, nil
	}
}

// Set adds the content to the store
func (s *MemoryStore) Set(desc ocispec.Descriptor, content []byte) {
	s.content[desc.Digest.String()] = content
}

// Get finds the content from the store
func (s *MemoryStore) Get(desc ocispec.Descriptor) ([]byte, bool) {
	content, ok := s.content[desc.Digest.String()]
	return content, ok
}

// ReaderAt provides contents
func (s *MemoryStore) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	if content, ok := s.content[desc.Digest.String()]; ok {
		return newReaderAt(content), nil

	}
	return nil, ErrNotFound
}

type readerAt struct {
	io.ReaderAt
	size int64
}

func newReaderAt(content []byte) *readerAt {
	return &readerAt{
		ReaderAt: bytes.NewReader(content),
		size:     int64(len(content)),
	}
}

func (r *readerAt) Close() error {
	return nil
}

func (r *readerAt) Size() int64 {
	return r.size
}
