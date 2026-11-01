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
	"os"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	orasio "oras.land/oras/internal/io"
)

type restoreOptions struct {
	option.Common
	option.Remote
	option.Terminal

	// flags
	input            string
	excludeReferrers bool
	dryRun           bool
	concurrency      int

	// derived options
	repository string
	tags       []string
}

func restoreCmd() *cobra.Command {
	var opts restoreOptions
	cmd := &cobra.Command{
		Use:   "restore [flags] --input <path> <registry>/<repository>[:<ref1>[,<ref2>...]]",
		Short: "[Experimental] Restore artifacts to a registry from an OCI image layout",
		Long: `[Experimental] Restore artifacts to a registry from an OCI image layout, which can be either a directory or a tar archive.

Example - Restore a single artifact from a tar archive:
  oras restore --input hello.tar localhost:5000/hello:v1

Example - Restore a single artifact from a directory:
  oras restore --input hello localhost:5000/hello:v1

Example - Perform a dry run without actually uploading artifacts:
  oras restore --input hello --dry-run localhost:5000/hello:v1

Example - Restore multiple specific tags:
  oras restore --input hello localhost:5000/hello:v1,v2

Example - Restore all tagged artifacts:
  oras restore --input hello localhost:5000/hello

Example - Exclude referrers when restoring artifacts:
  oras restore --input hello --exclude-referrers localhost:5000/hello

Example - Use Referrers API for discovering referrers:
  oras restore --input hello --distribution-spec v1.1-referrers-api localhost:5000/hello

Example - Use Referrers Tag Schema for discovering referrers:
  oras restore --input hello --distribution-spec v1.1-referrers-tag localhost:5000/hello

Example - Restore to an insecure registry:
  oras restore --input hello --insecure localhost:5000/hello:v1

Example - Restore to a plain HTTP registry (no TLS):
  oras restore --input hello --plain-http localhost:5000/hello:v1

Example - Set custom concurrency level:
  oras restore --input hello --concurrency 6 localhost:5000/hello:v1
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the targets to restore to"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}

			// parse repo and tags
			var err error
			opts.repository, opts.tags, err = parseArtifactReferences(args[0])
			if err != nil {
				return err
			}

			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = true // always print verbose output
			return runRestore(cmd, &opts)
		},
	}

	// required flag
	cmd.Flags().StringVar(&opts.input, "input", "", "path to the OCI layout, either a tar archive (*.tar) or a directory")
	_ = cmd.MarkFlagRequired("input")
	// optional flags
	cmd.Flags().BoolVar(&opts.excludeReferrers, "exclude-referrers", false, "restore artifacts excluding their referrers")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "simulate the restore process without actually uploading any artifacts")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 5, "concurrency level")
	opts.EnableDistributionSpecFlag()
	// apply flags
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Remote)
}

func runRestore(cmd *cobra.Command, opts *restoreOptions) error {
	if opts.input == "" {
		return errors.New("the input path cannot be empty")
	}
	startTime := time.Now() // start timing the restore process
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	// prepare the target registry
	dstRepo, err := opts.NewRepository(opts.repository, opts.Common, logger)
	if err != nil {
		return fmt.Errorf("failed to prepare target repository %q: %w", opts.repository, err)
	}
	statusHandler, metadataHandler := display.NewRestoreHandler(opts.Printer, opts.TTY, dstRepo, opts.dryRun)
	return doRestore(ctx, opts, dstRepo, startTime, statusHandler, metadataHandler)
}

func doRestore(
	ctx context.Context,
	opts *restoreOptions,
	dstRepo oras.GraphTarget,
	startTime time.Time,
	statusHandler status.RestoreHandler,
	metadataHandler metadata.RestoreHandler,
) error {
	// prepare the source OCI store
	var srcOCI oras.ReadOnlyGraphTarget
	fi, err := os.Stat(opts.input)
	if err != nil {
		return fmt.Errorf("failed to access input path %q: %w", opts.input, err)
	}
	switch {
	case fi.Mode().IsRegular():
		isTar, err := orasio.IsTarFile(opts.input)
		if err != nil {
			return fmt.Errorf("unable to determine if %q is a tar archive: %w", opts.input, err)
		}
		if !isTar {
			return fmt.Errorf("input path %q is not a tar archive", opts.input)
		}
		srcOCI, err = oci.NewFromTar(ctx, opts.input)
		if err != nil {
			return fmt.Errorf("failed to prepare OCI store from tar archive %q: %w", opts.input, err)
		}
		if err := metadataHandler.OnTarLoaded(opts.input, fi.Size()); err != nil {
			return err
		}
	case fi.IsDir():
		srcOCI, err = oci.NewWithContext(ctx, opts.input)
		if err != nil {
			return fmt.Errorf("failed to prepare OCI store from directory %q: %w", opts.input, err)
		}
	default:
		return fmt.Errorf("input path %q must be a directory or a tar archive", opts.input)
	}

	// resolve tags to restore
	tags, roots, err := resolveTags(ctx, srcOCI, opts.tags)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return &oerrors.Error{
			Err:            fmt.Errorf("no tags found in OCI layout %q", opts.input),
			Recommendation: fmt.Sprintf(`If you want to list available tags in %q, use "oras repo tags --oci-layout"`, opts.input),
		}
	}
	if err := metadataHandler.OnTagsFound(tags); err != nil {
		return err
	}

	// prepare copy options
	copyOpts := oras.DefaultCopyOptions
	copyOpts.Concurrency = opts.concurrency
	copyOpts.PreCopy = statusHandler.PreCopy
	copyOpts.PostCopy = statusHandler.PostCopy
	copyOpts.OnCopySkipped = statusHandler.OnCopySkipped
	extCopyGraphOpts := oras.ExtendedCopyGraphOptions{
		CopyGraphOptions: copyOpts.CopyGraphOptions,
		FindPredecessors: func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return registry.Referrers(ctx, src, desc, "")
		},
	}
	for i, tag := range tags {
		var referrerCount int
		if !opts.excludeReferrers {
			// count referrers from source
			referrerCount, err = countReferrers(ctx, srcOCI, tag, roots[i], extCopyGraphOpts)
			if err != nil {
				return fmt.Errorf("failed to count referrers for tag %q: %w", tag, err)
			}
		}
		if opts.dryRun {
			if err := metadataHandler.OnArtifactPushed(tag, referrerCount); err != nil {
				return err
			}
			// dry run, skip actual copy
			continue
		}

		if err := func() (retErr error) {
			trackedDst, err := statusHandler.StartTracking(dstRepo)
			if err != nil {
				return err
			}
			defer func() {
				stopErr := statusHandler.StopTracking()
				if retErr == nil {
					retErr = stopErr
				}
			}()

			if opts.excludeReferrers {
				_, err := oras.Copy(ctx, srcOCI, tag, trackedDst, tag, copyOpts)
				return err
			}
			return recursiveCopy(ctx, srcOCI, trackedDst, tag, roots[i], extCopyGraphOpts)
		}(); err != nil {
			return fmt.Errorf("failed to restore tag %q from %q to %q: %w", tag, opts.input, opts.repository, oerrors.UnwrapCopyError(err))
		}

		if err := metadataHandler.OnArtifactPushed(tag, referrerCount); err != nil {
			return err
		}
	}

	duration := time.Since(startTime)
	return metadataHandler.OnRestoreCompleted(len(tags), opts.repository, duration)
}
