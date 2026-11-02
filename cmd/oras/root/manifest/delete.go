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
	"errors"
	"fmt"
	"os"

	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/registryutil"
)

type deleteOptions struct {
	option.Common
	option.Confirmation
	option.Descriptor
	option.Pretty
	option.Target
}

func deleteCmd() *cobra.Command {
	var opts deleteOptions
	cmd := &cobra.Command{
		Use:     "delete [flags] <name>{:<tag>|@<digest>}",
		Aliases: []string{"remove", "rm"},
		Short:   "Delete a manifest from remote registry",
		Long: `Delete a manifest from remote registry

Example - Delete a manifest tagged with 'v1' from repository 'localhost:5000/hello':
  oras manifest delete localhost:5000/hello:v1

Example - Delete a manifest without prompting confirmation:
  oras manifest delete --force localhost:5000/hello:v1

Example - Delete a manifest and print its descriptor:
  oras manifest delete --descriptor localhost:5000/hello:v1

Example - Delete a manifest by digest 'sha256:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9' from repository 'localhost:5000/hello':
  oras manifest delete localhost:5000/hello@sha:99e4703fbf30916f549cd6bfa9cdbab614b5392fbe64fdee971359a77073cdf9
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the manifest to delete"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RawReference = args[0]
			if opts.OutputDescriptor && !opts.Force {
				return errors.New("must apply --force to confirm the deletion if the descriptor is outputted")
			}
			return option.Parse(cmd, &opts)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return deleteManifest(cmd, &opts)
		},
	}

	opts.EnableDistributionSpecFlag()
	option.ApplyFlags(&opts, cmd.Flags())
	option.AddDeprecatedVerboseFlag(cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func deleteManifest(cmd *cobra.Command, opts *deleteOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	manifests, err := opts.NewManifestDeleter(opts.Common, logger)
	if err != nil {
		return err
	}

	return doDeleteManifest(ctx, cmd, opts, manifests, display.NewManifestDeleteHandler(opts.Printer, &opts.Target))
}

func doDeleteManifest(
	ctx context.Context,
	cmd *cobra.Command,
	opts *deleteOptions,
	manifests option.ResolvableDeleter,
	metadataHandler metadata.ManifestDeleteHandler,
) error {
	if err := opts.EnsureReferenceNotEmpty(cmd, true); err != nil {
		return err
	}

	// add both pull and delete scope hints for dst repository to save potential delete-scope token requests during deleting
	hints := []string{auth.ActionPull, auth.ActionDelete}
	if opts.ReferrersAPI != option.ReferrersStateSupported {
		// possibly needed when adding a new referrers index
		hints = append(hints, auth.ActionPush)
	}
	ctx = registryutil.WithScopeHint(ctx, manifests, hints...)
	desc, err := manifests.Resolve(ctx, opts.Reference)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			if opts.Force && !opts.OutputDescriptor {
				return metadataHandler.OnManifestMissing()
			}
			return fmt.Errorf("%s: the specified manifest does not exist", opts.RawReference)
		}
		return err
	}

	prompt := fmt.Sprintf("Are you sure you want to delete the manifest %q and all tags associated with it?", desc.Digest)
	confirmed, err := opts.AskForConfirmation(os.Stdin, prompt)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	if err = manifests.Delete(ctx, desc); err != nil {
		return fmt.Errorf("failed to delete %s: %w", opts.RawReference, err)
	}

	if opts.OutputDescriptor {
		descJSON, err := opts.Marshal(desc)
		if err != nil {
			return err
		}
		return opts.Output(os.Stdout, descJSON)
	}
	return metadataHandler.OnManifestDeleted()
}
