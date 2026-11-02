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

	"github.com/oras-project/oras-go/v3/registry/remote/properties"
)

// ValidateExtraTags validates additional destination tags.
func ValidateExtraTags(tags []string) error {
	for _, tag := range tags {
		if tag == "" {
			return errors.New("empty extra tag")
		}

		ref := properties.Reference{Tag: tag}
		if err := ref.ValidateTag(); err != nil {
			return fmt.Errorf("invalid extra tag: %w", err)
		}
	}
	return nil
}
