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
	"context"
	"errors"
	"io/fs"
	"maps"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content/file"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/fileref"
)

func loadFiles(ctx context.Context, store *file.Store, annotations map[string]map[string]string, fileRefs []string, displayStatus status.PushHandler) ([]ocispec.Descriptor, error) {
	var files []ocispec.Descriptor
	for _, fileRef := range fileRefs {
		filename, mediaType, err := fileref.Parse(fileRef, "")
		if err != nil {
			return nil, err
		}

		// get shortest absolute path as unique name
		name := filepath.Clean(filename)
		if !filepath.IsAbs(name) {
			name = filepath.ToSlash(name)
		}

		if value, ok := annotations[filename]; ok {
			if nameFromAnnotations, ok := value[ocispec.AnnotationTitle]; ok {
				name = nameFromAnnotations
			}
		}

		err = displayStatus.OnFileLoading(name)
		if err != nil {
			return nil, err
		}
		file, err := addFile(ctx, store, name, mediaType, filename)
		if err != nil {
			return nil, err
		}
		if value, ok := annotations[filename]; ok {
			if file.Annotations == nil {
				file.Annotations = value
			} else {
				maps.Copy(file.Annotations, value)
			}
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		if err := displayStatus.OnEmptyArtifact(); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func addFile(ctx context.Context, store *file.Store, name string, mediaType string, filename string) (ocispec.Descriptor, error) {
	file, err := store.Add(ctx, name, mediaType, filename)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr
		}
		return ocispec.Descriptor{}, err
	}
	return file, nil
}
