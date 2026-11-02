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
	"fmt"
	"strings"

	"github.com/oras-project/oras-go/v3"
	"github.com/spf13/pflag"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
)

const (
	ImageSpecV1_1 = "v1.1"
	ImageSpecV1_0 = "v1.0"
)

const (
	DistributionSpecReferrersUnknown = "unknown"
	DistributionSpecReferrersTagV1_1 = "v1.1-referrers-tag"
	DistributionSpecReferrersAPIV1_1 = "v1.1-referrers-api"
)

// ImageSpec option struct which implements pflag.Value interface.
type ImageSpec struct {
	Flag        string
	PackVersion oras.PackManifestVersion
}

// Set validates and sets the flag value from a string argument.
func (is *ImageSpec) Set(value string) error {
	is.Flag = value
	switch value {
	case ImageSpecV1_1:
		is.PackVersion = oras.PackManifestVersion1_1
	case ImageSpecV1_0:
		is.PackVersion = oras.PackManifestVersion1_0
	default:
		return &oerrors.Error{
			Err:            fmt.Errorf("unknown image specification flag: %s", value),
			Recommendation: fmt.Sprintf("Available options: %s", is.Options()),
		}
	}
	return nil
}

// Type returns the string value of the inner flag.
func (is *ImageSpec) Type() string {
	return "string"
}

// Options returns the string of usable options for the flag.
func (is *ImageSpec) Options() string {
	return strings.Join([]string{
		ImageSpecV1_1,
		ImageSpecV1_0,
	}, ", ")
}

// String returns the string representation of the flag.
func (is *ImageSpec) String() string {
	// to avoid printing default value in usage doc
	return ""
}

// ApplyFlags applies flags to a command flag set.
func (is *ImageSpec) ApplyFlags(fs *pflag.FlagSet) {
	// default to v1.1, unless --config is used and --artifact-type is not used
	is.PackVersion = oras.PackManifestVersion1_1
	is.Flag = ImageSpecV1_1
	fs.Var(is, "image-spec", `[Preview] specify manifest type for building artifact. Options: v1.1, v1.0 (default v1.1, overridden to v1.0 if --config is used without --artifact-type)`)
}

// referrersState represents the state of Referrers API.
type referrersState = int32

const (
	// ReferrersStateUnknown represents an unknown state of Referrers API.
	ReferrersStateUnknown referrersState = iota
	// ReferrersStateSupported represents that the repository is known to
	// support Referrers API.
	ReferrersStateSupported
	// ReferrersStateUnsupported represents that the repository is known to
	// not support Referrers API.
	ReferrersStateUnsupported
)

// DistributionSpec option struct which implements pflag.Value interface.
type DistributionSpec struct {
	// ReferrersAPI indicates the preference of the implementation of the Referrers API.
	// Set to true for referrers API, false for referrers tag scheme, and nil for auto fallback.
	ReferrersAPI referrersState

	// specFlag should be provided in form of`<version>-<api>-<option>`
	flag string
}

// Set validates and sets the flag value from a string argument.
func (ds *DistributionSpec) Set(value string) error {
	ds.flag = value
	ds.ReferrersAPI = ReferrersStateUnknown
	switch ds.flag {
	case DistributionSpecReferrersTagV1_1:
		ds.ReferrersAPI = ReferrersStateUnsupported
	case DistributionSpecReferrersAPIV1_1:
		ds.ReferrersAPI = ReferrersStateSupported
	default:
		return &oerrors.Error{
			Err:            fmt.Errorf("unknown distribution specification flag: %s", value),
			Recommendation: fmt.Sprintf("Available options: %s", ds.Options()),
		}
	}
	return nil
}

// Type returns the string value of the inner flag.
func (ds *DistributionSpec) Type() string {
	return "string"
}

// Options returns the string of usable options for the flag.
func (ds *DistributionSpec) Options() string {
	return strings.Join([]string{
		DistributionSpecReferrersTagV1_1,
		DistributionSpecReferrersAPIV1_1,
	}, ", ")
}

// String returns the string representation of the flag.
func (ds *DistributionSpec) String() string {
	return ds.flag
}

// ApplyFlagsWithPrefix applies flags to a command flag set with a prefix string.
func (ds *DistributionSpec) ApplyFlagsWithPrefix(fs *pflag.FlagSet, prefix, description string) {
	ds.flag = DistributionSpecReferrersUnknown
	fs.Var(ds, prefix+"distribution-spec", fmt.Sprintf("[Preview] set OCI distribution spec version and API option for %starget. Options: %s", description, ds.Options()))
}
