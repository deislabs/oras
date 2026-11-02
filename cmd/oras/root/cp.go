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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/registry"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/oras-project/oras-go/v3/registry/remote/errcode"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	"oras.land/oras/cmd/oras/internal/command"
	"oras.land/oras/cmd/oras/internal/display"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/contentutil"
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/graph"
	"oras.land/oras/internal/listener"
	"oras.land/oras/internal/registryutil"
)

// dockerReferenceDigestAnnotation links a BuildKit attestation manifest in an
// index to the image manifest it describes. Other attestation association
// conventions are not inferred by platform filtering.
// Reference: https://github.com/moby/buildkit/blob/master/docs/attestations/attestation-storage.md
const dockerReferenceDigestAnnotation = "vnd.docker.reference.digest"

type copyOptions struct {
	option.Common
	option.Platforms
	option.BinaryTarget
	option.Terminal

	recursive   bool
	force       bool
	concurrency int
	extraRefs   []string
	// Deprecated: verbose is deprecated and will be removed in the future.
	verbose bool
}

func copyCmd() *cobra.Command {
	var opts copyOptions
	cmd := &cobra.Command{
		Use:     "cp [flags] <from>{:<tag>|@<digest>} <to>[:<tag>[,<tag>][...]]",
		Aliases: []string{"copy"},
		Short:   "Copy artifacts from one target to another",
		Long: `Copy artifacts from one target to another. When copying an image index, all of its manifests will be copied

Example - Copy an artifact between registries:
  oras cp localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1

Example - Download an artifact into an OCI image layout folder:
  oras cp --to-oci-layout localhost:5000/net-monitor:v1 ./downloaded:v1

Example - Upload an artifact from an OCI image layout folder:
  oras cp --from-oci-layout ./to-upload:v1 localhost:5000/net-monitor:v1

Example - Upload an artifact from an OCI layout tar archive:
  oras cp --from-oci-layout ./to-upload.tar:v1 localhost:5000/net-monitor:v1

Example - Copy an artifact and its referrers:
  oras cp -r localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1

Example - Copy an artifact and referrers using specific methods for the Referrers API:
  oras cp -r --from-distribution-spec v1.1-referrers-api --to-distribution-spec v1.1-referrers-tag \
    localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1

Example - Copy certain platform of an artifact:
  oras cp --platform linux/arm/v5 localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1

Example - Copy certain platforms of an artifact:
  oras cp --platform linux/amd64,linux/arm64,linux/arm/v7 localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1

Example - Copy an artifact with multiple tags:
  oras cp localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:tag1,tag2,tag3

Example - Copy an artifact with multiple tags with concurrency tuned:
  oras cp --concurrency 10 localhost:5000/net-monitor:v1 localhost:5000/net-monitor-copy:tag1,tag2,tag3

Example - Copy a multi-arch image to a destination that may be partially populated (e.g. a registry cache):
  oras cp --force localhost:5000/net-monitor:v1 localhost:6000/net-monitor-copy:v1
`,
		Args: oerrors.CheckArgs(argument.Exactly(2), "the source and destination for copying"),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.From.RawReference = args[0]
			refs := strings.Split(args[1], ",")
			opts.To.RawReference = refs[0]
			opts.extraRefs = refs[1:]
			err := option.Parse(cmd, &opts)
			if err != nil {
				return err
			}
			if err := option.ValidateExtraTags(opts.extraRefs); err != nil {
				return err
			}
			opts.DisableTTY(opts.Debug, false)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Printer.Verbose = opts.verbose
			return runCopy(cmd, &opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "[Preview] recursively copy the artifact and its referrer artifacts")
	cmd.Flags().BoolVarP(&opts.force, "force", "", false, "force a deep traversal of the destination graph before tagging the root; useful when the destination is partially populated (e.g. by a registry cache)")
	cmd.Flags().IntVarP(&opts.concurrency, "concurrency", "", 3, "concurrency level")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", true, "print status output for unnamed blobs")
	_ = cmd.Flags().MarkDeprecated("verbose", "and will be removed in a future release.")
	opts.EnableDistributionSpecFlag()
	option.ApplyFlags(&opts, cmd.Flags())
	return oerrors.Command(cmd, &opts.BinaryTarget)
}

func runCopy(cmd *cobra.Command, opts *copyOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)

	// Prepare source
	src, err := opts.From.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	if err := opts.EnsureSourceTargetReferenceNotEmpty(cmd); err != nil {
		return err
	}

	// Prepare destination
	dst, err := opts.To.NewTarget(opts.Common, logger)
	if err != nil {
		return err
	}
	ctx = registryutil.WithScopeHint(ctx, dst, auth.ActionPull, auth.ActionPush)
	statusHandler, metadataHandler := display.NewCopyHandler(opts.Printer, opts.TTY, dst)

	// Check if multiple platforms are specified
	if len(opts.Platforms.Platforms) > 1 {
		// Handle multiple platforms - copy manifests that match the specified platforms
		return copyMultiplePlatforms(ctx, logger, statusHandler, metadataHandler, src, dst, opts)
	}

	// Handle single platform or recursive mode
	return copyAndFinalize(ctx, statusHandler, metadataHandler, src, dst, opts, true)
}

func copyAndFinalize(ctx context.Context, statusHandler status.CopyHandler, metadataHandler metadata.CopyHandler, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, opts *copyOptions, rootInSource bool) error {
	desc, err := doCopy(ctx, statusHandler, src, dst, opts)
	if err != nil {
		return err
	}

	if from, err := digest.Parse(opts.From.Reference); rootInSource && err == nil && from != desc.Digest {
		// correct source digest
		opts.From.RawReference = fmt.Sprintf("%s@%s", opts.From.Path, desc.Digest.String())
	}

	if err := metadataHandler.OnCopied(&opts.BinaryTarget, desc); err != nil {
		return err
	}

	if len(opts.extraRefs) != 0 {
		tagNOpts := oras.DefaultTagNOptions
		tagNOpts.Concurrency = opts.concurrency
		tagListener := listener.NewTaggedListener(dst, metadataHandler.OnTagged)
		if _, err = oras.TagN(ctx, tagListener, opts.To.Reference, opts.extraRefs, tagNOpts); err != nil {
			return err
		}
	}

	return metadataHandler.Render()
}

// copyMultiplePlatforms handles copying when multiple platforms are specified
func copyMultiplePlatforms(ctx context.Context, logger logrus.FieldLogger, statusHandler status.CopyHandler, metadataHandler metadata.CopyHandler, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, opts *copyOptions) error {
	// Resolve the source reference to get the root descriptor
	resolveOpts := oras.DefaultResolveOptions
	// We don't set TargetPlatform here since we want to get the full index/list
	root, err := oras.Resolve(ctx, src, opts.From.Reference, resolveOpts)
	if err != nil {
		return &oras.CopyError{Origin: oras.CopyErrorOriginSource, Err: err}
	}

	// Check if the resolved descriptor is an index/manifest list
	isIndex := root.MediaType == ocispec.MediaTypeImageIndex || root.MediaType == docker.MediaTypeManifestList
	if !isIndex {
		sourceReference := opts.From.RawReference
		if sourceReference == "" {
			sourceReference = opts.From.Reference
		}
		return &oerrors.Error{
			Err:            fmt.Errorf("%q is not an image index or a manifest list", sourceReference),
			Recommendation: "selecting multiple platforms requires a multi-platform source; use a single --platform value to copy a specific platform of an image",
		}
	}

	indexContent, index, kept, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		return err
	}

	// If filtering retained every index entry, preserve the original root so
	// that its digest, referrers, and all entries are kept.
	if len(kept) == len(index.Manifests) {
		return copyAndFinalize(ctx, statusHandler, metadataHandler, src, dst, opts, true)
	}

	// Equivalent platform selectors can resolve to a single manifest. Keep the
	// established single-platform result instead of wrapping it in a one-entry
	// index merely because the request used more than one spelling.
	if len(kept) == 1 && index.Manifests[kept[0]].Platform != nil {
		singleOpts := *opts
		singleOpts.Platforms.Platforms = []*ocispec.Platform{index.Manifests[kept[0]].Platform}
		return copyAndFinalize(ctx, statusHandler, metadataHandler, src, dst, &singleOpts, true)
	}

	// Perform multiple copies
	return doMultipleCopy(ctx, logger, statusHandler, metadataHandler, src, dst, opts, root, indexContent, kept)
}

// filterManifestByPlatform returns the raw index bytes, the parsed index, and
// the positions of the entries that survive filtering. Positions rather than
// descriptors, because an index may list the same manifest under more than one
// platform and descriptors carry no way to tell those entries apart.
func filterManifestByPlatform(ctx context.Context, src oras.ReadOnlyGraphTarget, root ocispec.Descriptor, opts *copyOptions) ([]byte, ocispec.Index, []int, error) {
	// For indexes/lists, fetch the index content
	indexContent, err := content.FetchAll(ctx, src, root)
	if err != nil {
		return nil, ocispec.Index{}, nil, &oras.CopyError{
			Origin: oras.CopyErrorOriginSource,
			Err:    fmt.Errorf("failed to fetch index: %w", err),
		}
	}

	var index ocispec.Index
	if err = json.Unmarshal(indexContent, &index); err != nil {
		return nil, ocispec.Index{}, nil, &oras.CopyError{
			Origin: oras.CopyErrorOriginSource,
			Err:    fmt.Errorf("failed to parse index: %w", err),
		}
	}

	// Find the platform-bearing manifests selected by the request first. This
	// lets us retain associated attestations even when they use unknown/unknown
	// or omit the platform field.
	var availablePlatforms []string
	availablePlatformSet := make(map[string]bool)
	selected := make(map[int]bool, len(index.Manifests))
	for i, manifest := range index.Manifests {
		if manifest.Platform == nil {
			continue
		}
		platform := formatPlatform(manifest.Platform)
		if manifest.Annotations[dockerReferenceDigestAnnotation] == "" && !availablePlatformSet[platform] {
			availablePlatforms = append(availablePlatforms, platform)
			availablePlatformSet[platform] = true
		}
		if matchesAnyPlatform(manifest.Platform, opts.Platforms.Platforms) {
			selected[i] = true
		}
	}

	// Check if any platforms were unmatched
	var unmatchedPlatforms []string
	for _, platform := range opts.Platforms.Platforms {
		platformStr := formatPlatform(platform)
		contains := slices.ContainsFunc(index.Manifests, func(manifest ocispec.Descriptor) bool {
			return manifest.Annotations[dockerReferenceDigestAnnotation] == "" && platformMatches(manifest.Platform, platform)
		})
		if !contains {
			unmatchedPlatforms = append(unmatchedPlatforms, platformStr)
		}
	}
	// Return error with details about unmatched platforms
	if len(unmatchedPlatforms) > 0 {
		return nil, ocispec.Index{}, nil, &oerrors.Error{
			Err:            fmt.Errorf("unmatched platforms: [%s]", strings.Join(unmatchedPlatforms, ", ")),
			Recommendation: fmt.Sprintf("available platforms in index: [%s]", strings.Join(availablePlatforms, ", ")),
		}
	}

	var kept []int
	for i, manifest := range index.Manifests {
		if keepsManifest(i, manifest, index.Manifests, selected) {
			kept = append(kept, i)
		}
	}
	return indexContent, index, kept, nil
}

// keepsManifest reports whether the index entry at position i survives platform
// filtering, given the positions selected by platform matching. BuildKit
// attestations using dockerReferenceDigestAnnotation follow their subject
// manifest. A platform-less entry is retained only when it is not another
// reference to a manifest excluded by platform filtering. Other entries are
// kept only when their own position was selected.
func keepsManifest(i int, manifest ocispec.Descriptor, manifests []ocispec.Descriptor, selected map[int]bool) bool {
	if referenceDigest := manifest.Annotations[dockerReferenceDigestAnnotation]; referenceDigest != "" {
		for j := range manifests {
			if selected[j] && manifests[j].Digest.String() == referenceDigest {
				return true
			}
		}
		return false
	}
	if manifest.Platform == nil {
		for j, candidate := range manifests {
			if candidate.Platform != nil && !selected[j] && candidate.Digest == manifest.Digest {
				return false
			}
		}
		return true
	}
	return selected[i]
}

func formatPlatform(platform *ocispec.Platform) string {
	if platform == nil {
		return "<unknown>"
	}
	result := platform.OS + "/" + platform.Architecture
	if platform.Variant != "" {
		result += "/" + platform.Variant
	}
	if platform.OSVersion != "" {
		result += ":" + platform.OSVersion
	}
	return result
}

func doMultipleCopy(ctx context.Context, logger logrus.FieldLogger, statusHandler status.CopyHandler, metadataHandler metadata.CopyHandler, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, opts *copyOptions, root ocispec.Descriptor, originalIndexContent []byte, kept []int) error {
	indexContent, err := filterIndexContent(originalIndexContent, kept)
	if err != nil {
		return fmt.Errorf("failed to filter index: %w", err)
	}

	filteredRoot := content.NewDescriptorFromBytes(root.MediaType, indexContent)
	filteredSource := &filteredIndexSource{
		ReadOnlyGraphTarget: src,
		reference:           opts.From.Reference,
		root:                filteredRoot,
		content:             indexContent,
	}
	var filteredTarget oras.ReadOnlyGraphTarget = filteredSource
	if referrerLister, ok := src.(registry.ReferrerLister); ok {
		filteredTarget = &filteredIndexReferrerSource{
			filteredIndexSource: filteredSource,
			ReferrerLister:      referrerLister,
		}
	}
	if opts.recursive {
		referrers, err := registry.Referrers(ctx, src, root, "")
		if err != nil || len(referrers) > 0 {
			logger.Warn("referrers of the source index are not copied because selecting a subset of platforms produces a new index digest")
		}
	}
	return copyAndFinalize(ctx, statusHandler, metadataHandler, filteredTarget, dst, opts, false)
}

// filterIndexContent rewrites only the manifests array, preserving source key
// order and the raw JSON for all retained descriptors and other index fields.
func filterIndexContent(indexContent []byte, kept []int) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(indexContent))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, errors.New("index is not a JSON object")
	}
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	var foundManifests bool
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("index contains a non-string key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if key == "manifests" {
			if foundManifests {
				return nil, errors.New("index contains multiple manifests arrays")
			}
			foundManifests = true
			value, err = filterManifestArray(value, kept)
			if err != nil {
				return nil, err
			}
		}
		if buffer.Len() > 1 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	if !foundManifests {
		return nil, errors.New("index does not contain a manifests array")
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("index contains trailing JSON values")
		}
		return nil, err
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func filterManifestArray(manifestContent json.RawMessage, kept []int) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(manifestContent))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('[') {
		return nil, errors.New("index manifests is not an array")
	}
	keep := make(map[int]bool, len(kept))
	for _, i := range kept {
		keep[i] = true
	}
	filtered := make([]json.RawMessage, 0, len(kept))
	for i := 0; decoder.More(); i++ {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		var manifest ocispec.Descriptor
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return nil, err
		}
		if keep[i] {
			filtered = append(filtered, raw)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("index manifests contains trailing JSON values")
		}
		return nil, err
	}
	var buffer bytes.Buffer
	buffer.WriteByte('[')
	for i, raw := range filtered {
		if i > 0 {
			buffer.WriteByte(',')
		}
		buffer.Write(raw)
	}
	buffer.WriteByte(']')
	return buffer.Bytes(), nil
}

// filteredIndexSource presents a filtered index at the original source
// reference while delegating all other content and predecessor lookups to the
// source target. This lets the normal copy machinery handle manifests, blobs,
// referrers, force traversal, status reporting, and tagging consistently.
type filteredIndexSource struct {
	oras.ReadOnlyGraphTarget
	reference string
	root      ocispec.Descriptor
	content   []byte
}

func (s *filteredIndexSource) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	if reference == s.reference {
		return s.root, nil
	}
	return s.ReadOnlyGraphTarget.Resolve(ctx, reference)
}

func (s *filteredIndexSource) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	if content.Equal(target, s.root) {
		return io.NopCloser(bytes.NewReader(s.content)), nil
	}
	return s.ReadOnlyGraphTarget.Fetch(ctx, target)
}

func (s *filteredIndexSource) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	if content.Equal(target, s.root) {
		return true, nil
	}
	return s.ReadOnlyGraphTarget.Exists(ctx, target)
}

func (s *filteredIndexSource) Predecessors(ctx context.Context, target ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	if content.Equal(target, s.root) {
		// The filtered root has a new digest, so referrers of the original root
		// cannot be attached to it without rewriting their subjects.
		return nil, nil
	}
	return s.ReadOnlyGraphTarget.Predecessors(ctx, target)
}

func (s *filteredIndexSource) Unwrap() oras.ReadOnlyGraphTarget {
	return s.ReadOnlyGraphTarget
}

type filteredIndexReferrerSource struct {
	*filteredIndexSource
	registry.ReferrerLister
}

func (s *filteredIndexReferrerSource) Referrers(ctx context.Context, desc ocispec.Descriptor, artifactType string, fn func([]ocispec.Descriptor) error) error {
	if content.Equal(desc, s.root) {
		return nil
	}
	return s.ReferrerLister.Referrers(ctx, desc, artifactType, fn)
}

// matchesAnyPlatform checks if a manifest platform matches any of the specified platforms
func matchesAnyPlatform(manifestPlatform *ocispec.Platform, platforms []*ocispec.Platform) bool {
	return slices.ContainsFunc(platforms, func(platform *ocispec.Platform) bool {
		return platformMatches(manifestPlatform, platform)
	})
}

// platformMatches checks if two platforms match
func platformMatches(manifestPlatform, targetPlatform *ocispec.Platform) bool {
	if manifestPlatform == nil || targetPlatform == nil {
		return false
	}
	if manifestPlatform.OS != targetPlatform.OS || manifestPlatform.Architecture != targetPlatform.Architecture {
		return false
	}

	// An omitted variant or OS version is a wildcard. When supplied, the
	// requested value must match the descriptor exactly.
	if targetPlatform.Variant != "" && manifestPlatform.Variant != targetPlatform.Variant {
		return false
	}
	if targetPlatform.OSVersion != "" && manifestPlatform.OSVersion != targetPlatform.OSVersion {
		return false
	}

	return true
}

func doCopy(ctx context.Context, copyHandler status.CopyHandler, src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, opts *copyOptions) (desc ocispec.Descriptor, err error) {
	// Prepare copy options
	extendedCopyGraphOptions := oras.DefaultExtendedCopyGraphOptions
	extendedCopyGraphOptions.Concurrency = opts.concurrency
	extendedCopyGraphOptions.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		return registry.Referrers(ctx, src, desc, "")
	}

	if mountRepo, canMount := getMountPoint(src, dst, opts); canMount {
		extendedCopyGraphOptions.MountFrom = func(_ context.Context, _ ocispec.Descriptor) ([]string, error) {
			return []string{mountRepo}, nil
		}
	}

	if opts.force {
		// Defeat the sub-DAG skip in oras-go's copyGraph so that every
		// referenced manifest is visited and any missing successor (manifest
		// or blob) is copied to the destination before the root tag is
		// updated. This handles destinations that hold only a partial copy
		// of the artifact (e.g. registries with a pull-through cache that
		// has been populated from a single platform).
		dst = &contentutil.TraversingTarget{GraphTarget: dst}
	}
	dst, err = copyHandler.StartTracking(dst)
	if err != nil {
		return desc, err
	}
	defer func() {
		stopErr := copyHandler.StopTracking()
		if err == nil {
			err = stopErr
		}
	}()

	// Hook up handlers
	extendedCopyGraphOptions.OnCopySkipped = copyHandler.OnCopySkipped
	extendedCopyGraphOptions.PreCopy = copyHandler.PreCopy
	extendedCopyGraphOptions.PostCopy = copyHandler.PostCopy
	extendedCopyGraphOptions.OnMounted = copyHandler.OnMounted

	rOpts := oras.DefaultResolveOptions
	if len(opts.Platforms.Platforms) == 1 {
		rOpts.TargetPlatform = opts.Platforms.Platforms[0]
	}

	// Define the execution logic as a closure so we can retry it
	executeCopy := func(copyOpts oras.ExtendedCopyGraphOptions) (ocispec.Descriptor, error) {
		if opts.recursive {
			root, resolveErr := oras.Resolve(ctx, src, opts.From.Reference, rOpts)
			if resolveErr != nil {
				return ocispec.Descriptor{}, fmt.Errorf("failed to resolve %s: %w", opts.From.Reference, resolveErr)
			}
			return root, recursiveCopy(ctx, src, dst, opts.To.Reference, root, copyOpts)
		}

		if opts.To.Reference == "" {
			root, resolveErr := oras.Resolve(ctx, src, opts.From.Reference, rOpts)
			if resolveErr != nil {
				return ocispec.Descriptor{}, fmt.Errorf("failed to resolve %s: %w", opts.From.Reference, resolveErr)
			}
			return root, oras.CopyGraph(ctx, src, dst, root, copyOpts.CopyGraphOptions)
		}

		// Standard copy
		stdCopyOpts := oras.CopyOptions{
			CopyGraphOptions: copyOpts.CopyGraphOptions,
		}
		if len(opts.Platforms.Platforms) == 1 {
			stdCopyOpts.WithTargetPlatform(opts.Platforms.Platforms[0])
		}
		return oras.Copy(ctx, src, opts.From.Reference, dst, opts.To.Reference, stdCopyOpts)
	}

	desc, err = executeCopy(extendedCopyGraphOptions)

	// Mount failed due to permissions, retry without mounting
	if err != nil && extendedCopyGraphOptions.MountFrom != nil {
		var copyErr *oras.CopyError
		if errors.As(err, &copyErr) && copyErr.Op == "Mount" {
			var errResp *errcode.ErrorResponse
			if errors.As(copyErr.Err, &errResp) {
				if errResp.StatusCode == http.StatusUnauthorized ||
					errResp.StatusCode == http.StatusForbidden {
					// Disable mounting and retry
					extendedCopyGraphOptions.MountFrom = nil
					desc, err = executeCopy(extendedCopyGraphOptions)
				}
			}
		}
	}
	// leave the CopyError to oerrors.Modifier for prefix processing
	return desc, err
}

// recursiveCopy copies an artifact and its referrers from one target to another.
// If the artifact is a manifest list or index, referrers of its manifests are copied as well.
func recursiveCopy(ctx context.Context, src oras.ReadOnlyGraphTarget, dst oras.Target, dstRef string, root ocispec.Descriptor, opts oras.ExtendedCopyGraphOptions) error {
	opts, copyRoot, err := prepareCopyOption(ctx, src, dst, root, opts)
	if err != nil {
		return err
	}
	if err := oras.ExtendedCopyGraph(ctx, src, dst, copyRoot, opts); err != nil {
		return err
	}
	if dstRef != "" && dstRef != root.Digest.String() {
		if err := dst.Tag(ctx, root, dstRef); err != nil {
			// oras.Copy reports a failed root tagging as a destination-side
			// copy error. Do the same here so that the error is attributed to
			// the destination instead of the source.
			return &oras.CopyError{
				Op:     "Tag",
				Origin: oras.CopyErrorOriginDestination,
				Err:    err,
			}
		}
	}
	return nil
}

func prepareCopyOption(ctx context.Context, src oras.ReadOnlyGraphTarget, _ oras.Target, root ocispec.Descriptor, opts oras.ExtendedCopyGraphOptions) (oras.ExtendedCopyGraphOptions, ocispec.Descriptor, error) {
	if root.MediaType != ocispec.MediaTypeImageIndex && root.MediaType != docker.MediaTypeManifestList {
		return opts, root, nil
	}

	fetched, err := content.FetchAll(ctx, src, root)
	if err != nil {
		return oras.ExtendedCopyGraphOptions{}, ocispec.Descriptor{}, err
	}
	var index ocispec.Index
	if err = json.Unmarshal(fetched, &index); err != nil {
		return oras.ExtendedCopyGraphOptions{}, ocispec.Descriptor{}, err
	}

	if len(index.Manifests) == 0 {
		// no child manifests, thus no child referrers
		return opts, root, nil
	}

	referrers, err := graph.FindPredecessors(ctx, src, index.Manifests, opts)
	if err != nil {
		return oras.ExtendedCopyGraphOptions{}, ocispec.Descriptor{}, err
	}

	referrers = slices.DeleteFunc(referrers, func(desc ocispec.Descriptor) bool {
		return content.Equal(desc, root)
	})

	// A registry without Referrers API support may resolve the referrers tag
	// sha256-<hex> as the digest sha256:<hex> and answer with the index itself,
	// reporting the index's own children as its referrers. A child can never be
	// a genuine referrer of its own parent: it would have to carry digest(root)
	// in its subject, while digest(root) is taken over JSON that already
	// contains the child's digest, so the pair is unconstructible. Left in
	// place these phantom referrers make extended copy walk up into the
	// children, take those for the graph roots and never copy the index, so the
	// root tagging in recursiveCopy has nothing to tag.
	// Reference: https://github.com/oras-project/oras/issues/2148
	type findPredecessorsFunc = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error)
	dropOwnChildren := func(findPredecessors findPredecessorsFunc) findPredecessorsFunc {
		return func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			descs, err := findPredecessors(ctx, src, desc)
			if err != nil || !content.Equal(desc, root) {
				return descs, err
			}
			// Collect into a new slice: descs belongs to the wrapped
			// implementation and must not be modified in place.
			kept := make([]ocispec.Descriptor, 0, len(descs))
			for _, predecessor := range descs {
				if slices.ContainsFunc(index.Manifests, func(child ocispec.Descriptor) bool {
					return content.Equal(child, predecessor)
				}) {
					continue
				}
				kept = append(kept, predecessor)
			}
			return kept, nil
		}
	}

	if len(referrers) == 0 {
		// No child referrers, but root still needs the guard above. The CLI
		// always sets FindPredecessors in doCopy; this nil case is for library
		// callers. Default to src.Predecessors rather than registry.Referrers
		// so that predecessor discovery on this path is otherwise unchanged.
		findPredecessors := opts.FindPredecessors
		if findPredecessors == nil {
			findPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
				return src.Predecessors(ctx, desc)
			}
		}
		opts.FindPredecessors = dropOwnChildren(findPredecessors)
		return opts, root, nil
	}

	if opts.FindPredecessors == nil {
		opts.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return registry.Referrers(ctx, src, desc, "")
		}
	}
	opts.FindPredecessors = dropOwnChildren(opts.FindPredecessors)

	rootReferrers, err := opts.FindPredecessors(ctx, src, root)
	if err != nil {
		return oras.ExtendedCopyGraphOptions{}, ocispec.Descriptor{}, err
	}

	// If root has no referrers, we set copyRoot, which is the entry point of
	// extended copy, to the first manifest in the index. We also put the root
	// and the referrers of the manifests as the predecessors of copyRoot. This
	// is to ensure that all these nodes can be copied by calling extended copy.
	// Reference: https://github.com/oras-project/oras/issues/1728
	if len(rootReferrers) == 0 {
		copyRoot := index.Manifests[0]
		copyRootReferrers := append([]ocispec.Descriptor{root}, referrers...)
		findPredecessor := opts.FindPredecessors
		opts.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			switch {
			case content.Equal(desc, root):
				return nil, nil
			case content.Equal(desc, copyRoot):
				return copyRootReferrers, nil
			}
			return findPredecessor(ctx, src, desc)
		}
		return opts, copyRoot, nil
	}

	rootReferrers = append(rootReferrers, referrers...)
	findPredecessor := opts.FindPredecessors
	opts.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if content.Equal(desc, root) {
			return rootReferrers, nil
		}
		return findPredecessor(ctx, src, desc)
	}
	return opts, root, nil
}

// getMountPoint checks if mounting can be performed between two targets and returns
// the repository name to be mounted from if applicable. Mount can be performed if the two
// targets are both remote repositories, are in the same registry and have identical credentials.
func getMountPoint(src oras.ReadOnlyGraphTarget, dst oras.GraphTarget, opts *copyOptions) (string, bool) {
	if unwrapper, ok := src.(interface {
		Unwrap() oras.ReadOnlyGraphTarget
	}); ok {
		src = unwrapper.Unwrap()
	}
	srcRepo, srcIsRemote := src.(*remote.Repository)
	dstRepo, dstIsRemote := dst.(*remote.Repository)
	if !srcIsRemote || !dstIsRemote {
		return "", false
	}
	if srcRepo.Registry.Reference.Registry != dstRepo.Registry.Reference.Registry {
		return "", false
	}
	srcCred := opts.From.Credential()
	dstCred := opts.To.Credential()
	if srcCred != dstCred {
		return "", false
	}
	return srcRepo.RepositoryName, true
}
