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
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/graph"
	orasio "oras.land/oras/internal/io"
)

// outputFormat defines the format of the backup output.
type outputFormat int

const (
	// outputFormatDir indicates the output is a directory.
	outputFormatDir outputFormat = iota
	// outputFormatTar indicates the output is a tar archive.
	outputFormatTar
)

// errTagListNotSupported is returned when the target does not support tag listing.
var errTagListNotSupported = errors.New("the target does not support tag listing")

type backupOptions struct {
	option.Common
	option.Remote
	option.Terminal

	// flags
	output           string
	includeReferrers bool
	concurrency      int

	// derived options
	outputFormat outputFormat
	repository   string
	tags         []string
}

func backupCmd() *cobra.Command {
	var opts backupOptions
	cmd := &cobra.Command{
		Use:   "backup [flags] --output <path> <registry>/<repository>[:<ref1>[,<ref2>...]]",
		Short: "[Experimental] Back up artifacts from a registry into an OCI image layout",
		Long: `[Experimental] Back up artifacts from a registry into an OCI image layout, saved either as a directory or a tar archive.
The output format is determined by the file extension of the specified output path: if it ends with ".tar", the output will be a tar archive; otherwise, it will be a directory.

Example - Back up a single artifact to a directory:
  oras backup --output hello localhost:5000/hello:v1

Example - Back up to a tar archive:
  oras backup --output hello.tar localhost:5000/hello:v1

Example - Back up an artifact along with its referrers (e.g. attestations, SBOMs):
  oras backup --output hello --include-referrers localhost:5000/hello:v1

Example - Back up multiple specific tags:
  oras backup --output hello localhost:5000/hello:v1,v2,v3

Example - Back up all tagged artifacts in a repository:
  oras backup --output hello localhost:5000/hello

Example - Use Referrers API for discovering referrers:
  oras backup --output hello --include-referrers --distribution-spec v1.1-referrers-api localhost:5000/hello:v1

Example - Use Referrers Tag Schema for discovering referrers:
  oras backup --output hello --include-referrers --distribution-spec v1.1-referrers-tag localhost:5000/hello:v1

Example - Back up from an insecure registry:
  oras backup --output hello --insecure localhost:5000/hello:v1

Example - Back up from a registry using plain HTTP (no TLS):
  oras backup --output hello --plain-http localhost:5000/hello:v1

Example - Set custom concurrency level:
  oras backup --output hello --concurrency 6 localhost:5000/hello:v1
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the artifacts to back up"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}

			// parse repo and references
			var err error
			opts.repository, opts.tags, err = parseArtifactReferences(args[0])
			if err != nil {
				return err
			}

			// parse output format
			if strings.EqualFold(filepath.Ext(opts.output), ".tar") {
				opts.outputFormat = outputFormatTar
			} else {
				opts.outputFormat = outputFormatDir
			}

			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = true // always print verbose output
			return runBackup(cmd, &opts)
		},
	}

	// required flags
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "path to the target output, either a tar archive (*.tar) or a directory")
	_ = cmd.MarkFlagRequired("output")
	// optional flags
	cmd.Flags().BoolVarP(&opts.includeReferrers, "include-referrers", "", false, "back up the artifact with its referrers (e.g., attestations, SBOMs)")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 3, "concurrency level")
	opts.EnableDistributionSpecFlag()
	// apply flags
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Remote)
}

func runBackup(cmd *cobra.Command, opts *backupOptions) error {
	if opts.output == "" {
		return errors.New("the output path cannot be empty")
	}
	startTime := time.Now() // start timing the backup process
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	var dstRoot string
	switch opts.outputFormat {
	case outputFormatDir:
		dstRoot = opts.output
	case outputFormatTar:
		// test if the output file can be created and fail early if there is an issue
		fp, err := os.OpenFile(opts.output, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			if fi, statErr := os.Stat(opts.output); statErr == nil && fi.IsDir() {
				return &oerrors.Error{
					Err:            fmt.Errorf("the output path %q already exists and is a directory", opts.output),
					Recommendation: "To back up to a tar archive, please specify a different output file name or remove the existing directory.",
				}
			}
			return fmt.Errorf("unable to create output file %s: %w", opts.output, err)
		}
		if err := fp.Close(); err != nil {
			return fmt.Errorf("unable to close output file %s: %w", opts.output, err)
		}

		// create a temporary directory as the working directory for OCI store
		tempDir, err := os.MkdirTemp("", "oras-backup-*")
		if err != nil {
			return fmt.Errorf("failed to create temporary directory for backup: %w", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				logger.Debugf("failed to remove temporary directory %s: %v", tempDir, err)
			}
		}()
		dstRoot = tempDir
	default:
		// this should not happen, just a safeguard
		return fmt.Errorf("unsupported output format")
	}

	// Prepare copy source and destination
	srcRepo, err := opts.NewRepository(opts.repository, opts.Common, logger)
	if err != nil {
		return fmt.Errorf("failed to prepare repository %s for backup: %w", opts.repository, err)
	}
	dstOCI, err := oci.New(dstRoot)
	if err != nil {
		return fmt.Errorf("failed to prepare OCI store for backup: %w", err)
	}
	statusHandler, metadataHandler := display.NewBackupHandler(opts.Printer, opts.TTY, opts.repository, dstOCI)
	return doBackup(ctx, opts, logger, srcRepo, dstOCI, dstRoot, startTime, statusHandler, metadataHandler)
}

func doBackup(
	ctx context.Context,
	opts *backupOptions,
	logger logrus.FieldLogger,
	srcRepo oras.ReadOnlyGraphTarget,
	dstOCI oras.GraphTarget,
	dstRoot string,
	startTime time.Time,
	statusHandler status.BackupHandler,
	metadataHandler metadata.BackupHandler,
) error {
	// Resolve tags to back up
	tags, roots, err := resolveTags(ctx, srcRepo, opts.tags)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return &oerrors.Error{
			Err:            fmt.Errorf("no tags found in repository %q", opts.repository),
			Recommendation: fmt.Sprintf(`If you want to list available tags in %q, use "oras repo tags"`, opts.repository),
		}
	}
	if err := metadataHandler.OnTagsFound(tags); err != nil {
		return err
	}

	// Prepare copy options
	copyGraphOpts := oras.DefaultCopyGraphOptions
	copyGraphOpts.Concurrency = opts.concurrency
	copyGraphOpts.PreCopy = statusHandler.PreCopy
	copyGraphOpts.PostCopy = statusHandler.PostCopy
	copyGraphOpts.OnCopySkipped = statusHandler.OnCopySkipped
	extCopyGraphOpts := oras.ExtendedCopyGraphOptions{
		CopyGraphOptions: copyGraphOpts,
		FindPredecessors: func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return registry.Referrers(ctx, src, desc, "")
		},
	}

	for i, tag := range tags {
		referrerCount, err := func() (referrerCount int, retErr error) {
			trackedDst, err := statusHandler.StartTracking(dstOCI)
			if err != nil {
				return 0, err
			}
			defer func() {
				stopErr := statusHandler.StopTracking()
				if retErr == nil {
					retErr = stopErr
				}
			}()

			if opts.includeReferrers {
				return backupTagWithReferrers(ctx, srcRepo, trackedDst, tag, roots[i], extCopyGraphOpts)
			}
			return 0, backupTag(ctx, srcRepo, trackedDst, tag, roots[i], copyGraphOpts)
		}()
		if err != nil {
			return fmt.Errorf("failed to back up tag %q from %q to %q: %w", tag, opts.repository, dstRoot, oerrors.UnwrapCopyError(err))
		}
		if err := metadataHandler.OnArtifactPulled(tag, referrerCount); err != nil {
			return err
		}
	}

	if err := finalizeBackupOutput(dstRoot, opts, logger, metadataHandler); err != nil {
		return err
	}
	duration := time.Since(startTime)
	return metadataHandler.OnBackupCompleted(len(tags), opts.output, duration)
}

// backupTag copies the artifact identified by the tag from src to dst.
func backupTag(ctx context.Context, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, tag string, root ocispec.Descriptor, copyGraphOpts oras.CopyGraphOptions) error {
	if err := oras.CopyGraph(ctx, src, dst, root, copyGraphOpts); err != nil {
		return err
	}
	if err := dst.Tag(ctx, root, tag); err != nil {
		return fmt.Errorf("failed to tag %q with %q: %w", root.Digest.String(), tag, err)
	}
	return nil
}

// backupTagWithReferrers copies the artifact identified by tag and its referrers from src to dst.
func backupTagWithReferrers(ctx context.Context, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, tag string, root ocispec.Descriptor, extCopyGraphOpts oras.ExtendedCopyGraphOptions) (int, error) {
	if err := recursiveCopy(ctx, src, dst, tag, root, extCopyGraphOpts); err != nil {
		return 0, err
	}
	return countReferrers(ctx, dst, tag, root, extCopyGraphOpts)
}

// countReferrers counts the total number of referrers for the given artifact identified by tag, including the referrers
// of its children manifests if the artifact is an image index or manifest list.
func countReferrers(ctx context.Context, target oras.ReadOnlyGraphTarget, tag string, root ocispec.Descriptor, extCopyGraphOpts oras.ExtendedCopyGraphOptions) (int, error) {
	referrers, err := graph.RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{root}, extCopyGraphOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to count referrers for tag %q, digest %q: %w", tag, root.Digest.String(), err)
	}
	referrerCount := len(referrers)
	if root.MediaType != ocispec.MediaTypeImageIndex && root.MediaType != docker.MediaTypeManifestList {
		// If the root is not an image index or manifest list, we have counted all referrers
		return referrerCount, nil
	}

	// count referrers of children manifests
	manifestBytes, err := content.FetchAll(ctx, target, root)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch content of tag %q, digest %q: %w", tag, root.Digest.String(), err)
	}
	var index ocispec.Index
	if err = json.Unmarshal(manifestBytes, &index); err != nil {
		return 0, fmt.Errorf("failed to unmarshal index for tag %q, digest %q: %w", tag, root.Digest.String(), err)
	}
	childrenReferrers, err := graph.RecursiveFindReferrers(ctx, target, index.Manifests, extCopyGraphOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to count referrers for children manifests of tag %q, digest %q: %w", tag, root.Digest.String(), err)
	}
	referrerCount += len(childrenReferrers)
	return referrerCount, nil
}

// finalizeBackupOutput finalizes the backup output by removing temporary directories and exporting to a tar archive if needed.
func finalizeBackupOutput(dstRoot string, opts *backupOptions, logger logrus.FieldLogger, metadataHandler metadata.BackupHandler) (returnErr error) {
	// Remove ingest dir for a cleaner output
	ingestDir := filepath.Join(dstRoot, "ingest")
	if err := os.RemoveAll(ingestDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Debugf("failed to remove ingest directory: %v", err)
	}
	if opts.outputFormat != outputFormatTar {
		// If output format is not a tar, we are done
		return nil
	}

	// exporting the backup to a tar archive
	if err := metadataHandler.OnTarExporting(opts.output); err != nil {
		return err
	}
	tarFile, err := os.Create(opts.output)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", opts.output, err)
	}
	defer func() {
		err := tarFile.Close()
		if returnErr == nil {
			returnErr = err
		}
	}()
	if err := orasio.TarDirectory(tarFile, dstRoot); err != nil {
		// remove the output file in case of error
		if err := os.Remove(opts.output); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logger.Debugf("failed to remove output file %s: %v", opts.output, err)
		}
		return fmt.Errorf("failed to create tar archive at %s: %w", opts.output, err)
	}
	fi, err := os.Stat(opts.output)
	if err != nil {
		return fmt.Errorf("failed to stat output file %s: %w", opts.output, err)
	}
	return metadataHandler.OnTarExported(opts.output, fi.Size())
}

// resolveTags resolves tags to their descriptors.
// It returns the resolved tags and their corresponding descriptors.
func resolveTags(ctx context.Context, target oras.ReadOnlyTarget, specifiedTags []string) ([]string, []ocispec.Descriptor, error) {
	var descs []ocispec.Descriptor
	resolve := func(tags []string) error {
		for _, tag := range tags {
			desc, err := oras.Resolve(ctx, target, tag, oras.DefaultResolveOptions)
			if err != nil {
				return fmt.Errorf("failed to resolve tag %q: %w", tag, err)
			}
			descs = append(descs, desc)
		}
		return nil
	}
	if len(specifiedTags) > 0 {
		// resolve the specified tags
		descs = make([]ocispec.Descriptor, 0, len(specifiedTags))
		if err := resolve(specifiedTags); err != nil {
			return nil, nil, err
		}
		return specifiedTags, descs, nil
	}

	// discover all tags in the repository and resolve them
	var tags []string
	tagLister, ok := target.(registry.TagLister)
	if !ok {
		return nil, nil, errTagListNotSupported
	}
	if err := tagLister.Tags(ctx, "", func(gotTags []string) error {
		if err := resolve(gotTags); err != nil {
			return err
		}
		tags = append(tags, gotTags...)
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to find tags: %w", err)
	}
	return tags, descs, nil
}

// parseArtifactReferences parses the input string into a repository
// and a slice of tags.
func parseArtifactReferences(artifactRefs string) (string, []string, error) {
	// validate input
	if artifactRefs == "" {
		return "", nil, errors.New("artifact reference cannot be empty")
	}
	// reject digest references early
	if strings.ContainsRune(artifactRefs, '@') {
		return "", nil, fmt.Errorf("digest references are not supported: %q", artifactRefs)
	}
	refParts := strings.Split(artifactRefs, ",")
	mainRef := refParts[0]
	extraTags := refParts[1:]

	// validate repository
	parsedRepo, err := registry.ParseReference(mainRef)
	if err != nil {
		return "", nil, fmt.Errorf("invalid reference %q: %w", mainRef, err)
	}
	mainTag := parsedRepo.Reference
	parsedRepo.Reference = "" // clear the tag
	repository := parsedRepo.String()
	if mainTag == "" && len(extraTags) == 0 {
		// no tags
		return repository, nil, nil
	}

	// validate each tag
	tags := append([]string{mainTag}, extraTags...)
	for _, tag := range tags {
		if tag == "" {
			return "", nil, fmt.Errorf("empty tag in reference %q", artifactRefs)
		}
		parsedRepo.Reference = tag
		if err := parsedRepo.ValidateReferenceAsTag(); err != nil {
			return "", nil, fmt.Errorf("invalid tag %q in reference %q: %w", tag, artifactRefs, err)
		}
	}
	return repository, tags, nil
}
