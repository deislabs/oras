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

import (
	"testing"
)

func TestMakeDirectoryAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		dirName  string
		wantPath string
		wantName string
	}{
		{
			name:     "root directory",
			path:     ".",
			dirName:  "mydir",
			wantPath: ".",
			wantName: "mydir",
		},
		{
			name:     "subdirectory",
			path:     "subdir1",
			dirName:  "subdir1",
			wantPath: "subdir1",
			wantName: "subdir1",
		},
		{
			name:     "nested directory",
			path:     "subdir1/nested",
			dirName:  "nested",
			wantPath: "subdir1/nested",
			wantName: "nested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeDirectoryAnnotations(tt.path, tt.dirName)

			if got[AnnotationDirectoryPath] != tt.wantPath {
				t.Errorf("MakeDirectoryAnnotations() path = %v, want %v", got[AnnotationDirectoryPath], tt.wantPath)
			}
			if got[AnnotationDirectoryName] != tt.wantName {
				t.Errorf("MakeDirectoryAnnotations() name = %v, want %v", got[AnnotationDirectoryName], tt.wantName)
			}
			if len(got) != 2 {
				t.Errorf("MakeDirectoryAnnotations() returned %d annotations, want 2", len(got))
			}
		})
	}
}

func TestMakeFileAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{
			name:     "root level file",
			path:     "file.txt",
			wantPath: "file.txt",
		},
		{
			name:     "nested file",
			path:     "subdir/file.txt",
			wantPath: "subdir/file.txt",
		},
		{
			name:     "deeply nested file",
			path:     "a/b/c/file.txt",
			wantPath: "a/b/c/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeFileAnnotations(tt.path)

			if got[AnnotationFilePath] != tt.wantPath {
				t.Errorf("MakeFileAnnotations() path = %v, want %v", got[AnnotationFilePath], tt.wantPath)
			}
			if len(got) != 1 {
				t.Errorf("MakeFileAnnotations() returned %d annotations, want 1", len(got))
			}
		})
	}
}

func TestMakeRootAnnotations(t *testing.T) {
	got := MakeRootAnnotations()

	if got[AnnotationRecursiveRoot] != "true" {
		t.Errorf("MakeRootAnnotations() root = %v, want %v", got[AnnotationRecursiveRoot], "true")
	}
	if got[AnnotationRecursiveVersion] != RecursiveFormatVersion {
		t.Errorf("MakeRootAnnotations() version = %v, want %v", got[AnnotationRecursiveVersion], RecursiveFormatVersion)
	}
	if len(got) != 2 {
		t.Errorf("MakeRootAnnotations() returned %d annotations, want 2", len(got))
	}
}

func TestAnnotationConstants(t *testing.T) {
	// Verify annotation key formats follow org.oras namespace
	annotations := []string{
		AnnotationDirectoryPath,
		AnnotationDirectoryName,
		AnnotationFilePath,
		AnnotationRecursiveRoot,
		AnnotationRecursiveVersion,
	}

	for _, ann := range annotations {
		if len(ann) < 10 || ann[:9] != "org.oras." {
			t.Errorf("Annotation %q should start with 'org.oras.'", ann)
		}
	}

	// Verify version format
	if RecursiveFormatVersion != "1.0" {
		t.Errorf("RecursiveFormatVersion = %v, want %v", RecursiveFormatVersion, "1.0")
	}
}
