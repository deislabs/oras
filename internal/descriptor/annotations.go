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

package descriptor

// Annotation keys for recursive directory push feature.
// These annotations are used to preserve directory structure information
// in OCI manifests and indexes.
const (
	// AnnotationDirectoryPath is the relative path of a directory within the
	// recursive push structure. This is set on Image Index descriptors that
	// represent subdirectories.
	AnnotationDirectoryPath = "org.oras.content.directory.path"

	// AnnotationDirectoryName is the base name of a directory.
	// This is set on Image Index descriptors that represent subdirectories.
	AnnotationDirectoryName = "org.oras.content.directory.name"

	// AnnotationFilePath is the relative path of a file within the recursive
	// push structure. This supplements ocispec.AnnotationTitle.
	AnnotationFilePath = "org.oras.content.file.path"

	// AnnotationRecursiveRoot marks the root manifest/index of a recursive push.
	// Value should be "true" if this is the root.
	AnnotationRecursiveRoot = "org.oras.content.recursive.root"

	// AnnotationRecursiveVersion indicates the version of the recursive push format.
	// This allows for future format changes while maintaining compatibility.
	AnnotationRecursiveVersion = "org.oras.content.recursive.version"

	// RecursiveFormatVersion is the current version of the recursive push format.
	RecursiveFormatVersion = "1.0"
)

// MakeDirectoryAnnotations creates annotations for a directory descriptor.
func MakeDirectoryAnnotations(path, name string) map[string]string {
	return map[string]string{
		AnnotationDirectoryPath: path,
		AnnotationDirectoryName: name,
	}
}

// MakeFileAnnotations creates annotations for a file descriptor.
func MakeFileAnnotations(path string) map[string]string {
	return map[string]string{
		AnnotationFilePath: path,
	}
}

// MakeRootAnnotations creates annotations for the root manifest/index.
func MakeRootAnnotations() map[string]string {
	return map[string]string{
		AnnotationRecursiveRoot:    "true",
		AnnotationRecursiveVersion: RecursiveFormatVersion,
	}
}
