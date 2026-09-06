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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/root/manifest/common"
	"oras.land/oras/internal/descriptor"
)

type fetchConfigOptions struct {
	option.Cache
	option.Common
	option.Descriptor
	option.Platform
	option.Pretty
	option.Target

	outputPath string
}

func fetchConfigCmd() *cobra.Command {
	var opts fetchConfigOptions
	cmd := &cobra.Command{
		Use:     "fetch-config [flags] <name>{:<tag>|@<digest>}",
		Aliases: []string{"get-config"},
		Short:   "Fetch the config of a manifest from a registry or an OCI image layout",
		Long: `Fetch the config of a manifest from a registry or an OCI image layout

Example - Fetch the config:
  oras manifest fetch-config localhost:5000/hello:v1

Example - Fetch the config of certain platform:
  oras manifest fetch-config --platform 'linux/arm/v5' localhost:5000/hello:v1

Example - Fetch and print the prettified config:
  oras manifest fetch-config --pretty localhost:5000/hello:v1

Example - Fetch the config and save it to a local file:
  oras manifest fetch-config --output config.json localhost:5000/hello:v1

Example - Fetch the descriptor of the config:
  oras manifest fetch-config --descriptor localhost:5000/hello:v1

Example - Fetch and print the prettified descriptor of the config:
  oras manifest fetch-config --descriptor --pretty localhost:5000/hello:v1
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the manifest config to fetch"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.outputPath == "-" && opts.OutputDescriptor {
				return errors.New("`--output -` cannot be used with `--descriptor` at the same time")
			}
			opts.RawReference = args[0]
			return option.Parse(cmd, &opts)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fetchConfig(cmd, &opts)
		},
	}

	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "file `path` to write the fetched config to, use - for stdout")
	option.AddDeprecatedVerboseFlag(cmd.Flags())
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func fetchConfig(cmd *cobra.Command, opts *fetchConfigOptions) (fetchErr error) {
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	repo, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}
	src, err := opts.CachedTarget(repo)
	if err != nil {
		return err
	}

	// fetch config descriptor
	configDesc, err := fetchConfigDesc(ctx, src, opts.Reference, opts.Platform.Platform)
	if err != nil {
		return err
	}

	if !opts.OutputDescriptor || opts.outputPath != "" {
		// fetch config content
		if opts.outputPath != "" && (opts.outputPath != "-" || !opts.Pretty.Pretty) {
			reader, err := src.Fetch(ctx, configDesc)
			if err != nil {
				return err
			}
			defer reader.Close()

			var output io.Writer
			var file *os.File
			if opts.outputPath == "-" {
				output = cmd.OutOrStdout()
			} else {
				file, err = os.CreateTemp(filepath.Dir(opts.outputPath), "oras-config-*")
				if err != nil {
					return err
				}
				defer func() {
					_ = file.Close()
					if fetchErr != nil {
						_ = os.Remove(file.Name())
					}
				}()
				output = file
			}

			vr := content.NewVerifyReader(reader, configDesc)
			if _, err = io.Copy(output, vr); err != nil {
				return err
			}
			if err := vr.Verify(); err != nil {
				return err
			}
			if opts.outputPath != "-" {
				if err := file.Chmod(0644); err != nil {
					return err
				}
				if err := file.Sync(); err != nil {
					return err
				}
				if err := file.Close(); err != nil {
					return err
				}
				if err := os.Rename(file.Name(), opts.outputPath); err != nil {
					return err
				}
			}
		} else {
			contentBytes, err := fetchConfigContent(ctx, src, configDesc)
			if err != nil {
				return err
			}

			// output config content
			return opts.Output(cmd.OutOrStdout(), contentBytes)
		}
	}

	if opts.OutputDescriptor {
		// output config's descriptor
		descBytes, err := json.Marshal(configDesc)
		if err != nil {
			return err
		}
		return opts.Output(cmd.OutOrStdout(), descBytes)
	}

	return nil
}

func fetchConfigContent(ctx context.Context, src oras.ReadOnlyTarget, configDesc ocispec.Descriptor) ([]byte, error) {
	if configDesc.Size > common.MaxConfigSize {
		return nil, &oerrors.Error{
			Err: fmt.Errorf(
				"config size %v exceeds the limit of %v bytes: %w",
				configDesc.Size,
				common.MaxConfigSize,
				errdef.ErrSizeExceedsLimit,
			),
			Recommendation: fmt.Sprintf(
				`To download a large config, use "oras blob fetch --output <file> <repository>@%s"`,
				configDesc.Digest,
			),
		}
	}

	return content.FetchAll(ctx, src, configDesc)
}

func fetchConfigDesc(ctx context.Context, src oras.ReadOnlyTarget, reference string, targetPlatform *ocispec.Platform) (ocispec.Descriptor, error) {
	// fetch manifest descriptor and content
	fetchOpts := oras.DefaultFetchBytesOptions
	fetchOpts.TargetPlatform = targetPlatform
	manifestDesc, manifestContent, err := oras.FetchBytes(ctx, src, reference, fetchOpts)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	if !descriptor.IsImageManifest(manifestDesc) {
		return ocispec.Descriptor{}, fmt.Errorf("%q is not an image manifest and does not have a config", manifestDesc.Digest)
	}

	// unmarshal manifest content to extract config descriptor
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		return ocispec.Descriptor{}, err
	}
	return manifest.Config, nil
}
