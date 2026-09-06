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
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/oras-project/oras-go/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
)

// BinaryTarget struct contains flags and arguments specifying two registries or
// image layouts.
// BinaryTarget implements errors.Handler interface.
type BinaryTarget struct {
	From        Target
	To          Target
	resolveFlag []string
}

// EnsureSourceTargetReferenceNotEmpty ensures that from target reference is not empty.
func (target *BinaryTarget) EnsureSourceTargetReferenceNotEmpty(cmd *cobra.Command) error {
	if target.From.Reference == "" {
		return oerrors.NewErrEmptyTagOrDigest(target.From.RawReference, cmd, true)
	}
	return nil
}

// EnableDistributionSpecFlag set distribution specification flag as applicable.
func (target *BinaryTarget) EnableDistributionSpecFlag() {
	target.From.EnableDistributionSpecFlag()
	target.To.EnableDistributionSpecFlag()
}

// ApplyFlags applies flags to a command flag set fs.
func (target *BinaryTarget) ApplyFlags(fs *pflag.FlagSet) {
	target.From.setFlagDetails("from", "source")
	target.From.ApplyFlags(fs)
	target.To.setFlagDetails("to", "destination")
	target.To.ApplyFlags(fs)
	fs.StringArrayVarP(&target.resolveFlag, "resolve", "", nil, "base DNS rules formatted in `host:port:address[:address_port]` for --from-resolve and --to-resolve")
}

// Parse parses user-provided flags and arguments into option struct.
func (target *BinaryTarget) Parse(cmd *cobra.Command) error {
	target.From.warned = make(map[string]*sync.Map)
	target.To.warned = target.From.warned
	// resolve are parsed in array order, latter will overwrite former
	target.From.resolveFlag = slices.Concat(target.resolveFlag, target.From.resolveFlag)
	target.To.resolveFlag = slices.Concat(target.resolveFlag, target.To.resolveFlag)
	return Parse(cmd, target)
}

// ModifyError handles error during cmd execution.
func (target *BinaryTarget) ModifyError(cmd *cobra.Command, err error) (bool, error) {
	var copyErr *oras.CopyError
	if !errors.As(err, &copyErr) {
		return target.modifyError(cmd, err)
	}

	err = copyErr.Err // extract the inner error
	var errTarget Target
	switch copyErr.Origin {
	case oras.CopyErrorOriginSource:
		errTarget = target.From
	case oras.CopyErrorOriginDestination:
		errTarget = target.To
	default:
		_, err := target.modifyError(cmd, err)
		return true, err
	}

	_, err = target.modifyError(cmd, err)
	// Example: Error from source registry for "localhost:5000/test:v1":
	// Example: Error from destination oci-layout for "oci-dir:v1":
	cmd.SetErrPrefix(fmt.Sprintf("Error from %s %s for %q:", copyErr.Origin, errTarget.Type, errTarget.RawReference))
	return true, err
}

func (target *BinaryTarget) modifyError(cmd *cobra.Command, err error) (bool, error) {
	if modified, modifiedErr := target.From.ModifyError(cmd, err); modified {
		return modified, modifiedErr
	}
	return target.To.ModifyError(cmd, err)
}
