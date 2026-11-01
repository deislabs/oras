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

package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/contentutil"
)

type showTagsOptions struct {
	option.Common
	option.Target
	option.Format

	last             string
	excludeDigestTag bool
}

func showTagsCmd() *cobra.Command {
	var opts showTagsOptions
	cmd := &cobra.Command{
		Use:   "tags [flags] <name>",
		Short: "Show tags of the target repository",
		Long: `Show tags of the target repository

Example - Show tags of the target repository:
  oras repo tags localhost:5000/hello

Example - Show tags in the target repository with digest-like tags hidden:
  oras repo tags --exclude-digest-tags localhost:5000/hello

Example - Show tags of the target repository that include values lexically after last:
  oras repo tags --last "last_tag" localhost:5000/hello

Example - Show tags of the target OCI image layout folder 'layout-dir':
  oras repo tags --oci-layout layout-dir

Example - Show tags of the target OCI layout archive 'layout.tar':
  oras repo tags --oci-layout layout.tar

Example - [Experimental] Show tags associated with a particular tagged resource:
  oras repo tags localhost:5000/hello:latest

Example - [Experimental] Show tags associated with a digest:
  oras repo tags localhost:5000/hello@sha256:c551125a624189cece9135981621f3f3144564ddabe14b523507bf74c2281d9b

Example - [Experimental] Show tags of the target repository in JSON format:
  oras repo tags localhost:5000/hello --format json

Example - [Experimental] Show tags of the target repository using the given Go template:
  oras repo tags localhost:5000/hello --format go-template --template "{{.tags}}"

Example - [Experimental] Show tags of a specific repository in OCI layout:
  oras repo tags --oci-layout-path layout-dir localhost:5000/hello
`,
		Args:    oerrors.CheckArgs(argument.Exactly(1), "the target repository to list tags from"),
		Aliases: []string{"show-tags"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RawReference = args[0]
			return option.Parse(cmd, &opts)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return showTags(cmd, &opts)
		},
	}
	cmd.Flags().StringVar(&opts.last, "last", "", "start after the tag specified by `last`")
	cmd.Flags().BoolVar(&opts.excludeDigestTag, "exclude-digest-tags", false, "[Preview] exclude all digest-like tags such as 'sha256-aaaa...'")
	option.AddDeprecatedVerboseFlag(cmd.Flags())
	opts.SetTypes(option.FormatTypeText, option.FormatTypeJSON, option.FormatTypeGoTemplate)
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.Target)
}

func showTags(cmd *cobra.Command, opts *showTagsOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	finder, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}

	// if a repository path is given, filter the tags under the repository
	var targetPrefix string
	if opts.Target.Type == option.TargetTypeOCILayout {
		ref, err := registry.ParseReference(opts.Reference)
		if err == nil && ref.Reference == "" {
			targetPrefix = fmt.Sprintf("%s/%s:", ref.Registry, ref.Repository)
		}
	}

	// if a tag is given, show the associated tags
	var targetDigest string
	if targetPrefix == "" && opts.Reference != "" {
		if contentutil.IsDigest(opts.Reference) {
			targetDigest = opts.Reference
		} else {
			desc, err := finder.Resolve(ctx, opts.Reference)
			if err != nil {
				return err
			}
			targetDigest = desc.Digest.String()
		}
		logger.Warnf("[Experimental] querying tags associated to %s, it may take a while...\n", targetDigest)
	}

	handler, err := display.NewRepoTagsHandler(opts.Printer, opts.Format)
	if err != nil {
		return err
	}
	return doShowTags(ctx, finder, opts, targetPrefix, targetDigest, handler)
}

func doShowTags(ctx context.Context, finder option.ReadOnlyGraphTagFinderTarget, opts *showTagsOptions, targetPrefix, targetDigest string, handler metadata.RepoTagsHandler) error {
	err := finder.Tags(ctx, opts.last, func(tags []string) error {
		for _, tag := range tags {
			// if --oci-layout-path is used with a repository path, filter the
			// tags under the repository.
			if targetPrefix != "" {
				scopedTag, ok := strings.CutPrefix(tag, targetPrefix)
				if !ok {
					continue
				}
				tag = scopedTag
			}

			// if --exclude-digest-tags is used, skip digest-like tags
			if opts.excludeDigestTag && isDigestTag(tag) {
				continue
			}

			// if a tag or digest is given, show the associated tags
			if targetDigest != "" {
				if tag == opts.Reference {
					if err := handler.OnTagListed(tag); err != nil {
						return err
					}
					continue
				}
				desc, err := finder.Resolve(ctx, tag)
				if err != nil {
					return err
				}
				if desc.Digest.String() != targetDigest {
					continue
				}
			}

			// show the tags in the repository
			if err := handler.OnTagListed(tag); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return handler.Render()
}

func isDigestTag(tag string) bool {
	dgst := strings.Replace(tag, "-", ":", 1)
	_, err := digest.Parse(dgst)
	return err == nil
}
