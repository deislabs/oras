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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/fileref"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/contentutil"
	"oras.land/oras/internal/dir"
	"oras.land/oras/internal/listener"
	"oras.land/oras/internal/manifestutil"
	"oras.land/oras/internal/registryutil"
)

type pushOptions struct {
	option.Common
	option.Packer
	option.ArtifactPlatform
	option.ImageSpec
	option.Target
	option.Format
	option.Terminal
	option.Recursive

	extraRefs         []string
	manifestConfigRef string
	artifactType      string
	force             bool
	concurrency       int
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func pushCmd() *cobra.Command {
	var opts pushOptions
	cmd := &cobra.Command{
		Use:   "push [flags] <name>[:<tag>[,<tag>][...]] <file>[:<type>] [...]",
		Short: "Push files to a registry or an OCI image layout",
		Long: `Push files to a registry or an OCI image layout

Example - Push file "hi.txt" with media type "application/vnd.oci.image.layer.v1.tar" (default):
  oras push localhost:5000/hello:v1 hi.txt

Example - Push file "hi.txt" and export the pushed manifest to a specified path:
  oras push --export-manifest manifest.json localhost:5000/hello:v1 hi.txt

Example - Push file "hi.txt" with the custom media type "application/vnd.me.hi":
  oras push localhost:5000/hello:v1 hi.txt:application/vnd.me.hi

Example - Push multiple files with different media types:
  oras push localhost:5000/hello:v1 hi.txt:application/vnd.me.hi bye.txt:application/vnd.me.bye

Example - Push file with colon in name "hi:txt" with the default media type:
  oras push localhost:5000/hello:v1 hi:txt:

Example - Push file "hi.txt" with artifact type "application/vnd.example+type":
  oras push --artifact-type application/vnd.example+type localhost:5000/hello:v1 hi.txt

Example - Push file "hi.txt" with config type "application/vnd.me.config":
  oras push --image-spec v1.0 --artifact-type application/vnd.me.config localhost:5000/hello:v1 hi.txt

Example - Push file "hi.txt" with the custom manifest config "config.json" of the custom media type "application/vnd.me.config":
  oras push --config config.json:application/vnd.me.config localhost:5000/hello:v1 hi.txt

Example - [Experimental] Push file "hi.txt" and format output in JSON:
  oras push localhost:5000/hello:v1 hi.txt --format json

Example - [Experimental] Push file "hi.txt" and format output with Go template:
  oras push localhost:5000/hello:v1 hi.txt --format go-template="{{.digest}}"

Example - Push file to the insecure registry:
  oras push --insecure localhost:5000/hello:v1 hi.txt

Example - Push file to the HTTP registry:
  oras push --plain-http localhost:5000/hello:v1 hi.txt

Example - Push repository with manifest annotations:
  oras push --annotation "key=val" localhost:5000/hello:v1

Example - Push repository with manifest annotation file:
  oras push --annotation-file annotation.json localhost:5000/hello:v1

Example - [Experimental] Push artifact to repository with platform:
  oras push --artifact-platform linux/arm/v5 localhost:5000/hello:v1

Example - Push file "hi.txt" with multiple tags:
  oras push localhost:5000/hello:tag1,tag2,tag3 hi.txt

Example - Push file "hi.txt" with multiple tags and concurrency level tuned:
  oras push --concurrency 6 localhost:5000/hello:tag1,tag2,tag3 hi.txt

Example - Push file "hi.txt" into an OCI image layout folder 'layout-dir' with tag 'test':
  oras push --oci-layout layout-dir:test hi.txt

Example - Push file "hi.txt" into an OCI image layout folder 'layout-dir' with tag 'example.com:test':
  oras push example.com:test hi.txt --oci-layout-path layout-dir

Example - [Experimental] Push a directory recursively with hierarchical manifests:
  oras push --recursive localhost:5000/hello:v1 ./mydir

Example - [Experimental] Push a directory recursively with custom max blobs per manifest:
  oras push --recursive --max-blobs-per-manifest 500 localhost:5000/hello:v1 ./mydir
`,
		Args: oerrors.CheckArgs(argument.AtLeast(1), "the destination for pushing"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			refs := strings.Split(args[0], ",")
			opts.RawReference = refs[0]
			opts.extraRefs = refs[1:]
			opts.FileRefs = args[1:]
			// For recursive push the argument is a root directory path, not a
			// file-as-artifact path. The directory path is never stored as an
			// annotation title, so the absolute-path check is irrelevant.
			if opts.Recursive.Recursive {
				opts.PathValidationDisabled = true
			}
			if err := option.Parse(cmd, &opts); err != nil {
				return err
			}
			if err := option.ValidateExtraTags(opts.extraRefs); err != nil {
				return err
			}
			opts.DisableTTY(opts.Debug, false)
			if opts.manifestConfigRef != "" && opts.artifactType == "" {
				if !cmd.Flags().Changed("image-spec") {
					// switch to v1.0 manifest since artifact type is suggested
					// by OCI v1.1 artifact guidance but is not presented
					// see https://github.com/opencontainers/image-spec/blob/e7f7c0ca69b21688c3cea7c87a04e4503e6099e2/manifest.md?plain=1#L170
					opts.Flag = option.ImageSpecV1_0
					opts.PackVersion = oras.PackManifestVersion1_0
				} else if opts.Flag == option.ImageSpecV1_1 {
					return &oerrors.Error{
						Err:            errors.New(`missing artifact type for OCI image-spec v1.1 artifacts`),
						Recommendation: "set an artifact type via `--artifact-type` or consider image spec v1.0",
					}
				}
			}
			configAndPlatform := []string{"config", "artifact-platform"}
			if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), configAndPlatform...); err != nil {
				return err
			}

			switch opts.PackVersion {
			case oras.PackManifestVersion1_0:
				if opts.manifestConfigRef != "" && opts.artifactType != "" {
					return errors.New("--artifact-type and --config cannot both be provided for 1.0 OCI image")
				}
			case oras.PackManifestVersion1_1:
				if opts.manifestConfigRef == "" && opts.artifactType == "" {
					opts.artifactType = oras.MediaTypeUnknownArtifact
				}
			}

			// Validate recursive options
			if err := opts.Validate(); err != nil {
				return err
			}

			if !opts.Recursive.Recursive && cmd.Flags().Changed("max-blobs-per-manifest") {
				return errors.New("--max-blobs-per-manifest requires --recursive")
			}

			// Recursive push has some restrictions
			if opts.Recursive.Recursive {
				if opts.manifestConfigRef != "" {
					return errors.New("--config cannot be used with --recursive")
				}
				if opts.Platform.Platform != nil {
					return errors.New("--artifact-platform cannot be used with --recursive")
				}
				if opts.Flag == option.ImageSpecV1_0 {
					return errors.New("--image-spec v1.0 cannot be used with --recursive: hierarchical manifests require OCI image-spec v1.1")
				}
				if opts.artifactType != "" {
					if err := manifestutil.ValidateMediaType(opts.artifactType); err != nil {
						return fmt.Errorf("invalid artifact type: %w", err)
					}
				}
				if len(opts.FileRefs) != 1 {
					return errors.New("--recursive requires exactly one directory argument")
				}
				// Verify the argument is a directory
				info, err := os.Stat(opts.FileRefs[0])
				if err != nil {
					return fmt.Errorf("cannot access %q: %w", opts.FileRefs[0], err)
				}
				if !info.IsDir() {
					return fmt.Errorf("%q is not a directory; --recursive requires a directory", opts.FileRefs[0])
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = opts.verbose
			if opts.Recursive.Recursive {
				return runPushRecursive(cmd, &opts)
			}
			return runPush(cmd, &opts)
		},
	}
	cmd.Flags().StringVarP(&opts.manifestConfigRef, "config", "", "", "`path` of image config file")
	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "artifact type")
	cmd.Flags().BoolVarP(&opts.force, "force", "", false, "force a deep traversal of the destination graph before tagging the root; useful when the destination is partially populated (e.g. by a registry cache)")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 5, "concurrency level")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "print status output for unnamed blobs")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	opts.SetTypes(option.FormatTypeText, option.FormatTypeJSON, option.FormatTypeGoTemplate)
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func runPush(cmd *cobra.Command, opts *pushOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	// prepare pack
	packOpts := oras.PackManifestOptions{
		ConfigAnnotations:   opts.Annotations[option.AnnotationConfig],
		ManifestAnnotations: opts.Annotations[option.AnnotationManifest],
	}
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if opts.manifestConfigRef != "" {
		path, cfgMediaType, err := fileref.Parse(opts.manifestConfigRef, oras.MediaTypeUnknownConfig)
		if err != nil {
			return err
		}
		desc, err := addFile(ctx, store, option.AnnotationConfig, cfgMediaType, path)
		if err != nil {
			return err
		}
		desc.Annotations = packOpts.ConfigAnnotations
		packOpts.ConfigDescriptor = &desc
	} else if opts.Platform.Platform != nil {
		blob, err := json.Marshal(opts.Platform.Platform)
		if err != nil {
			return err
		}
		if opts.Flag == option.ImageSpecV1_0 && opts.artifactType != "" {
			return &oerrors.Error{
				Err:            errors.New(`artifact type cannot be customized for OCI image-spec v1.0 when platform is specified`),
				Recommendation: "consider using image spec v1.1 or remove --artifact-type",
			}
		}
		desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageConfig, blob)
		err = store.Push(ctx, desc, bytes.NewReader(blob))
		if err != nil {
			return err
		}
		desc.Annotations = packOpts.ConfigAnnotations
		packOpts.ConfigDescriptor = &desc
	}
	memoryStore := memory.New()
	union := contentutil.MultiReadOnlyTarget(memoryStore, store)
	statusHandler, metadataHandler, err := display.NewPushHandler(opts.Printer, opts.Format, opts.TTY, union)
	if err != nil {
		return err
	}
	descs, err := loadFiles(ctx, store, opts.Annotations, opts.FileRefs, statusHandler)
	if err != nil {
		return err
	}
	packOpts.Layers = descs
	pack := func() (ocispec.Descriptor, error) {
		root, err := oras.PackManifest(ctx, memoryStore, opts.PackVersion, opts.artifactType, packOpts)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		if err = memoryStore.Tag(ctx, root, root.Digest.String()); err != nil {
			return ocispec.Descriptor{}, err
		}
		return root, nil
	}

	// prepare push
	originalDst, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	// Keep the unwrapped destination for scope hinting; WithScopeHint relies
	// on a concrete *remote.Repository type assertion and would silently
	// skip the hint if passed the TraversingTarget wrapper.
	scopeHintDst := originalDst
	if opts.force {
		originalDst = &contentutil.TraversingTarget{GraphTarget: originalDst}
	}
	dst, stopTrack, err := statusHandler.TrackTarget(originalDst)
	if err != nil {
		return err
	}
	copyOptions := oras.DefaultCopyOptions
	copyOptions.Concurrency = opts.concurrency
	copyOptions.OnCopySkipped = statusHandler.OnCopySkipped
	copyOptions.PreCopy = statusHandler.PreCopy
	copyOptions.PostCopy = statusHandler.PostCopy
	copyWithScopeHint := func(root ocispec.Descriptor) error {
		// add both pull and push scope hints for dst repository
		// to save potential push-scope token requests during copy
		ctx = registryutil.WithScopeHint(ctx, scopeHintDst, auth.ActionPull, auth.ActionPush)

		if tag := opts.Reference; tag == "" {
			err = oras.CopyGraph(ctx, union, dst, root, copyOptions.CopyGraphOptions)
		} else {
			_, err = oras.Copy(ctx, union, root.Digest.String(), dst, tag, copyOptions)
		}
		return oerrors.UnwrapCopyError(err) // we don't need the CopyError information so we unwrap it here
	}

	// Push
	root, err := doPush(dst, stopTrack, pack, copyWithScopeHint)
	if err != nil {
		return err
	}
	err = metadataHandler.OnCopied(&opts.Target, root)
	if err != nil {
		return err
	}

	if len(opts.extraRefs) != 0 {
		contentBytes, err := content.FetchAll(ctx, memoryStore, root)
		if err != nil {
			return err
		}
		tagBytesNOpts := oras.DefaultTagBytesNOptions
		tagBytesNOpts.Concurrency = opts.concurrency
		dst := listener.NewTagListener(originalDst, nil, metadataHandler.OnTagged)
		if _, err = oras.TagBytesN(ctx, dst, root.MediaType, contentBytes, opts.extraRefs, tagBytesNOpts); err != nil {
			return err
		}
	}

	err = metadataHandler.Render()
	if err != nil {
		return err
	}

	// Export manifest
	return opts.ExportManifest(ctx, memoryStore, root)
}

func doPush(dst oras.Target, stopTrack status.StopTrackTargetFunc, pack packFunc, copyFunc copyFunc) (ocispec.Descriptor, error) {
	defer func() {
		_ = stopTrack()
	}()
	// Push
	return pushArtifact(dst, pack, copyFunc)
}

type packFunc func() (ocispec.Descriptor, error)
type copyFunc func(desc ocispec.Descriptor) error

func pushArtifact(_ oras.Target, pack packFunc, copyFunc copyFunc) (ocispec.Descriptor, error) {
	root, err := pack()
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	// push
	if err = copyFunc(root); err != nil {
		return ocispec.Descriptor{}, err
	}
	return root, nil
}

// runPushRecursive handles recursive directory push with hierarchical manifests.
func runPushRecursive(cmd *cobra.Command, opts *pushOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	dirPath := opts.FileRefs[0]

	// Walk the directory tree
	walkOpts := dir.WalkOptions{
		FollowSymlinks: opts.FollowSymlinks,
		IncludeEmpty:   opts.PreserveEmptyDirs,
		OnSkip: func(path string) {
			logger.Warnf("skipped symbolic link %q; use --follow-symlinks to follow symbolic links", path)
		},
	}
	rootNode, err := dir.Walk(dirPath, walkOpts)
	if err != nil {
		return fmt.Errorf("failed to walk directory %q: %w", dirPath, err)
	}

	fileCount := rootNode.FileCount()
	if fileCount == 0 && !opts.PreserveEmptyDirs {
		return errors.New("directory is empty; nothing to push")
	}

	// Create file store
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	memoryStore := memory.New()
	union := contentutil.MultiReadOnlyTarget(memoryStore, store)

	statusHandler, metadataHandler, err := display.NewPushHandler(opts.Printer, opts.Format, opts.TTY, union)
	if err != nil {
		return err
	}

	// Load all files and get their descriptors
	allFiles := dir.FlattenFiles(rootNode)
	fileDescs := make(map[string]ocispec.Descriptor)

	for _, fileNode := range allFiles {
		// Resolve the annotated name before registering the file, like
		// loadFiles does: the store must know the file by the name used in
		// the descriptor, or copying fails with "not found".
		name := fileNode.Path
		annotations := opts.Annotations[fileNode.Path]
		if title, ok := annotations[ocispec.AnnotationTitle]; ok {
			name = title
		}

		if err := statusHandler.OnFileLoading(name); err != nil {
			return err
		}
		desc, err := addFile(ctx, store, name, "application/octet-stream", fileNode.AbsPath)
		if err != nil {
			return fmt.Errorf("failed to add file %q: %w", fileNode.Path, err)
		}

		// Apply annotations if any
		if len(annotations) > 0 {
			if desc.Annotations == nil {
				desc.Annotations = make(map[string]string)
			}
			maps.Copy(desc.Annotations, annotations)
		}

		fileDescs[fileNode.Path] = desc
	}

	// Build the manifest hierarchy
	builder := manifestutil.NewBuilder(memoryStore, manifestutil.BuilderOptions{
		MaxBlobsPerManifest: opts.MaxBlobsPerManifest,
		ArtifactType:        opts.artifactType,
		ManifestAnnotations: opts.Annotations[option.AnnotationManifest],
		ConfigAnnotations:   opts.Annotations[option.AnnotationConfig],
		PreserveEmptyDirs:   opts.PreserveEmptyDirs,
	})

	result, err := builder.BuildFromNode(ctx, rootNode, fileDescs)
	if err != nil {
		return fmt.Errorf("failed to build manifest hierarchy: %w", err)
	}

	// Tag the root in memory store so it can be resolved
	if err := memoryStore.Tag(ctx, result.Root, result.Root.Digest.String()); err != nil {
		return fmt.Errorf("failed to tag root: %w", err)
	}

	// Prepare destination
	originalDst, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	// Keep the unwrapped destination for scope hinting; WithScopeHint relies
	// on a concrete *remote.Repository type assertion and would silently
	// skip the hint if passed the TraversingTarget wrapper.
	scopeHintDst := originalDst
	if opts.force {
		originalDst = &contentutil.TraversingTarget{GraphTarget: originalDst}
	}
	dst, stopTrack, err := statusHandler.TrackTarget(originalDst)
	if err != nil {
		return err
	}

	// Copy options
	copyOptions := oras.DefaultCopyOptions
	copyOptions.Concurrency = opts.concurrency
	copyOptions.OnCopySkipped = statusHandler.OnCopySkipped
	copyOptions.PreCopy = statusHandler.PreCopy
	copyOptions.PostCopy = statusHandler.PostCopy

	// Add scope hints
	ctx = registryutil.WithScopeHint(ctx, scopeHintDst, auth.ActionPull, auth.ActionPush)

	// Copy the graph
	if tag := opts.Reference; tag == "" {
		err = oras.CopyGraph(ctx, union, dst, result.Root, copyOptions.CopyGraphOptions)
	} else {
		_, err = oras.Copy(ctx, union, result.Root.Digest.String(), dst, tag, copyOptions)
	}
	// Stop progress tracking before rendering any summary output.
	_ = stopTrack()
	if err != nil {
		return oerrors.UnwrapCopyError(err)
	}

	if err := metadataHandler.OnCopied(&opts.Target, result.Root); err != nil {
		return err
	}

	// Handle extra tags
	if len(opts.extraRefs) != 0 {
		contentBytes, err := content.FetchAll(ctx, memoryStore, result.Root)
		if err != nil {
			return err
		}
		tagBytesNOpts := oras.DefaultTagBytesNOptions
		tagBytesNOpts.Concurrency = opts.concurrency
		tagDst := listener.NewTagListener(originalDst, nil, metadataHandler.OnTagged)
		if _, err = oras.TagBytesN(ctx, tagDst, result.Root.MediaType, contentBytes, opts.extraRefs, tagBytesNOpts); err != nil {
			return err
		}
	}

	if err := metadataHandler.Render(); err != nil {
		return err
	}

	// Print summary. Text format only: JSON and Go-template output must not
	// be followed by extra plain text.
	if opts.Format.Type == option.FormatTypeText.Name {
		opts.Printer.Println("Pushed", fileCount, "files in", result.ManifestCount, "manifest(s) and", result.IndexCount, "index(es)")
	}

	// Export manifest if requested
	return opts.ExportManifest(ctx, memoryStore, result.Root)
}
