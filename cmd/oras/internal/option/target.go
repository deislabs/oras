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

package option

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/fileref"
	"oras.land/oras/internal/contentutil"
)

const (
	TargetTypeRemote    = "registry"
	TargetTypeOCILayout = "oci-layout"
)

// Target struct contains flags and arguments specifying one registry or image
// layout.
// Target implements oerrors.Handler interface.
type Target struct {
	Remote
	RawReference string
	Type         string
	Reference    string //contains tag or digest
	// Path contains
	//  - path to the OCI image layout target, or
	//  - registry and repository for the remote target
	Path string

	IsOCILayout bool

	prefix      string
	description string
}

// GetDisplayReference returns full printable reference.
func (target *Target) GetDisplayReference() string {
	return fmt.Sprintf("[%s] %s", target.Type, target.RawReference)
}

// setFlagDetails set directional flag prefix and description details
func (target *Target) setFlagDetails(prefix, description string) {
	if prefix != "" {
		target.prefix = prefix + "-"
		target.description = description + " "
	}
}

// ApplyFlags applies flags to a command flag set
// The complete form of the `target` flag is designed to be
//
//	--target type=<type>[[,<key>=<value>][...]]
//
// For better UX, the boolean flag `--oci-layout` is introduced as an alias of
// `--target type=oci-layout`.
// Since there is only one target type besides the default `registry` type,
// the full form is not implemented until a new type comes in.
func (target *Target) ApplyFlags(fs *pflag.FlagSet) {
	target.ApplyFlagsWithPrefix(fs, target.prefix, target.description)
	if target.prefix == "" {
		target.applyStdinFlags(fs)
	}
	fs.BoolVarP(&target.IsOCILayout, target.prefix+"oci-layout", "", false, "set "+target.description+"target as an OCI image layout")
	fs.StringVar(&target.Path, target.prefix+"oci-layout-path", "", "[Experimental] set the path for the "+target.description+"OCI image layout target")
}

const errHeaderFlagsOnOCILayout = "custom header flags cannot be used on an OCI image layout target"

// Parse gets target options from user input.
func (target *Target) Parse(cmd *cobra.Command) error {
	if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), target.flagPrefix+"oci-layout-path", target.flagPrefix+"oci-layout"); err != nil {
		return err
	}

	switch {
	case target.IsOCILayout:
		target.Type = TargetTypeOCILayout
		if len(target.headerFlags) != 0 {
			return errors.New(errHeaderFlagsOnOCILayout)
		}
		return target.parseOCILayoutReference()
	case target.Path != "":
		target.Type = TargetTypeOCILayout
		if len(target.headerFlags) != 0 {
			return errors.New(errHeaderFlagsOnOCILayout)
		}
		target.Reference = target.RawReference
		return nil
	default:
		target.Type = TargetTypeRemote
		ref, err := registry.ParseReference(target.RawReference)
		if err != nil {
			return &oerrors.Error{
				OperationType:  oerrors.OperationTypeParseArtifactReference,
				Err:            fmt.Errorf("%q: %w", target.RawReference, err),
				Recommendation: "Please make sure the provided reference is in the form of <registry>/<repo>[:tag|@digest]",
			}
		}
		target.Reference = ref.Reference
		ref.Reference = ""
		target.Path = ref.String()
		return target.Remote.Parse(cmd)
	}
}

// parseOCILayoutReference parses the raw in format of <path>[:<tag>|@<digest>]
func (target *Target) parseOCILayoutReference() error {
	raw := target.RawReference
	var path string
	var ref string
	if idx := strings.LastIndex(raw, "@"); idx != -1 {
		// `digest` found
		path = raw[:idx]
		ref = raw[idx+1:]
		if path == "" {
			return errors.Join(fmt.Errorf("found empty file path in %q", raw), errdef.ErrInvalidReference)
		}
	} else {
		// find `tag`
		var err error
		path, ref, err = fileref.Parse(raw, "")
		if err != nil {
			return errors.Join(err, errdef.ErrInvalidReference)
		}
	}
	target.Path = path
	target.Reference = ref
	return nil
}

func (target *Target) newOCIStore() (*oci.Store, error) {
	return oci.New(target.Path)
}

func (target *Target) newRepository(common Common, logger logrus.FieldLogger) (*remote.Repository, error) {
	return target.NewRepository(target.RawReference, common, logger)
}

// NewTarget generates a new target based on target.
func (target *Target) NewTarget(common Common, logger logrus.FieldLogger) (oras.GraphTarget, error) {
	switch target.Type {
	case TargetTypeOCILayout:
		return target.newOCIStore()
	case TargetTypeRemote:
		return target.newRepository(common, logger)
	}
	return nil, fmt.Errorf("unknown target type: %q", target.Type)
}

// NewBlobDeleter generates a new blob deleter based on target.
func (target *Target) NewBlobDeleter(common Common, logger logrus.FieldLogger) (ResolvableDeleter, error) {
	switch target.Type {
	case TargetTypeOCILayout:
		return target.newOCIStore()
	case TargetTypeRemote:
		repo, err := target.newRepository(common, logger)
		if err != nil {
			return nil, err
		}
		return repo.Blobs(), nil
	}
	return nil, fmt.Errorf("unknown target type: %q", target.Type)
}

// NewManifestDeleter generates a new blob deleter based on target.
func (target *Target) NewManifestDeleter(common Common, logger logrus.FieldLogger) (ResolvableDeleter, error) {
	switch target.Type {
	case TargetTypeOCILayout:
		return target.newOCIStore()
	case TargetTypeRemote:
		repo, err := target.newRepository(common, logger)
		if err != nil {
			return nil, err
		}
		return repo.Manifests(), nil
	}
	return nil, fmt.Errorf("unknown target type: %q", target.Type)
}

// NewReadonlyTarget generates a new read only target based on target.
func (target *Target) NewReadonlyTarget(ctx context.Context, common Common, logger logrus.FieldLogger) (ReadOnlyGraphTagFinderTarget, error) {
	switch target.Type {
	case TargetTypeOCILayout:
		info, err := os.Stat(target.Path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("invalid argument %q: failed to find path %q: %w", target.RawReference, target.Path, err)
			}
			return nil, err
		}
		if info.IsDir() {
			return oci.NewFromFS(ctx, os.DirFS(target.Path))
		}
		store, err := oci.NewFromTar(ctx, target.Path)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("%q does not look like a tar archive: %w", target.Path, err)
			}
			return nil, err
		}
		return store, nil
	case TargetTypeRemote:
		return target.NewRepository(target.RawReference, common, logger)
	}
	return nil, fmt.Errorf("unknown target type: %q", target.Type)
}

// EnsureReferenceNotEmpty returns an error when the reference is empty
// or when tags are not allowed and the reference is not a digest.
func (target *Target) EnsureReferenceNotEmpty(cmd *cobra.Command, allowTag bool) error {
	if target.Reference == "" {
		return oerrors.NewErrEmptyTagOrDigest(target.RawReference, cmd, allowTag)
	}
	if !allowTag && !contentutil.IsDigest(target.Reference) {
		return oerrors.NewErrEmptyTagOrDigest(target.RawReference, cmd, false)
	}
	return nil
}

// ModifyError handles error during cmd execution.
func (target *Target) ModifyError(cmd *cobra.Command, err error) (bool, error) {
	if target.IsOCILayout {
		// short circuit for non-remote targets
		return false, err
	}

	// handle errors for remote targets
	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return true, target.DecorateCredentialError(err)
	}

	if errors.Is(err, errdef.ErrNotFound) {
		// special handling for not found error returned by registry target
		cmd.SetErrPrefix(oerrors.RegistryErrorPrefix)
		return true, err
	}

	var errResp *errcode.ErrorResponse
	if !errors.As(err, &errResp) {
		// short circuit if the error is not an ErrorResponse
		return false, err
	}

	ref := registry.Reference{Registry: target.RawReference}
	if errResp.URL.Host != ref.Host() {
		// raw reference is not registry host
		var parseErr error
		ref, parseErr = registry.ParseReference(target.RawReference)
		if parseErr != nil {
			// this should not happen
			return false, err
		}
		if errResp.URL.Host != ref.Host() {
			// not handle if the error is not from the target
			return false, err
		}
	}

	cmd.SetErrPrefix(oerrors.RegistryErrorPrefix)
	ret := &oerrors.Error{
		Err: oerrors.ReportErrResp(errResp),
	}

	if ref.Registry == "docker.io" && errResp.StatusCode == http.StatusUnauthorized {
		if ref.Repository != "" && !strings.Contains(ref.Repository, "/") {
			// docker.io/xxx -> docker.io/library/xxx
			ref.Repository = "library/" + ref.Repository
			ret.Recommendation = fmt.Sprintf("Namespace seems missing. Do you mean `%s %s`?", cmd.CommandPath(), ref)
		}
	}
	return true, ret
}
