/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package root

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/fileref"
	"oras.land/oras/internal/graph"
)

func runPullArchive(ctx context.Context, src oras.ReadOnlyTarget, opts *pullOptions, metadataHandler metadata.PullHandler, statusHandler status.PullHandler) error {
	if !strings.EqualFold(filepath.Ext(opts.OutputArchive), ".zip") {
		return fmt.Errorf("unsupported output archive %q: expected a .zip filename", opts.OutputArchive)
	}
	root, err := oras.Resolve(ctx, src, opts.Reference, oras.ResolveOptions{TargetPlatform: opts.Platform.Platform})
	if err != nil {
		return err
	}
	files, err := collectPullArchive(ctx, src, root, opts, metadataHandler)
	if err != nil {
		return err
	}
	if err := savePullArchive(opts.OutputArchive, opts.KeepOldFiles, func(w io.Writer) error {
		return writePullZIP(ctx, w, src, files, statusHandler)
	}); err != nil {
		return err
	}
	metadataHandler.OnPulled(&opts.Target, root)
	return metadataHandler.Render()
}

// collectPullArchive only fetches graph metadata. Payloads are streamed later,
// after ordering names independently of network timing or graph traversal order.
func collectPullArchive(ctx context.Context, src content.Fetcher, root ocispec.Descriptor, opts *pullOptions, metadataHandler metadata.PullHandler) ([]ocispec.Descriptor, error) {
	var configName, configType string
	if opts.ManifestConfigRef != "" {
		var err error
		configName, configType, err = fileref.Parse(opts.ManifestConfigRef, "")
		if err != nil {
			return nil, err
		}
	}
	configSeen := false
	visited := make(map[digest.Digest]bool)
	files := make(map[string]ocispec.Descriptor)
	pending := []ocispec.Descriptor{root}
	for len(pending) != 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if name := node.Annotations[ocispec.AnnotationTitle]; name != "" {
			if err := validateZIPName(name); err != nil {
				return nil, err
			}
			if previous, ok := files[name]; ok && (!content.Equal(previous, node) || !maps.Equal(previous.Annotations, node.Annotations)) {
				return nil, fmt.Errorf("conflicting archive entries for %q", name)
			}
			files[name] = node
		} else if node.Digest != root.Digest && !content.Equal(node, ocispec.DescriptorEmptyJSON) {
			if err := metadataHandler.OnLayerSkipped(node); err != nil {
				return nil, err
			}
		}
		// Record aliases before deduplicating graph traversal by digest.
		if visited[node.Digest] {
			continue
		}
		visited[node.Digest] = true
		nodes, subject, config, err := graph.Successors(ctx, src, node)
		if err != nil {
			return nil, err
		}
		if config != nil && !configSeen {
			configSeen = true
			if configName != "" && (configType == "" || config.MediaType == configType) {
				if config.Annotations == nil {
					config.Annotations = make(map[string]string)
				}
				config.Annotations[ocispec.AnnotationTitle] = configName
			}
		}
		if config != nil && (config.Size != ocispec.DescriptorEmptyJSON.Size || config.Digest != ocispec.DescriptorEmptyJSON.Digest || config.Annotations[ocispec.AnnotationTitle] != "") {
			nodes = append(nodes, *config)
		}
		if subject != nil && opts.IncludeSubject {
			nodes = append(nodes, *subject)
		}
		pending = append(pending, nodes...)
	}
	result := make([]ocispec.Descriptor, 0, len(files))
	for _, node := range files {
		result = append(result, node)
	}
	slices.SortFunc(result, func(a, b ocispec.Descriptor) int {
		return strings.Compare(a.Annotations[ocispec.AnnotationTitle], b.Annotations[ocispec.AnnotationTitle])
	})
	return result, nil
}

// savePullArchive never replaces an existing archive until writing succeeds.
// Only the single archive is staged; extracted payload files are never created.
func savePullArchive(filename string, keepOld bool, write func(io.Writer) error) (err error) {
	var f *os.File
	if keepOld {
		f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	} else {
		var stagingDir string
		stagingDir, err = os.MkdirTemp(filepath.Dir(filename), ".oras-archive-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stagingDir)
		// OpenFile applies the caller's umask, just as the keep-old-files path
		// does. A private directory keeps staged bytes private until rename.
		f, err = os.OpenFile(filepath.Join(stagingDir, "archive.zip"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if err != nil || !keepOld {
			_ = os.Remove(f.Name())
		}
	}()
	if err = write(f); err != nil {
		return err
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	if !keepOld {
		err = os.Rename(f.Name(), filename)
	}
	return err
}
