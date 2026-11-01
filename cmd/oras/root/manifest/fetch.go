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

package manifest

import (
	"context"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
)

type fetchOptions struct {
	option.Cache
	option.Common
	option.Descriptor
	option.Platform
	option.Pretty
	option.Target
	option.Format

	mediaTypes []string
	outputPath string
}

func fetchCmd() *cobra.Command {
	var opts fetchOptions
	cmd := &cobra.Command{
		Use:   "fetch [flags] <name>{:<tag>|@<digest>}",
		Short: "Fetch manifest of the target artifact",
		Long: `Fetch manifest of the target artifact

Example - Fetch raw manifest from a registry:
  oras manifest fetch localhost:5000/hello:v1

Example - Fetch the descriptor of a manifest from a registry:
  oras manifest fetch --descriptor localhost:5000/hello:v1

Example - [Experimental] Fetch the manifest digest from a registry similar to the resolve command:
  oras manifest fetch --format go-template --template '{{ .digest }}' localhost:5000/hello:v1

Example - [Experimental] Fetch manifest and output metadata encoded in JSON:
  oras manifest fetch localhost:5000/hello:v1 --format json

Example - Fetch manifest from a registry with specified media type:
  oras manifest fetch --media-type 'application/vnd.oci.image.manifest.v1+json' localhost:5000/hello:v1

Example - Fetch manifest from a registry with certain platform:
  oras manifest fetch --platform 'linux/arm/v5' localhost:5000/hello:v1

Example - Fetch manifest from a registry with prettified json result:
  oras manifest fetch --pretty localhost:5000/hello:v1

Example - Fetch raw manifest from an OCI image layout folder 'layout-dir':
  oras manifest fetch --oci-layout layout-dir:v1

Example - Fetch raw manifest from an OCI layout archive file 'layout.tar':
  oras manifest fetch --oci-layout layout.tar:v1

Example - Fetch raw manifest tagged 'example.com:v1' from an OCI image layout folder 'layout-dir':
  oras manifest fetch example.com:v1 --oci-layout-path layout-dir
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the manifest to fetch"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case opts.outputPath == "-" && opts.FormatFlag != option.FormatTypeText.Name:
				return fmt.Errorf("`--output -` cannot be used with `--format %s` at the same time", opts.FormatFlag)
			case opts.outputPath == "-" && opts.OutputDescriptor:
				return fmt.Errorf("`--descriptor` cannot be used with `--output -` at the same time")
			}
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "format", "pretty"); err != nil {
				return err
			}
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "format", "descriptor"); err != nil {
				return err
			}
			opts.RawReference = args[0]
			return option.Parse(cmd, &opts)
		},
		Aliases: []string{"get", "show"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fetchManifest(cmd, &opts)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.mediaTypes, "media-type", "", nil, "accepted media types")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "file `path` to write the fetched manifest to, use - for stdout")
	opts.SetTypes(
		option.FormatTypeText,
		option.FormatTypeJSON.WithUsage("Print in prettified JSON format"),
		option.FormatTypeGoTemplate.WithUsage("Print using the given Go template"),
	)
	option.AddDeprecatedVerboseFlag(cmd.Flags())
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func fetchManifest(cmd *cobra.Command, opts *fetchOptions) (fetchErr error) {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	metadataHandler, contentHandler, err := display.NewManifestFetchHandler(opts.Printer, opts.Format, opts.OutputDescriptor, opts.Pretty.Pretty, opts.outputPath)
	if err != nil {
		return err
	}

	target, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}
	if repo, ok := target.(*remote.Repository); ok {
		repo.ManifestMediaTypes = opts.mediaTypes
	} else if opts.mediaTypes != nil {
		return fmt.Errorf("`--media-type` cannot be used with `--oci-layout` at the same time")
	}

	src, err := opts.CachedTarget(target)
	if err != nil {
		return err
	}
	return doFetchManifest(ctx, src, opts, metadataHandler, contentHandler)
}

func doFetchManifest(ctx context.Context, src oras.ReadOnlyTarget, opts *fetchOptions, metadataHandler metadata.ManifestFetchHandler, contentHandler content.ManifestFetchHandler) error {
	var desc ocispec.Descriptor
	var manifestContent []byte
	var err error
	if opts.OutputDescriptor && opts.outputPath == "" {
		// fetch manifest descriptor only
		fetchOpts := oras.DefaultResolveOptions
		fetchOpts.TargetPlatform = opts.Platform.Platform
		desc, err = oras.Resolve(ctx, src, opts.Reference, fetchOpts)
		if err != nil {
			return fmt.Errorf("failed to find %q: %w", opts.RawReference, err)
		}
	} else {
		// fetch manifest descriptor and content
		fetchOpts := oras.DefaultFetchBytesOptions
		fetchOpts.TargetPlatform = opts.Platform.Platform
		desc, manifestContent, err = oras.FetchBytes(ctx, src, opts.Reference, fetchOpts)
		if err != nil {
			return fmt.Errorf("failed to fetch the content of %q: %w", opts.RawReference, err)
		}
		if err = contentHandler.OnContentFetched(desc, manifestContent); err != nil {
			return err
		}
	}
	return metadataHandler.OnFetched(opts.Path, desc, manifestContent)
}
