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

package repository

import (
	"fmt"
	"strings"

	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"
)

// RejectScheme returns an error if rawReference carries a URL scheme such as
// http://, https:// or oci://. oras-go v3 silently strips these schemes, which
// would turn http://host/repo into an HTTPS request; plain HTTP is selected
// with --plain-http instead.
func RejectScheme(rawReference string) error {
	if strings.Contains(rawReference, "://") {
		return fmt.Errorf("%w: %q: URL schemes are not supported, use --plain-http for plain HTTP registries", errdef.ErrInvalidReference, rawReference)
	}
	return nil
}

// ParseReference parses rawReference into a reference, rejecting URL schemes.
func ParseReference(rawReference string) (properties.Reference, error) {
	if err := RejectScheme(rawReference); err != nil {
		return properties.Reference{}, err
	}
	return properties.NewReference(rawReference)
}

// ParseRemoteRepository extracts hostname and namespace from rawReference.
func ParseRemoteRepository(rawReference string) (hostname, namespace string, err error) {
	rawReference = strings.TrimSuffix(rawReference, "/")
	if strings.Contains(rawReference, "/") {
		var ref properties.Reference
		ref, err = ParseReference(rawReference)
		if err != nil {
			return
		}
		if ref.GetReference() != "" {
			err = fmt.Errorf("tags or digests should not be provided")
			return
		}
		hostname = ref.Registry
		namespace = ref.Repository + "/"
	} else {
		hostname = rawReference
	}
	return
}
