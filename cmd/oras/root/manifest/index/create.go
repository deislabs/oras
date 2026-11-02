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

package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/root/manifest/common"
	"oras.land/oras/internal/contentutil"
	"oras.land/oras/internal/descriptor"
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/listener"
)

// mediaTypeRegexp is the regular expression pattern required for a valid
// media type, as defined in the image spec schema:
// - https://github.com/opencontainers/image-spec/blob/v1.1.1/schema/defs-descriptor.json#L7
var mediaTypeRegexp = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,126}/[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,126}$`)

type createOptions struct {
	option.Common
	option.Target
	option.Pretty
	option.Annotation

	artifactType string
	sources      []string
	extraRefs    []string
	outputPath   string
}

func createCmd() *cobra.Command {
	var opts createOptions
	cmd := &cobra.Command{
		Use:   "create [flags] <target>[:<tag[,<tag>][...]] <sources>[{<tag>|<digest>}...]",
		Short: "[Experimental] Create and push an index from provided manifests",
		Long: `[Experimental] Create and push an index from provided manifests. All manifests should be in the same repository

Example - Create an index from source manifests tagged 'linux-amd64' and 'linux-arm64', and push without tagging:
  oras manifest index create localhost:5000/hello linux-amd64 linux-arm64

Example - Create an index from source manifests tagged 'linux-amd64' and 'linux-arm64', and push with the tag 'v1':
  oras manifest index create localhost:5000/hello:v1 linux-amd64 linux-arm64

Example - Create an index from source manifests using both tags and digests, and push with tag 'v1':
  oras manifest index create localhost:5000/hello:v1 linux-amd64 sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9

Example - Create an index and push it with multiple tags:
  oras manifest index create localhost:5000/hello:tag1,tag2,tag3 linux-amd64 linux-arm64 sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9

Example - Create and push an index with annotations:
  oras manifest index create localhost:5000/hello:v1 linux-amd64 --annotation "key=val"

Example - Create an index with a specified artifact type:
  oras manifest index create --artifact-type="application/vnd.example+type" localhost:5000/hello linux-amd64

Example - Create an index and push to an OCI image layout folder 'layout-dir' and tag with 'v1':
  oras manifest index create layout-dir:v1 linux-amd64 sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9 --oci-layout

Example - Create an index and save it locally to index.json, auto push will be disabled:
  oras manifest index create localhost:5000/hello linux-amd64 linux-arm64 --output index.json

Example - Create an index and output the index to stdout, auto push will be disabled:
  oras manifest index create localhost:5000/hello linux-arm64 --output - --pretty
`,
		Args: oerrors.CheckArgs(argument.AtLeast(1), "the destination index to create."),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			refs := strings.Split(args[0], ",")
			opts.RawReference = refs[0]
			opts.extraRefs = refs[1:]
			opts.sources = args[1:]

			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}

			return option.ValidateExtraTags(opts.extraRefs)
		},
		Aliases: []string{"pack"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return createIndex(cmd, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "artifact type for overall index")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "file `path` to write the created index to, use - for stdout")
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func createIndex(cmd *cobra.Command, opts createOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	target, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	if opts.artifactType != "" {
		if err := validateMediaType(opts.artifactType); err != nil {
			return err
		}
	}
	displayStatus, displayMetadata, displayContent := display.NewManifestIndexCreateHandler(opts.outputPath, opts.Printer, opts.Pretty.Pretty)
	manifests, err := fetchSourceManifests(ctx, displayStatus, target, opts.sources)
	if err != nil {
		return err
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    ocispec.MediaTypeImageIndex,
		ArtifactType: opts.artifactType,
		Manifests:    manifests,
		Annotations:  opts.Annotations[option.AnnotationManifest],
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return err
	}
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexBytes)
	if err := displayStatus.OnIndexPacked(desc); err != nil {
		return err
	}
	if err := displayContent.OnContentCreated(indexBytes); err != nil {
		return err
	}
	if opts.outputPath == "" {
		if err := pushIndex(ctx, displayStatus, displayMetadata, target, desc, indexBytes, opts.Reference, opts.extraRefs, opts.GetDisplayReference()); err != nil {
			return err
		}
	}
	displayMetadata.OnIndexCreated(desc)
	return displayMetadata.Render()
}

func fetchSourceManifests(ctx context.Context, displayStatus status.ManifestIndexCreateHandler, target oras.ReadOnlyTarget, sources []string) ([]ocispec.Descriptor, error) {
	resolved := []ocispec.Descriptor{}
	for _, source := range sources {
		if err := displayStatus.OnFetching(source); err != nil {
			return nil, err
		}
		desc, content, err := oras.FetchBytes(ctx, target, source, oras.DefaultFetchBytesOptions)
		if err != nil {
			return nil, fmt.Errorf("could not find the manifest %s: %w", source, err)
		}
		if !descriptor.IsManifest(desc) {
			return nil, fmt.Errorf("%s is not a manifest", source)
		}
		if err := displayStatus.OnFetched(source, desc); err != nil {
			return nil, err
		}
		if desc, err = enrichDescriptor(ctx, target, desc, content); err != nil {
			return nil, err
		}
		resolved = append(resolved, desc)
	}
	return resolved, nil
}

func getPlatform(ctx context.Context, target oras.ReadOnlyTarget, manifest *ocispec.Manifest) (*ocispec.Platform, error) {
	if manifest.Config.MediaType != ocispec.MediaTypeImageConfig &&
		manifest.Config.MediaType != docker.MediaTypeConfig {
		return nil, nil //nolint:nilnil
	}
	// if config size is larger than 4 MiB, discontinue the fetch
	if manifest.Config.Size > common.MaxConfigSize {
		return nil, fmt.Errorf("config size %v exceeds the limit of %v bytes: %w", manifest.Config.Size, common.MaxConfigSize, errdef.ErrSizeExceedsLimit)
	}
	// fetch config content
	contentBytes, err := content.FetchAll(ctx, target, manifest.Config)
	if err != nil {
		return nil, err
	}
	var platform ocispec.Platform
	if err := json.Unmarshal(contentBytes, &platform); err != nil {
		// ignore JSON unmarshal errors if the manifest does not have platform information
		return nil, nil //nolint:nilerr,nilnil
	}
	if platform.Architecture == "" && platform.OS == "" {
		// ignore if the manifest does not have platform information
		return nil, nil //nolint:nilnil
	}
	return &platform, nil
}

func pushIndex(ctx context.Context, displayStatus status.ManifestIndexCreateHandler, taggedHandler metadata.TaggedHandler,
	target oras.Target, desc ocispec.Descriptor, content []byte, ref string, extraRefs []string, path string) error {
	// push the index
	var err error
	if ref == "" || contentutil.IsDigest(ref) {
		err = target.Push(ctx, desc, bytes.NewReader(content))
	} else {
		_, err = oras.TagBytes(ctx, target, desc.MediaType, content, ref)
	}
	if err != nil {
		return err
	}
	if err := displayStatus.OnIndexPushed(path); err != nil {
		return err
	}
	if len(extraRefs) != 0 {
		tagListener := listener.NewTaggedListener(target, taggedHandler.OnTagged)
		if _, err = oras.TagBytesN(ctx, tagListener, desc.MediaType, content, extraRefs, oras.DefaultTagBytesNOptions); err != nil {
			return err
		}
	}
	return nil
}

func enrichDescriptor(ctx context.Context, target oras.ReadOnlyTarget, desc ocispec.Descriptor, manifestBytes []byte) (ocispec.Descriptor, error) {
	desc = descriptor.Plain(desc)
	if descriptor.IsImageManifest(desc) {
		var err error
		var manifest ocispec.Manifest
		if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
			return ocispec.Descriptor{}, err
		}
		desc.Platform, err = getPlatform(ctx, target, &manifest)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		desc.ArtifactType = manifest.ArtifactType
		if desc.ArtifactType == "" {
			// Old-style artifacts predate the manifest artifactType field and
			// convey their type through config.mediaType instead. Per the
			// image-spec descriptor guidance, fall back to config.mediaType so
			// the enriched descriptor still advertises an artifactType.
			switch manifest.Config.MediaType {
			case ocispec.MediaTypeImageConfig, ocispec.MediaTypeEmptyJSON, docker.MediaTypeConfig:
				// standard image / empty config: not an artifact type, leave empty
			default:
				desc.ArtifactType = manifest.Config.MediaType
			}
		}
	} else if descriptor.IsIndex(desc) {
		var index ocispec.Index
		if err := json.Unmarshal(manifestBytes, &index); err != nil {
			return ocispec.Descriptor{}, err
		}
		desc.ArtifactType = index.ArtifactType
	}
	return desc, nil
}

// validateMediaType checks whether mediaType uses valid media type syntax,
// returning a non-nil error if not.
func validateMediaType(mediaType string) error {
	if !mediaTypeRegexp.MatchString(mediaType) {
		return fmt.Errorf("%s: %w", mediaType, errdef.ErrInvalidMediaType)
	}
	return nil
}
