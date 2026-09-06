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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/file"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/fileref"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/graph"
	"oras.land/oras/internal/registryutil"
)

type attachOptions struct {
	option.Common
	option.Packer
	option.Target
	option.Format
	option.Platform
	option.Terminal

	artifactType      string
	manifestConfigRef string
	concurrency       int
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func attachCmd() *cobra.Command {
	var opts attachOptions
	cmd := &cobra.Command{
		Use:   "attach [flags] --artifact-type=<type> <name>{:<tag>|@<digest>} {<file>[:<layer_media_type>]|--annotation <key>=<value>} [...]",
		Short: "Attach files to an existing artifact",
		Long: `Attach files to an existing artifact

Example - Attach file 'hi.txt' with artifact type 'doc/example' to manifest 'hello:v1' in registry 'localhost:5000':
  oras attach --artifact-type doc/example localhost:5000/hello:v1 hi.txt

Example - Attach file 'hi.txt' to a specific artifact with platform 'linux/amd64' in multi-arch index 'hello:v1'
  oras attach --artifact-type doc/example --platform linux/amd64 localhost:5000/hello:v1 hi.txt

Example - Push file "hi.txt" with the custom layer media type 'application/vnd.me.hi':
  oras attach --artifact-type doc/example localhost:5000/hello:v1 hi.txt:application/vnd.me.hi

Example - Attach file "hi.txt" using a specific method for the Referrers API:
  oras attach --artifact-type doc/example --distribution-spec v1.1-referrers-api localhost:5000/hello:v1 hi.txt # via API
  oras attach --artifact-type doc/example --distribution-spec v1.1-referrers-tag localhost:5000/hello:v1 hi.txt # via tag scheme

Example - Attach file 'hi.txt' and add annotations from file 'annotation.json':
  oras attach --artifact-type doc/example --annotation-file annotation.json localhost:5000/hello:v1 hi.txt

Example - Attach an artifact with manifest annotations:
  oras attach --artifact-type doc/example --annotation "key1=val1" --annotation "key2=val2" localhost:5000/hello:v1

Example - Attach file 'hi.txt' and add manifest annotations:
  oras attach --artifact-type doc/example --annotation "key=val" localhost:5000/hello:v1 hi.txt

Example - [Experimental] Attach file 'hi.txt' and format output in JSON:
  oras attach --artifact-type doc/example localhost:5000/hello:v1 hi.txt --format json

Example - [Experimental] Attach file 'hi.txt' and format output with Go template:
  oras attach --artifact-type doc/example localhost:5000/hello:v1 hi.txt --format go-template --template "{{.digest}}"

Example - Attach file 'hi.txt' and export the pushed manifest to 'manifest.json':
  oras attach --artifact-type doc/example --export-manifest manifest.json localhost:5000/hello:v1 hi.txt

Example - Attach file to the manifest tagged 'v1' in an OCI image layout folder 'layout-dir':
  oras attach --oci-layout --artifact-type doc/example layout-dir:v1 hi.txt

Example - Attach file to the manifest tagged 'example.com:v1' in an OCI image layout folder 'layout-dir':
  oras attach --artifact-type doc/example --oci-layout-path layout-dir example.com:v1 hi.txt

Example - Attach file 'hi.txt' with the custom manifest config "config.json" of the custom media type "application/vnd.me.config":
  oras attach --artifact-type doc/example --config config.json:application/vnd.me.config localhost:5000/hello:v1 hi.txt
`,
		Args: oerrors.CheckArgs(argument.AtLeast(1), "the destination artifact for attaching."),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RawReference = args[0]
			opts.FileRefs = args[1:]
			err := option.Parse(cmd, &opts)
			if err == nil {
				opts.DisableTTY(opts.Debug, false)
				if err = oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), "config", "platform"); err != nil {
					return err
				}
				if err = opts.EnsureReferenceNotEmpty(cmd, true); err == nil {
					return nil
				}
			}
			if len(opts.FileRefs) == 0 {
				// no file argument provided
				var oerr *oerrors.Error
				if errors.As(err, &oerr) && oerr.OperationType == oerrors.OperationTypeParseArtifactReference {
					// invalid reference
					oerr.Recommendation = fmt.Sprintf("Are you missing an artifact reference to attach to? %s", oerr.Recommendation)
				}
			}
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = opts.verbose
			return runAttach(cmd, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.artifactType, "artifact-type", "", "", "artifact type")
	cmd.Flags().StringVarP(&opts.manifestConfigRef, "config", "", "", "`path` of image config file")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 5, "concurrency level")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "print status output for unnamed blobs")
	opts.FlagDescription = "attach to an arch-specific subject"
	_ = cmd.MarkFlagRequired("artifact-type")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	opts.EnableDistributionSpecFlag()
	opts.SetTypes(option.FormatTypeText, option.FormatTypeJSON, option.FormatTypeGoTemplate)
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

// buildAttachPackOpts assembles a PackManifestOptions from the resolved
// attach options, subject descriptor, and layer descriptors.
func buildAttachPackOpts(opts *attachOptions, subject ocispec.Descriptor, descs []ocispec.Descriptor) oras.PackManifestOptions {
	return oras.PackManifestOptions{
		Subject:             &subject,
		ConfigAnnotations:   opts.Annotations[option.AnnotationConfig],
		ManifestAnnotations: opts.Annotations[option.AnnotationManifest],
		Layers:              descs,
	}
}

func runAttach(cmd *cobra.Command, opts *attachOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	if len(opts.FileRefs) == 0 && len(opts.Annotations[option.AnnotationManifest]) == 0 {
		return &oerrors.Error{
			Err:            errors.New(`neither file nor annotation provided in the command`),
			Usage:          fmt.Sprintf("%s %s", cmd.Parent().CommandPath(), cmd.Use),
			Recommendation: `To attach to an existing artifact, please provide files via argument or annotations via flag "--annotation". Run "oras attach -h" for more options and examples`,
		}
	}

	// prepare manifest
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	dst, err := opts.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	// add both pull and push scope hints for dst repository
	// to save potential push-scope token requests during copy
	ctx = registryutil.WithScopeHint(ctx, dst, auth.ActionPull, auth.ActionPush)
	fetchOpts := oras.DefaultResolveOptions
	fetchOpts.TargetPlatform = opts.Platform.Platform
	subject, err := oras.Resolve(ctx, dst, opts.Reference, fetchOpts)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", opts.Reference, err)
	}
	statusHandler, metadataHandler, err := display.NewAttachHandler(opts.Printer, opts.Format, opts.TTY, store)
	if err != nil {
		return err
	}
	return doAttach(ctx, opts, store, dst, subject, statusHandler, metadataHandler)
}

func doAttach(ctx context.Context, opts *attachOptions, store *file.Store, dst oras.GraphTarget, subject ocispec.Descriptor, statusHandler status.AttachHandler, metadataHandler metadata.AttachHandler) error {
	descs, err := loadFiles(ctx, store, opts.Annotations, opts.FileRefs, statusHandler)
	if err != nil {
		return err
	}

	// prepare push
	dst, stopTrack, err := statusHandler.TrackTarget(dst)
	if err != nil {
		return err
	}
	graphCopyOptions := oras.DefaultCopyGraphOptions
	graphCopyOptions.Concurrency = opts.concurrency
	graphCopyOptions.OnCopySkipped = statusHandler.OnCopySkipped
	graphCopyOptions.PreCopy = statusHandler.PreCopy
	graphCopyOptions.PostCopy = statusHandler.PostCopy

	packOpts := buildAttachPackOpts(opts, subject, descs)
	if opts.manifestConfigRef != "" {
		path, cfgMediaType, err := fileref.Parse(opts.manifestConfigRef, oras.MediaTypeUnknownConfig)
		if err != nil {
			return err
		}
		desc, err := addFile(ctx, store, option.AnnotationConfig, cfgMediaType, path)
		if err != nil {
			return err
		}
		desc.Annotations = opts.Annotations[option.AnnotationConfig]
		packOpts.ConfigDescriptor = &desc
	}
	pack := func() (ocispec.Descriptor, error) {
		return oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, opts.artifactType, packOpts)
	}

	copyFunc := func(root ocispec.Descriptor) error {
		graphCopyOptions.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			if content.Equal(node, root) {
				// skip duplicated Resolve on subject
				successors, _, config, err := graph.Successors(ctx, fetcher, node)
				if err != nil {
					return nil, err
				}
				if config != nil {
					successors = append(successors, *config)
				}
				return successors, nil
			}
			return content.Successors(ctx, fetcher, node)
		}
		err := oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
		return oerrors.UnwrapCopyError(err) // we don't need the CopyError information so we unwrap it here
	}

	// Attach
	root, err := doPush(dst, stopTrack, pack, copyFunc)
	if err != nil {
		return err
	}
	metadataHandler.OnAttached(&opts.Target, root, subject)
	err = metadataHandler.Render()
	if err != nil {
		return err
	}

	// Export manifest
	return opts.ExportManifest(ctx, store, root)
}
