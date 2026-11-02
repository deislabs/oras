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

package table

import (
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/output"
)

// discoverHandler handles json metadata output for discover events.
type discoverHandler struct {
	out          io.Writer
	rawReference string
	root         ocispec.Descriptor
	verbose      bool
	referrers    []ocispec.Descriptor
}

// NewDiscoverHandler creates a new handler for discover events.
func NewDiscoverHandler(out io.Writer, rawReference string, root ocispec.Descriptor, verbose bool) metadata.DiscoverHandler {
	return &discoverHandler{
		out:          out,
		rawReference: rawReference,
		root:         root,
		verbose:      verbose,
	}
}

// OnDiscovered implements metadata.DiscoverHandler.
func (h *discoverHandler) OnDiscovered(referrer, subject ocispec.Descriptor) error {
	if !content.Equal(subject, h.root) {
		return fmt.Errorf("unexpected subject descriptor: %v", subject)
	}
	h.referrers = append(h.referrers, referrer)
	return nil
}

// Render implements metadata.DiscoverHandler.
func (h *discoverHandler) Render() (err error) {
	if n := len(h.referrers); n != 1 {
		_, err = fmt.Fprintln(h.out, "Discovered", n, "artifacts referencing", h.rawReference)
	} else {
		_, err = fmt.Fprintln(h.out, "Discovered", n, "artifact referencing", h.rawReference)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(h.out, "Digest:", h.root.Digest)
	if err != nil {
		return err
	}
	if len(h.referrers) == 0 {
		return nil
	}
	_, err = fmt.Fprintln(h.out)
	if err != nil {
		return err
	}
	return h.printDiscoveredReferrersTable()
}

func (h *discoverHandler) printDiscoveredReferrersTable() (err error) {
	typeNameTitle := "Artifact Type"
	typeNameLength := len(typeNameTitle)
	for _, ref := range h.referrers {
		if length := len(ref.ArtifactType); length > typeNameLength {
			typeNameLength = length
		}
	}

	printKey := func(key string, value any) (err error) {
		_, err = fmt.Fprintln(h.out, key, strings.Repeat(" ", typeNameLength-len(key)+1), value)
		return err
	}

	err = printKey(typeNameTitle, "Digest")
	if err != nil {
		return err
	}
	for _, ref := range h.referrers {
		err = printKey(ref.ArtifactType, ref.Digest)
		if err != nil {
			return err
		}
		if h.verbose {
			if err := output.PrintPrettyJSON(h.out, ref); err != nil {
				return fmt.Errorf("error printing JSON: %w", err)
			}
		}
	}
	return nil
}
