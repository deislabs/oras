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

	"github.com/spf13/pflag"
)

// DefaultMaxBlobsPerManifest is the default maximum number of blobs per manifest.
const DefaultMaxBlobsPerManifest = 1000

// Recursive option struct for recursive directory push.
type Recursive struct {
	// Recursive enables recursive directory push with hierarchical manifests.
	Recursive bool
	// MaxBlobsPerManifest limits the number of blobs per manifest for scalability.
	MaxBlobsPerManifest int
	// PreserveEmptyDirs preserves empty directories in the pushed structure.
	PreserveEmptyDirs bool
	// FollowSymlinks follows symbolic links when walking directories.
	FollowSymlinks bool
}

// ApplyFlags applies flags to a command flag set.
func (opts *Recursive) ApplyFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&opts.Recursive, "recursive", "r", false, "[Experimental] recursively push directory contents as hierarchical OCI artifacts")
	fs.IntVar(&opts.MaxBlobsPerManifest, "max-blobs-per-manifest", DefaultMaxBlobsPerManifest, "[Experimental] maximum number of blobs per manifest (used with --recursive)")
	fs.BoolVar(&opts.PreserveEmptyDirs, "preserve-empty-dirs", false, "[Experimental] preserve empty directories when pushing recursively")
	fs.BoolVar(&opts.FollowSymlinks, "follow-symlinks", false, "[Experimental] follow symbolic links when pushing recursively")
}

// Validate validates the recursive options.
func (opts *Recursive) Validate() error {
	if opts.MaxBlobsPerManifest < 1 {
		return errors.New("--max-blobs-per-manifest must be at least 1")
	}
	if opts.MaxBlobsPerManifest > 10000 {
		return fmt.Errorf("--max-blobs-per-manifest exceeds maximum allowed value of 10000")
	}
	if !opts.Recursive {
		if opts.PreserveEmptyDirs {
			return errors.New("--preserve-empty-dirs requires --recursive")
		}
		if opts.FollowSymlinks {
			return errors.New("--follow-symlinks requires --recursive")
		}
	}
	return nil
}
