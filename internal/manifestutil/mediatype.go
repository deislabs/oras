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

package manifestutil

import (
	"fmt"
	"regexp"

	"oras.land/oras-go/v2/errdef"
)

// mediaTypeRegexp checks the format of media types. It mirrors the validation
// oras.PackManifest performs, so that manifests built by this package are held
// to the same rules as ordinary packing.
// References:
//   - https://github.com/opencontainers/image-spec/blob/v1.1.1/schema/defs-descriptor.json#L7
//   - https://datatracker.ietf.org/doc/html/rfc6838#section-4.2
var mediaTypeRegexp = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,126}/[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,126}$`)

// ValidateMediaType returns an error wrapping errdef.ErrInvalidMediaType if
// mediaType is not a valid media type.
func ValidateMediaType(mediaType string) error {
	if !mediaTypeRegexp.MatchString(mediaType) {
		return fmt.Errorf("%s: %w", mediaType, errdef.ErrInvalidMediaType)
	}
	return nil
}
