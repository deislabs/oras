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
	"strings"

	"github.com/oras-project/oras-go/v3/registry/remote/properties"
)

func validateTag(tag string) error {
	ref := properties.Reference{Tag: tag}
	return ref.ValidateTag()
}

// ValidateOCILayoutTag validates an OCI layout tag.
func ValidateOCILayoutTag(tag string) error {
	return validateTag(tag)
}

// ValidateOCILayoutPathReference validates an OCI layout path reference.
func ValidateOCILayoutPathReference(raw string) error {
	if raw == "" {
		return errors.New("empty OCI layout reference")
	}
	ref := properties.Reference{}

	if atIndex := strings.LastIndex(raw, "@"); atIndex != -1 {
		ref.Repository = raw[:atIndex]
		ref.Digest = raw[atIndex+1:]
		if err := ref.ValidateRepository(); err != nil {
			return err
		}
		return ref.ValidateDigest()
	}

	colonIndex := strings.LastIndex(raw, ":")
	slashIndex := strings.LastIndex(raw, "/")

	if colonIndex > slashIndex {
		ref.Repository = raw[:colonIndex]
		if ref.Repository == "" {
			return errors.New("empty repository in OCI layout reference")
		}
		ref.Tag = raw[colonIndex+1:]
	} else if slashIndex == -1 {
		ref.Tag = raw
	} else {
		ref.Repository = raw
	}

	if ref.Repository != "" {
		if err := ref.ValidateRepository(); err != nil {
			return err
		}
	}

	if ref.Tag != "" {
		return validateTag(ref.Tag)
	}
	return nil
}

// ValidateExtraTags validates additional destination tags.
func ValidateExtraTags(tags []string) error {
	for _, tag := range tags {
		if tag == "" {
			return errors.New("empty extra tag")
		}

		if err := validateTag(tag); err != nil {
			return fmt.Errorf("invalid extra tag: %w", err)
		}
	}
	return nil
}
