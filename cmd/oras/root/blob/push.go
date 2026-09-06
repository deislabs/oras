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
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/file"
)

type pushBlobOptions struct {
	option.Common
	option.Descriptor
	option.Pretty
	option.Target
	option.Terminal

	fileRef   string
	mediaType string
	size      int64
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func pushCmd() *cobra.Command {
	var opts pushBlobOptions
	cmd := &cobra.Command{
		Use:   "push [flags] <name>[@digest] <file>",
		Short: "Push a blob to a registry or an OCI image layout",
		Long: `Push a blob to a registry or an OCI image layout

Example - Push blob 'hi.txt' to a registry:
  oras blob push localhost:5000/hello hi.txt

Example - Push blob 'hi.txt' with the specific digest:
  oras blob push localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5 hi.txt

Example - Push blob from stdin with blob size and digest:
  oras blob push --size 12 localhost:5000/hello@sha256:9a201d228ebd966211f7d1131be19f152be428bd373a92071c71d8deaf83b3e5 -

Example - Push blob 'hi.txt' and output the descriptor:
  oras blob push --descriptor localhost:5000/hello hi.txt

Example - Push blob 'hi.txt' with the specific returned media type in the descriptor:
  oras blob push --media-type application/vnd.oci.image.config.v1+json --descriptor localhost:5000/hello hi.txt

Example - Push blob 'hi.txt' and output the prettified descriptor:
  oras blob push --descriptor --pretty localhost:5000/hello hi.txt

Example - Push blob without TLS:
  oras blob push --insecure localhost:5000/hello hi.txt

Example - Push blob 'hi.txt' into an OCI image layout folder 'layout-dir':
  oras blob push --oci-layout layout-dir hi.txt
`,
		Args: oerrors.CheckArgs(argument.Exactly(2), "the destination to push to and the file to read blob content from"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RawReference = args[0]
			opts.fileRef = args[1]
			if opts.fileRef == "-" {
				if err := option.CheckStdinConflict(cmd.Flags()); err != nil {
					return err
				}
				if opts.size < 0 {
					return errors.New("`--size` must be provided if the blob is read from stdin")
				}
			}
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = opts.verbose && !opts.OutputDescriptor
			return pushBlob(cmd, &opts)
		},
	}

	cmd.Flags().Int64VarP(&opts.size, "size", "", -1, "provide the blob size")
	cmd.Flags().StringVarP(&opts.mediaType, "media-type", "", ocispec.MediaTypeImageLayer, "specify the returned media type in the descriptor if --descriptor is used")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "print status output for unnamed blobs")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func pushBlob(cmd *cobra.Command, opts *pushBlobOptions) (err error) {
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	target, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}

	// prepare blob content
	desc, rc, err := file.PrepareBlobContent(opts.fileRef, opts.mediaType, opts.Reference, opts.size)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	statusHandler, metadataHandler := display.NewBlobPushHandler(opts.Printer, opts.OutputDescriptor, opts.Pretty.Pretty, desc, opts.TTY)
	if err := doPush(ctx, statusHandler, target, desc, rc); err != nil {
		return err
	}

	// outputs blob's descriptor
	if opts.OutputDescriptor {
		descJSON, err := opts.Marshal(desc)
		if err != nil {
			return err
		}
		return opts.Output(os.Stdout, descJSON)
	}

	if err := metadataHandler.OnBlobPushed(&opts.Target); err != nil {
		return err
	}
	return metadataHandler.Render()
}

func doPush(ctx context.Context, statusHandler status.BlobPushHandler, t oras.GraphTarget, desc ocispec.Descriptor, r io.Reader) (err error) {
	gt, err := statusHandler.StartTracking(t)
	if err != nil {
		return err
	}
	defer func() {
		stopErr := statusHandler.StopTracking()
		if err == nil {
			err = stopErr
		}
	}()
	exists, err := gt.Exists(ctx, desc)
	if err != nil {
		return err
	}
	if exists {
		return statusHandler.OnBlobExists()
	}
	if err := statusHandler.OnBlobUploading(); err != nil {
		return err
	}
	if err := gt.Push(ctx, desc, r); err != nil {
		return err
	}
	return statusHandler.OnBlobUploaded()
}
