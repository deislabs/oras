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
	"context"
	"encoding/json"
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/contentutil"
	"oras.land/oras/internal/descriptor"
)

type updateOptions struct {
	option.Common
	option.Target
	option.Pretty

	artifactType    string
	addArguments    []string
	mergeArguments  []string
	removeArguments []string
	tags            []string
	outputPath      string
}

func updateCmd() *cobra.Command {
	var opts updateOptions
	cmd := &cobra.Command{
		Use:   "update <index>{:<tag>|@<digest>} [{--add|--merge|--remove} <manifest>{<tag>|<digest>}] [...]",
		Short: "[Experimental] Update and push an image index",
		Long: `[Experimental] Update and push an image index. All manifests should be in the same repository

Example - Remove a manifest and add two manifests from an index tagged 'v1'. The tag will point to the updated index:
  oras manifest index update localhost:5000/hello:v1 --add linux-amd64 --add linux-arm64 --remove sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9

Example - Create a new index by updating an existing index specified by its digest:
  oras manifest index update localhost:5000/hello@sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9 --add linux-amd64 --remove sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb

Example - Merge manifests from the index 'v2-windows' to the index 'v2':
  oras manifest index update localhost:5000/hello:v2 --merge v2-windows

Example - Update an index and tag the updated index as 'v2.1.0' and 'v2':
  oras manifest index update localhost:5000/hello@sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9 --add linux-amd64 --tag "v2.1.0" --tag "v2"

Example - Update an index and push to an OCI image layout folder 'layout-dir' and tag with 'v2':
  oras manifest index update layout-dir@99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9 --add linux-arm64 --tag "v2" --oci-layout

Example - Update an index and save it locally to index.json, auto push will be disabled:
  oras manifest index update localhost:5000/hello:v2 --add v2-linux-amd64 --output index.json

Example - Update an index and output the index to stdout, auto push will be disabled:
  oras manifest index update localhost:5000/hello:v2 --remove sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9 --output - --pretty

Example - Update an index to use a different artifact type:
  oras manifest index update localhost:5000/hello:v1 --artifact-type="application/vnd.example+type"

Example - Update an index to remove any existing artifact type:
  oras manifest index update localhost:5000/hello:v1 --artifact-type=""
  `,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the target index to update"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "tag", "output"); err != nil {
				return err
			}
			opts.RawReference = args[0]
			for _, manifestRef := range opts.removeArguments {
				if !contentutil.IsDigest(manifestRef) {
					return fmt.Errorf("remove: %s is not a digest", manifestRef)
				}
			}
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			return option.ValidateExtraTags(opts.tags)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return updateIndex(cmd, opts)
		},
	}
	option.ApplyFlags(&opts, cmd.Flags())
	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "new artifact type for overall index")
	cmd.Flags().StringArrayVarP(&opts.addArguments, "add", "", nil, "manifests to add to the index")
	cmd.Flags().StringArrayVarP(&opts.mergeArguments, "merge", "", nil, "indexes to be merged into the index")
	cmd.Flags().StringArrayVarP(&opts.removeArguments, "remove", "", nil, "manifests to remove from the index, must be digests")
	cmd.Flags().StringArrayVarP(&opts.tags, "tag", "", nil, "extra tags for the updated index")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "file `path` to write the created index to, use - for stdout")
	return oerrors.Command(cmd, &opts.Target)
}

func updateIndex(cmd *cobra.Command, opts updateOptions) error {
	// if no update flag is used, do nothing
	if !updateFlagsUsed(cmd.Flags()) {
		_ = opts.Printer.Println("Nothing to update as no change is requested")
		return nil
	}
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	target, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}
	if opts.artifactType != "" {
		if err := validateMediaType(opts.artifactType); err != nil {
			return err
		}
	}
	displayStatus, displayMetadata, displayContent := display.NewManifestIndexUpdateHandler(opts.outputPath, opts.Printer, opts.Pretty.Pretty)
	index, err := fetchIndex(ctx, displayStatus, target, opts.Reference)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("artifact-type") {
		index.ArtifactType = opts.artifactType
	}
	manifests, err := removeManifests(displayStatus, index.Manifests, target, opts)
	if err != nil {
		return err
	}
	manifests, err = addManifests(ctx, displayStatus, manifests, target, opts.addArguments)
	if err != nil {
		return err
	}
	manifests, err = mergeIndexes(ctx, displayStatus, manifests, target, opts.mergeArguments)
	if err != nil {
		return err
	}
	index.Manifests = manifests
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return err
	}
	desc := content.NewDescriptorFromBytes(index.MediaType, indexBytes)
	if err := displayStatus.OnIndexPacked(desc); err != nil {
		return err
	}
	path := getPushPath(opts.RawReference, opts.Type, opts.Reference, opts.Path)
	if err := displayContent.OnContentCreated(indexBytes); err != nil {
		return err
	}
	if opts.outputPath == "" {
		if err := pushIndex(ctx, displayStatus, displayMetadata, target, desc, indexBytes, opts.Reference, opts.tags, path); err != nil {
			return err
		}
	}
	displayMetadata.OnIndexCreated(desc)
	return displayMetadata.Render()
}

func fetchIndex(ctx context.Context, handler status.ManifestIndexUpdateHandler, target oras.ReadOnlyTarget, reference string) (ocispec.Index, error) {
	if err := handler.OnFetching(reference); err != nil {
		return ocispec.Index{}, err
	}
	desc, content, err := oras.FetchBytes(ctx, target, reference, oras.DefaultFetchBytesOptions)
	if err != nil {
		return ocispec.Index{}, fmt.Errorf("could not find the index %s: %w", reference, err)
	}
	if !descriptor.IsIndex(desc) {
		return ocispec.Index{}, fmt.Errorf("%s is not an index", reference)
	}
	if err := handler.OnFetched(reference, desc); err != nil {
		return ocispec.Index{}, err
	}
	var index ocispec.Index
	if err := json.Unmarshal(content, &index); err != nil {
		return ocispec.Index{}, err
	}
	if index.MediaType == "" {
		// mediaType is optional in older indexes. Fall back to the resolved
		// descriptor, which IsIndex has already checked, so the updated index
		// is not repacked as application/octet-stream.
		index.MediaType = desc.MediaType
	}
	return index, nil
}

func addManifests(ctx context.Context, displayStatus status.ManifestIndexUpdateHandler, manifests []ocispec.Descriptor, target oras.ReadOnlyTarget, addArguments []string) ([]ocispec.Descriptor, error) {
	for _, manifestRef := range addArguments {
		if err := displayStatus.OnFetching(manifestRef); err != nil {
			return nil, err
		}
		desc, content, err := oras.FetchBytes(ctx, target, manifestRef, oras.DefaultFetchBytesOptions)
		if err != nil {
			return nil, fmt.Errorf("could not find the manifest %s: %w", manifestRef, err)
		}
		if !descriptor.IsManifest(desc) {
			return nil, fmt.Errorf("%s is not a manifest", manifestRef)
		}
		if err := displayStatus.OnFetched(manifestRef, desc); err != nil {
			return nil, err
		}
		if desc, err = enrichDescriptor(ctx, target, desc, content); err != nil {
			return nil, err
		}
		manifests = append(manifests, desc)
		if err := displayStatus.OnManifestAdded(manifestRef, desc); err != nil {
			return nil, err
		}
	}
	return manifests, nil
}

func mergeIndexes(ctx context.Context, displayStatus status.ManifestIndexUpdateHandler, manifests []ocispec.Descriptor, target oras.ReadOnlyTarget, mergeArguments []string) ([]ocispec.Descriptor, error) {
	for _, indexRef := range mergeArguments {
		if err := displayStatus.OnFetching(indexRef); err != nil {
			return nil, err
		}
		desc, content, err := oras.FetchBytes(ctx, target, indexRef, oras.DefaultFetchBytesOptions)
		if err != nil {
			return nil, fmt.Errorf("could not find the index %s: %w", indexRef, err)
		}
		if !descriptor.IsIndex(desc) {
			return nil, fmt.Errorf("%s is not an index", indexRef)
		}
		if err := displayStatus.OnFetched(indexRef, desc); err != nil {
			return nil, err
		}
		var index ocispec.Index
		if err := json.Unmarshal(content, &index); err != nil {
			return nil, err
		}
		manifests = append(manifests, index.Manifests...)
		if err := displayStatus.OnIndexMerged(indexRef, desc); err != nil {
			return nil, err
		}
	}
	return manifests, nil
}

func removeManifests(handler status.ManifestIndexUpdateHandler, manifests []ocispec.Descriptor, _ oras.ReadOnlyTarget, opts updateOptions) ([]ocispec.Descriptor, error) {
	// create a set of digests to speed up the remove
	digestToRemove := make(map[digest.Digest]bool)
	for _, manifestRef := range opts.removeArguments {
		digestToRemove[digest.Digest(manifestRef)] = false
	}
	return doRemoveManifests(manifests, digestToRemove, handler, opts.Reference)
}

func doRemoveManifests(originalManifests []ocispec.Descriptor, digestToRemove map[digest.Digest]bool, handler status.ManifestIndexUpdateHandler, indexRef string) ([]ocispec.Descriptor, error) {
	manifests := []ocispec.Descriptor{}
	for _, m := range originalManifests {
		if _, exists := digestToRemove[m.Digest]; exists {
			digestToRemove[m.Digest] = true
		} else {
			manifests = append(manifests, m)
		}
	}
	for digest, removed := range digestToRemove {
		if !removed {
			return nil, fmt.Errorf("%s does not exist in the index %s", digest, indexRef)
		}
		if err := handler.OnManifestRemoved(digest); err != nil {
			return nil, err
		}
	}
	return manifests, nil
}

func updateFlagsUsed(flags *pflag.FlagSet) bool {
	return flags.Changed("add") || flags.Changed("remove") || flags.Changed("merge") || flags.Changed("artifact-type")
}

func getPushPath(rawReference string, targetType string, reference string, path string) string {
	if contentutil.IsDigest(reference) {
		return fmt.Sprintf("[%s] %s", targetType, path)
	}
	return fmt.Sprintf("[%s] %s", targetType, rawReference)
}
