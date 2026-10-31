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

package dir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalk(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create test structure:
	// tmpDir/
	//   file1.txt
	//   file2.txt
	//   subdir1/
	//     file3.txt
	//     nested/
	//       file4.txt
	//   subdir2/
	//     file5.txt
	//   empty/

	files := []string{
		"file1.txt",
		"file2.txt",
		"subdir1/file3.txt",
		"subdir1/nested/file4.txt",
		"subdir2/file5.txt",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Create empty directory
	if err := os.MkdirAll(filepath.Join(tmpDir, "empty"), 0755); err != nil {
		t.Fatalf("failed to create empty directory: %v", err)
	}

	t.Run("basic walk", func(t *testing.T) {
		node, err := Walk(tmpDir, WalkOptions{})
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if !node.IsDir {
			t.Error("root node should be a directory")
		}

		fileCount := node.FileCount()
		if fileCount != 5 {
			t.Errorf("FileCount() = %d, want 5", fileCount)
		}

		// Empty directory should be pruned by default
		dirCount := node.DirCount()
		if dirCount != 4 { // root + subdir1 + nested + subdir2
			t.Errorf("DirCount() = %d, want 4", dirCount)
		}
	})

	t.Run("walk with empty dirs", func(t *testing.T) {
		node, err := Walk(tmpDir, WalkOptions{IncludeEmpty: true})
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		dirCount := node.DirCount()
		if dirCount != 5 { // root + subdir1 + nested + subdir2 + empty
			t.Errorf("DirCount() = %d, want 5", dirCount)
		}
	})
}

func TestNode_Methods(t *testing.T) {
	// Create a test node structure manually
	root := &Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*Node{
			{Name: "file1.txt", Path: "file1.txt", IsDir: false, Size: 100},
			{Name: "file2.txt", Path: "file2.txt", IsDir: false, Size: 200},
			{
				Name:  "subdir",
				Path:  "subdir",
				IsDir: true,
				Children: []*Node{
					{Name: "file3.txt", Path: "subdir/file3.txt", IsDir: false, Size: 150},
				},
			},
		},
	}

	t.Run("FileCount", func(t *testing.T) {
		if got := root.FileCount(); got != 3 {
			t.Errorf("FileCount() = %d, want 3", got)
		}
	})

	t.Run("DirCount", func(t *testing.T) {
		if got := root.DirCount(); got != 2 {
			t.Errorf("DirCount() = %d, want 2", got)
		}
	})

	t.Run("HasFiles", func(t *testing.T) {
		if !root.HasFiles() {
			t.Error("HasFiles() = false, want true")
		}
	})

	t.Run("HasDirs", func(t *testing.T) {
		if !root.HasDirs() {
			t.Error("HasDirs() = false, want true")
		}
	})

	t.Run("Files", func(t *testing.T) {
		files := root.Files()
		if len(files) != 2 {
			t.Errorf("Files() length = %d, want 2", len(files))
		}
	})

	t.Run("Dirs", func(t *testing.T) {
		dirs := root.Dirs()
		if len(dirs) != 1 {
			t.Errorf("Dirs() length = %d, want 1", len(dirs))
		}
	})
}

func TestFlattenFiles(t *testing.T) {
	root := &Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*Node{
			{Name: "file1.txt", Path: "file1.txt", IsDir: false},
			{
				Name:  "subdir",
				Path:  "subdir",
				IsDir: true,
				Children: []*Node{
					{Name: "file2.txt", Path: "subdir/file2.txt", IsDir: false},
					{Name: "file3.txt", Path: "subdir/file3.txt", IsDir: false},
				},
			},
		},
	}

	files := FlattenFiles(root)
	if len(files) != 3 {
		t.Errorf("FlattenFiles() length = %d, want 3", len(files))
	}

	// Check paths
	expectedPaths := map[string]bool{
		"file1.txt":        true,
		"subdir/file2.txt": true,
		"subdir/file3.txt": true,
	}
	for _, f := range files {
		if !expectedPaths[f.Path] {
			t.Errorf("unexpected file path: %s", f.Path)
		}
	}
}

func TestWalk_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "single.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	node, err := Walk(filePath, WalkOptions{})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	if node.IsDir {
		t.Error("node should not be a directory")
	}
	if node.Name != "single.txt" {
		t.Errorf("node.Name = %q, want %q", node.Name, "single.txt")
	}
}

func TestWalk_BrokenSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Symlink(filepath.Join(tmpDir, "missing"), filepath.Join(tmpDir, "broken")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	if _, err := Walk(tmpDir, WalkOptions{FollowSymlinks: true}); err == nil {
		t.Fatal("expected error for broken symlink")
	}
}

func TestNode_FileNodeMethods(t *testing.T) {
	file := &Node{Name: "a.txt", Path: "a.txt"}
	if file.HasFiles() {
		t.Error("HasFiles() = true for a file node")
	}
	if file.HasDirs() {
		t.Error("HasDirs() = true for a file node")
	}
	if pruneEmpty(file) {
		t.Error("pruneEmpty() = true for a file node")
	}
}

func TestNode_HasFilesHasDirs_Negative(t *testing.T) {
	onlyDirs := &Node{IsDir: true, Children: []*Node{{IsDir: true}}}
	if onlyDirs.HasFiles() {
		t.Error("HasFiles() = true for a directory with only subdirectories")
	}
	onlyFiles := &Node{IsDir: true, Children: []*Node{{Name: "a"}}}
	if onlyFiles.HasDirs() {
		t.Error("HasDirs() = true for a directory with only files")
	}
}

func TestWalk_NotExist(t *testing.T) {
	if _, err := Walk(filepath.Join(t.TempDir(), "missing"), WalkOptions{}); err == nil {
		t.Fatal("expected error for missing root")
	}
}

// TestWalk_SymlinksSkippedWithoutFollow verifies that without FollowSymlinks,
// symbolic links are excluded from the tree entirely, not typed as regular
// files, which would let loading open a target outside the walked root.
func TestWalk_SymlinksSkippedWithoutFollow(t *testing.T) {
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("OUTSIDE DATA"), 0644); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "regular.txt"), []byte("in"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(tmpDir, "filelink")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(tmpDir, "dirlink")); err != nil {
		t.Fatal(err)
	}

	var skipped []string
	node, err := Walk(tmpDir, WalkOptions{OnSkip: func(path string) {
		skipped = append(skipped, path)
	}})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	files := FlattenFiles(node)
	if len(files) != 1 || files[0].Path != "regular.txt" {
		t.Errorf("expected only regular.txt in tree, got %v", files)
	}
	if len(skipped) != 2 {
		t.Errorf("OnSkip called with %v, want [dirlink filelink]", skipped)
	}
}

// TestWalk_FollowSymlinks_EscapeRejected verifies that followed symlinks must
// resolve within the walked root.
func TestWalk_FollowSymlinks_EscapeRejected(t *testing.T) {
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("OUTSIDE DATA"), 0644); err != nil {
		t.Fatal(err)
	}

	for name, target := range map[string]string{"file link": outsideFile, "dir link": outsideDir} {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.Symlink(target, filepath.Join(tmpDir, "link")); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}
			_, err := Walk(tmpDir, WalkOptions{FollowSymlinks: true})
			if err == nil || !strings.Contains(err.Error(), "outside the directory being pushed") {
				t.Fatalf("got error %v, want containment error", err)
			}
		})
	}
}

// TestWalk_FollowSymlinks_WithinRoot verifies that a symlink resolving inside
// the root is followed.
func TestWalk_FollowSymlinks_WithinRoot(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "real", "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tmpDir, "real"), filepath.Join(tmpDir, "alias")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	node, err := Walk(tmpDir, WalkOptions{FollowSymlinks: true})
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
	var paths []string
	for _, f := range FlattenFiles(node) {
		paths = append(paths, f.Path)
	}
	want := []string{"alias/a.txt", "real/a.txt"}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Errorf("file paths = %v, want %v", paths, want)
	}
}

// TestWalk_FollowSymlinks_CycleDetected verifies that a symlink resolving to a
// directory already on the traversal chain fails with an explicit cycle error
// instead of the OS "too many levels of symbolic links" error.
func TestWalk_FollowSymlinks_CycleDetected(t *testing.T) {
	t.Run("self link to root", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tmpDir, filepath.Join(tmpDir, "self")); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}
		_, err := Walk(tmpDir, WalkOptions{FollowSymlinks: true})
		if err == nil || !strings.Contains(err.Error(), "symbolic link cycle detected") {
			t.Fatalf("got error %v, want cycle error", err)
		}
	})

	t.Run("link to ancestor", func(t *testing.T) {
		tmpDir := t.TempDir()
		nested := filepath.Join(tmpDir, "a", "b")
		if err := os.MkdirAll(nested, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(tmpDir, "a"), filepath.Join(nested, "up")); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}
		_, err := Walk(tmpDir, WalkOptions{FollowSymlinks: true, IncludeEmpty: true})
		if err == nil || !strings.Contains(err.Error(), "symbolic link cycle detected") {
			t.Fatalf("got error %v, want cycle error", err)
		}
	})
}

func TestWalk_UnreadableSubdir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	tmpDir := t.TempDir()
	locked := filepath.Join(tmpDir, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })

	if _, err := Walk(tmpDir, WalkOptions{}); err == nil {
		t.Fatal("expected error for unreadable subdirectory")
	}
}
