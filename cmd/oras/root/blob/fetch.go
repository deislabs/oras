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

package blob

import (
	"context"
	"errors"
	"io"
	"os"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display/status/track"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/progress"
)

type fetchBlobOptions struct {
	option.Cache
	option.Common
	option.Descriptor
	option.Pretty
	option.Target
	option.Terminal

	outputPath string
}

func fetchCmd() *cobra.Command {
	var opts fetchBlobOptions
	cmd := &cobra.Command{
		Use:   "fetch [flags] {--output <file> | --descriptor} <name>@<digest>",
		Short: "Fetch a blob from a registry or an OCI image layout",
		Long: `Fetch a blob from a registry or an OCI image layout

Example - Fetch a blob from registry and save it to a local file:
  oras blob fetch --output blob.tar.gz localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5

Example - Fetch a blob from registry and print the raw blob content:
  oras blob fetch --output - localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5

Example - Fetch and print the descriptor of a blob:
  oras blob fetch --descriptor localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5

Example - Fetch a blob, save it to a local file and print the descriptor:
  oras blob fetch --output blob.tar.gz --descriptor localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5

Example - Fetch and print a blob from OCI image layout folder 'layout-dir':
  oras blob fetch --oci-layout --output - layout-dir@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5

Example - Fetch and print a blob from OCI image layout archive file 'layout.tar':
  oras blob fetch --oci-layout --output - layout.tar@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the target blob to fetch"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.outputPath == "" && !opts.OutputDescriptor {
				return errors.New("either `--output` or `--descriptor` must be provided")
			}

			if opts.outputPath == "-" && opts.OutputDescriptor {
				return errors.New("`--output -` cannot be used with `--descriptor` at the same time")
			}
			opts.RawReference = args[0]
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			opts.DisableTTY(opts.Debug, opts.outputPath == "-")
			return nil
		},
		Aliases: []string{"get"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fetchBlob(cmd, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "output file `path`, use - for stdout")
	option.AddDeprecatedVerboseFlag(cmd.Flags())
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func fetchBlob(cmd *cobra.Command, opts *fetchBlobOptions) (fetchErr error) {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	var target oras.ReadOnlyTarget
	target, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}

	if err := opts.EnsureReferenceNotEmpty(cmd, false); err != nil {
		return err
	}

	if repo, ok := target.(*remote.Repository); ok {
		target = repo.Blobs()
	}
	src, err := opts.CachedTarget(target)
	if err != nil {
		return err
	}
	desc, err := opts.doFetch(ctx, src)
	if err != nil {
		return err
	}

	// outputs blob's descriptor if `--descriptor` is used
	if opts.OutputDescriptor {
		descJSON, err := opts.Marshal(desc)
		if err != nil {
			return err
		}
		if err := opts.Output(os.Stdout, descJSON); err != nil {
			return err
		}
	}

	return nil
}

func (opts *fetchBlobOptions) doFetch(ctx context.Context, src oras.ReadOnlyTarget) (desc ocispec.Descriptor, fetchErr error) {
	var err error
	if opts.outputPath == "" {
		// fetch blob descriptor only
		return oras.Resolve(ctx, src, opts.Reference, oras.DefaultResolveOptions)
	}
	// fetch blob content
	var rc io.ReadCloser
	desc, rc, err = oras.Fetch(ctx, src, opts.Reference, oras.DefaultFetchOptions)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	defer func() { _ = rc.Close() }()
	vr := content.NewVerifyReader(rc, desc)

	// outputs blob content if "--output -" is used
	writer := os.Stdout
	if opts.outputPath != "-" {
		// save blob content into the local file if the output path is provided
		file, err := os.Create(opts.outputPath)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		defer func() {
			if err := file.Close(); fetchErr == nil {
				fetchErr = err
			}
		}()
		writer = file
	}

	if opts.TTY == nil {
		// none TTY output
		if _, err = io.Copy(writer, vr); err != nil {
			return ocispec.Descriptor{}, err
		}
	} else {
		// TTY output
		trackedReader, err := track.NewReader(vr, desc, "Downloading", "Downloaded ", opts.TTY)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		defer trackedReader.StopManager()
		if err := progress.Start(trackedReader.Tracker()); err != nil {
			return ocispec.Descriptor{}, err
		}
		if _, err = io.Copy(writer, trackedReader); err != nil {
			return ocispec.Descriptor{}, err
		}
		if err := progress.Done(trackedReader.Tracker()); err != nil {
			return ocispec.Descriptor{}, err
		}
	}
	if err := vr.Verify(); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}
