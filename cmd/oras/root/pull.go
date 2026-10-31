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
	"os"
	"path/filepath"
	"strings"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/fileref"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/descriptor"
	"oras.land/oras/internal/graph"
)

type pullOptions struct {
	option.Cache
	option.Common
	option.Platform
	option.Target
	option.Format
	option.Terminal

	concurrency       int
	KeepOldFiles      bool
	IncludeSubject    bool
	PathTraversal     bool
	Output            string
	ManifestConfigRef string
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func pullCmd() *cobra.Command {
	var opts pullOptions
	cmd := &cobra.Command{
		Use:   "pull [flags] <name>{:<tag>|@<digest>}",
		Short: "Pull files from a registry or an OCI image layout",
		Long: `Pull files from a registry or an OCI image layout

Example - Pull artifact files from a registry:
  oras pull localhost:5000/hello:v1

Example - Recursively pulling all files from a registry, including subjects of hello:v1:
  oras pull --include-subject localhost:5000/hello:v1

Example - Pull files from an insecure registry:
  oras pull --insecure localhost:5000/hello:v1

Example - Pull files from the HTTP registry:
  oras pull --plain-http localhost:5000/hello:v1

Example - Pull files from a registry with local cache:
  export ORAS_CACHE=~/.oras/cache
  oras pull localhost:5000/hello:v1

Example - Pull files from a registry with certain platform:
  oras pull --platform linux/arm/v5 localhost:5000/hello:v1

Example - Pull all files with concurrency level tuned:
  oras pull --concurrency 6 localhost:5000/hello:v1

Example - [Experimental] Pull files and format output in JSON:
  oras pull localhost:5000/hello:v1 --format json

Example - [Experimental] Pull files and format output with Go template:
  oras pull localhost:5000/hello:v1 --format go-template="{{.reference}}"

Example - Pull artifact files from an OCI image layout folder 'layout-dir':
  oras pull --oci-layout layout-dir:v1

Example - Pull artifact files from an OCI layout archive 'layout.tar':
  oras pull --oci-layout layout.tar:v1

Example - Pull artifact files tagged 'example.com:v1' from an OCI image layout folder 'layout-dir':
  oras pull example.com:v1 --oci-layout-path layout-dir
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the artifact reference you want to pull"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RawReference = args[0]
			err := option.Parse(cmd, &opts)
			if err != nil {
				return err
			}
			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = opts.verbose
			return runPull(cmd, &opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.KeepOldFiles, "keep-old-files", "k", false, "do not replace existing files when pulling, treat them as errors")
	cmd.Flags().BoolVarP(&opts.PathTraversal, "allow-path-traversal", "T", false, "allow storing files out of the output directory")
	cmd.Flags().BoolVarP(&opts.IncludeSubject, "include-subject", "", false, "recursively pull the subject of artifacts")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", ".", "output directory")
	cmd.Flags().StringVarP(&opts.ManifestConfigRef, "config", "", "", "output manifest config file")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 3, "concurrency level")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "print status output for unnamed blobs")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	opts.SetTypes(option.FormatTypeText, option.FormatTypeJSON, option.FormatTypeGoTemplate)
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

type pullCleanup struct {
	mu      sync.Mutex
	root    string
	seen    map[string]struct{}
	created map[string]struct{}
}

func newPullCleanup(output string) (*pullCleanup, error) {
	root, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	return &pullCleanup{
		root:    root,
		seen:    make(map[string]struct{}),
		created: make(map[string]struct{}),
	}, nil
}

// track marks name for cleanup if it does not exist yet.
// It must be called before name is written.
func (c *pullCleanup) track(name string) {
	if !filepath.IsLocal(name) {
		// outside the output directory: not ours to remove
		return
	}
	path := filepath.Join(c.root, name)
	if path == c.root {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[path]; ok {
		return
	}
	c.seen[path] = struct{}{}
	outermost := ""
	for p := path; p != c.root && filepath.Dir(p) != p; p = filepath.Dir(p) {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			break
		}
		outermost = p
	}
	if outermost != "" {
		c.created[outermost] = struct{}{}
	}
}

func (c *pullCleanup) cleanup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	for path := range c.created {
		err = errors.Join(err, os.RemoveAll(path))
	}
	return err
}

func runPull(cmd *cobra.Command, opts *pullOptions) (pullError error) {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	statusHandler, metadataHandler, err := display.NewPullHandler(opts.Printer, opts.Format, opts.Path, opts.TTY)
	if err != nil {
		return err
	}
	// Copy Options
	copyOptions := oras.DefaultCopyOptions
	copyOptions.Concurrency = opts.concurrency
	if opts.Platform.Platform != nil {
		copyOptions.WithTargetPlatform(opts.Platform.Platform)
	}
	target, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}
	src, err := opts.CachedTarget(target)
	if err != nil {
		return err
	}

	cleanup, err := newPullCleanup(opts.Output)
	if err != nil {
		return err
	}

	dst, err := file.New(opts.Output)
	if err != nil {
		return err
	}

	defer func() {
		if err := dst.Close(); pullError == nil {
			pullError = err
		}
	}()
	dst.AllowPathTraversalOnWrite = opts.PathTraversal
	dst.DisableOverwrite = opts.KeepOldFiles

	desc, err := doPull(ctx, src, dst, copyOptions, metadataHandler, statusHandler, opts, cleanup)
	if err != nil {
		if cleanupErr := cleanup.cleanup(); cleanupErr != nil {
			logger.Warnf("failed to remove partial output: %v", cleanupErr)
		}

		if !errors.Is(err, file.ErrPathTraversalDisallowed) {
			return err
		}
		// customize friendly message for path traversal error
		return &oerrors.Error{
			Err:            err,
			Recommendation: `Pulling files outside of working directory is insecure and blocked by default. If you trust the content producer, use --allow-path-traversal to bypass this check.`,
		}
	}
	metadataHandler.OnPulled(&opts.Target, desc)
	return metadataHandler.Render()
}

func doPull(ctx context.Context, src oras.ReadOnlyTarget, dst oras.GraphTarget, opts oras.CopyOptions, metadataHandler metadata.PullHandler, statusHandler status.PullHandler, po *pullOptions, cleanup *pullCleanup) (ocispec.Descriptor, error) {
	var configPath, configMediaType string
	var err error

	if po.ManifestConfigRef != "" {
		configPath, configMediaType, err = fileref.Parse(po.ManifestConfigRef, "")
		if err != nil {
			return ocispec.Descriptor{}, err
		}
	}
	dst, stopTrack, err := statusHandler.TrackTarget(dst)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	defer func() {
		_ = stopTrack()
	}()
	var printed sync.Map
	var getConfigOnce sync.Once
	opts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		statusFetcher := content.FetcherFunc(func(ctx context.Context, target ocispec.Descriptor) (fetched io.ReadCloser, fetchErr error) {
			if _, ok := printed.LoadOrStore(descriptor.GenerateContentKey(target), true); ok {
				return fetcher.Fetch(ctx, target)
			}
			if err := statusHandler.OnNodeDownloading(target); err != nil {
				return nil, err
			}
			rc, err := fetcher.Fetch(ctx, target)
			if err != nil {
				return nil, err
			}
			defer func() {
				if fetchErr != nil {
					_ = rc.Close()
				}
			}()
			return rc, statusHandler.OnNodeProcessing(target)
		})

		nodes, subject, config, err := graph.Successors(ctx, statusFetcher, desc)
		if err != nil {
			return nil, err
		}
		if subject != nil && po.IncludeSubject {
			nodes = append(nodes, *subject)
		}
		if config != nil {
			getConfigOnce.Do(func() {
				if configPath != "" && (configMediaType == "" || config.MediaType == configMediaType) {
					if config.Annotations == nil {
						config.Annotations = make(map[string]string)
					}
					config.Annotations[ocispec.AnnotationTitle] = configPath
				}
			})
			if config.Size != ocispec.DescriptorEmptyJSON.Size || config.Digest != ocispec.DescriptorEmptyJSON.Digest || config.Annotations[ocispec.AnnotationTitle] != "" {
				nodes = append(nodes, *config)
			}
		}

		var ret []ocispec.Descriptor
		for _, s := range nodes {
			if s.Annotations[ocispec.AnnotationTitle] == "" {
				if content.Equal(s, ocispec.DescriptorEmptyJSON) {
					// empty layer
					continue
				}
				ss, err := content.Successors(ctx, fetcher, s)
				if err != nil {
					return nil, err
				}
				if len(ss) == 0 {
					// skip s if it is unnamed AND has no successors; unnamed
					// manifests and indexes are still traversed, not skipped.
					if err = metadataHandler.OnLayerSkipped(s); err != nil {
						return nil, err
					}
					if err := notifyOnce(&printed, s, statusHandler.OnNodeSkipped); err != nil {
						return nil, err
					}
					continue
				}
			}
			ret = append(ret, s)
		}
		for _, s := range ret {
			if name := s.Annotations[ocispec.AnnotationTitle]; name != "" {
				cleanup.track(name)
			}
		}
		return ret, nil
	}

	opts.PreCopy = func(_ context.Context, desc ocispec.Descriptor) error {
		return notifyOnce(&printed, desc, statusHandler.OnNodeDownloading)
	}
	opts.PostCopy = func(ctx context.Context, desc ocispec.Descriptor) error {
		// restore named but deduplicated successor nodes
		successors, err := content.Successors(ctx, dst, desc)
		if err != nil {
			return err
		}
		for _, s := range successors {
			if name, ok := s.Annotations[ocispec.AnnotationTitle]; ok {
				if err = metadataHandler.OnFilePulled(name, po.Output, s, po.Path); err != nil {
					return err
				}

				if err = notifyOnce(&printed, s, statusHandler.OnNodeRestored); err != nil {
					return err
				}
			}
			// Restore directories recorded by recursive push. File writes
			// create their parent directories implicitly; this is what
			// restores empty directories.
			if dirPath, ok := s.Annotations[descriptor.AnnotationDirectoryPath]; ok {
				if err = restoreDirectory(po.Output, dirPath, po.PathTraversal, cleanup); err != nil {
					return err
				}
			}
		}
		printed.Store(descriptor.GenerateContentKey(desc), true)
		return statusHandler.OnNodeDownloaded(desc)
	}

	// Copy
	desc, err := oras.Copy(ctx, src, po.Reference, dst, po.Reference, opts)
	return desc, oerrors.UnwrapCopyError(err) // we don't need the CopyError information so we unwrap it here
}

// restoreDirectory creates the directory recorded in a descriptor's directory
// path annotation under the output directory. The annotation value comes from
// remote content, so it is held to the same path-traversal policy as file
// names.
func restoreDirectory(output, dirPath string, allowPathTraversal bool, cleanup *pullCleanup) error {
	if dirPath == "" || dirPath == "." {
		// the root of the pushed directory is the output directory itself
		return nil
	}
	rel := filepath.Clean(filepath.FromSlash(dirPath))
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(output, rel)
	}
	if !allowPathTraversal {
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("%s: %w", dirPath, file.ErrPathTraversalDisallowed)
		}
		// The lexical check above does not resolve symlinks: os.MkdirAll
		// follows them, so an existing symlink component under the output
		// directory would redirect creation outside it. Re-check after
		// resolving symlinks, like the file store does for file writes.
		if err := checkDirectoryEscape(output, target); err != nil {
			return fmt.Errorf("%s: %w", dirPath, err)
		}
	}
	cleanup.track(rel)
	return os.MkdirAll(target, 0777)
}

// checkDirectoryEscape returns file.ErrPathTraversalDisallowed if resolving
// symlinks in target's existing ancestors causes it to escape base. It mirrors
// the symlink-aware containment check the file store applies to file writes.
func checkDirectoryEscape(base, target string) error {
	// Normalize both paths to absolute before resolving symlinks: with a
	// relative output directory, a safe absolute symlink target inside it
	// would otherwise be compared against a relative base and filepath.Rel
	// would report traversal.
	base, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		if os.IsNotExist(err) {
			// base does not exist yet; no symlinks to follow
			return nil
		}
		return err
	}
	// resolve symlinks in the deepest existing ancestor of target and append
	// the non-existent components verbatim
	realTarget := target
	dir := target
	var suffix string
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			realTarget = filepath.Join(resolved, suffix)
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// reached the filesystem root
			break
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		dir = parent
	}
	rel, err := filepath.Rel(realBase, realTarget)
	if err != nil {
		return file.ErrPathTraversalDisallowed
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return file.ErrPathTraversalDisallowed
	}
	return nil
}

func notifyOnce(notified *sync.Map, s ocispec.Descriptor, notify func(ocispec.Descriptor) error) error {
	if _, loaded := notified.LoadOrStore(descriptor.GenerateContentKey(s), true); !loaded {
		return notify(s)
	}
	return nil
}
