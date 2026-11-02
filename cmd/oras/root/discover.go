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

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/registry"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
)

type discoverOptions struct {
	option.Common
	option.Platform
	option.Target
	option.Format
	option.Terminal

	artifactType string
	depth        int
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func discoverCmd() *cobra.Command {
	var opts discoverOptions
	cmd := &cobra.Command{
		Use:   "discover [flags] <name>{:<tag>|@<digest>}",
		Short: "[Preview] Discover referrers of a manifest in a registry or an OCI image layout",
		Long: `[Preview] Discover referrers of a manifest in a registry or an OCI image layout

** This command is in preview and under development. **

Example - Discover referrers of manifest 'hello:v1' in registry 'localhost:5000', displayed in a tree view:
  oras discover localhost:5000/hello:v1

Example - Discover referrers via referrers API:
  oras discover --distribution-spec v1.1-referrers-api localhost:5000/hello:v1

Example - Discover referrers via tag scheme:
  oras discover --distribution-spec v1.1-referrers-tag localhost:5000/hello:v1

Example - [Experimental] Discover referrers and display in a table view:
  oras discover localhost:5000/hello:v1 --format table

Example - [Experimental] Discover referrers and format output with Go template:
  oras discover localhost:5000/hello:v1 --format go-template --template "{{.referrers}}"

Example - [Experimental] Discover only direct referrers, displayed in json view:
  oras discover localhost:5000/hello:v1 --format json --depth 1

Example - Discover referrers with type 'test-artifact' of manifest 'hello:v1' in registry 'localhost:5000':
  oras discover --artifact-type test-artifact localhost:5000/hello:v1

Example - Discover referrers of the manifest tagged 'v1' in an OCI image layout folder 'layout-dir':
  oras discover --oci-layout layout-dir:v1

Example - Discover referrers of the manifest tagged 'example.com:v1' in an OCI image layout folder 'layout-dir':
  oras discover example.com:v1 --oci-layout-path layout-dir
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the target artifact to discover referrers from"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "format", "output"); err != nil {
				return err
			}
			if cmd.Flags().Changed("depth") && opts.depth < 1 {
				return errors.New("depth value should be at least 1")
			}
			// only show direct referrers for table format
			if opts.FormatFlag == option.FormatTypeTable.Name {
				opts.depth = 1
			}
			opts.RawReference = args[0]
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			if cmd.Flags().Changed("output") {
				switch opts.Format.Type {
				case option.FormatTypeTree.Name, option.FormatTypeJSON.Name, option.FormatTypeTable.Name:
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[DEPRECATED] --output is deprecated, try `--format %s` instead\n", opts.Format.Type)
				default:
					return errors.New("output type can only be tree, table or json")
				}
			}
			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiscover(cmd, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "artifact type")
	cmd.Flags().StringVarP(&opts.FormatFlag, "output", "o", "tree", "[Deprecated] format in which to display referrers (table, json, or tree).")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "display full metadata of referrers")
	cmd.Flags().IntVarP(&opts.depth, "depth", "", 0, "[Experimental] level of referrers to display, if unused shows referrers of all levels")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	opts.SetTypes(
		option.FormatTypeTree,
		option.FormatTypeTable,
		option.FormatTypeJSON.WithUsage("Get referrers and output in JSON format"),
		option.FormatTypeGoTemplate.WithUsage("Print referrers using the given Go template"),
	)
	opts.EnableDistributionSpecFlag()
	option.ApplyFlags(&opts, cmd.Flags())
	cmd.Flags().Lookup(option.NoTTYFlag).Usage = "[Preview] disable colors"
	return oerrors.Command(cmd, &opts.Target)
}

func runDiscover(cmd *cobra.Command, opts *discoverOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	repo, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}

	// discover artifacts
	resolveOpts := oras.DefaultResolveOptions
	resolveOpts.TargetPlatform = opts.Platform.Platform
	desc, err := oras.Resolve(ctx, repo, opts.Reference, resolveOpts)
	if err != nil {
		return err
	}

	handler, err := display.NewDiscoverHandler(opts.Printer, opts.Format, opts.Path, opts.RawReference, desc, opts.verbose, opts.TTY)
	if err != nil {
		return err
	}
	return doDiscover(ctx, repo, desc, opts.artifactType, opts.depth, handler)
}

func doDiscover(ctx context.Context, repo oras.ReadOnlyGraphTarget, desc ocispec.Descriptor, artifactType string, depth int, handler metadata.DiscoverHandler) error {
	if err := fetchAllReferrers(ctx, repo, desc, artifactType, handler, depth, make(map[digest.Digest]bool)); err != nil {
		return err
	}
	return handler.Render()
}

// fetchAllReferrers recursively discovers the referrers of desc. visited tracks
// the descriptors already traversed so that a cyclic referrer graph (e.g.
// A -> B -> A), which a malicious registry can craft, does not cause unbounded
// recursion.
func fetchAllReferrers(ctx context.Context, repo oras.ReadOnlyGraphTarget, desc ocispec.Descriptor, artifactType string, handler metadata.DiscoverHandler, depth int, visited map[digest.Digest]bool) error {
	if visited[desc.Digest] {
		return nil
	}
	visited[desc.Digest] = true

	results, err := registry.Referrers(ctx, repo, desc, artifactType)
	if err != nil {
		return err
	}

	var nextDepth int
	if depth > 0 {
		nextDepth = depth - 1
	}
	for _, r := range results {
		if err := handler.OnDiscovered(r, desc); err != nil {
			return err
		}
		if depth == 1 {
			continue
		}
		if err := fetchAllReferrers(ctx, repo, ocispec.Descriptor{
			Digest:    r.Digest,
			Size:      r.Size,
			MediaType: r.MediaType,
		}, artifactType, handler, nextDepth, visited); err != nil {
			return err
		}
	}
	return nil
}
