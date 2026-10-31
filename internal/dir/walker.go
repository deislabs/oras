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

// Package dir provides utilities for directory traversal and tree building.
package dir

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Node represents a file or directory in the tree structure.
type Node struct {
	// Name is the base name of the file or directory.
	Name string
	// Path is the full path relative to the root.
	Path string
	// AbsPath is the absolute path on the filesystem.
	AbsPath string
	// IsDir indicates whether this node is a directory.
	IsDir bool
	// Size is the file size in bytes (0 for directories).
	Size int64
	// Children contains child nodes (only for directories).
	Children []*Node
	// Parent points to the parent node (nil for root).
	Parent *Node
}

// WalkOptions configures the directory walking behavior.
type WalkOptions struct {
	// FollowSymlinks follows symbolic links when walking. Followed links must
	// resolve within the walked root; targets outside the root are rejected.
	// When disabled, symbolic links are skipped entirely so that their targets
	// are never opened.
	FollowSymlinks bool
	// IncludeEmpty includes empty directories in the tree.
	IncludeEmpty bool
	// OnSkip, if set, is called with the slash-separated relative path of each
	// symbolic link skipped because FollowSymlinks is disabled.
	OnSkip func(path string)
}

// FileCount returns the total number of files in this node and its descendants.
func (n *Node) FileCount() int {
	if !n.IsDir {
		return 1
	}
	count := 0
	for _, child := range n.Children {
		count += child.FileCount()
	}
	return count
}

// DirCount returns the total number of directories in this node and its descendants.
func (n *Node) DirCount() int {
	if !n.IsDir {
		return 0
	}
	count := 1
	for _, child := range n.Children {
		count += child.DirCount()
	}
	return count
}

// HasFiles returns true if this directory contains any files (not just subdirectories).
func (n *Node) HasFiles() bool {
	if !n.IsDir {
		return false
	}
	for _, child := range n.Children {
		if !child.IsDir {
			return true
		}
	}
	return false
}

// HasDirs returns true if this directory contains any subdirectories.
func (n *Node) HasDirs() bool {
	if !n.IsDir {
		return false
	}
	for _, child := range n.Children {
		if child.IsDir {
			return true
		}
	}
	return false
}

// Files returns all file children (non-directories) of this node.
func (n *Node) Files() []*Node {
	var files []*Node
	for _, child := range n.Children {
		if !child.IsDir {
			files = append(files, child)
		}
	}
	return files
}

// Dirs returns all directory children of this node.
func (n *Node) Dirs() []*Node {
	var dirs []*Node
	for _, child := range n.Children {
		if child.IsDir {
			dirs = append(dirs, child)
		}
	}
	return dirs
}

// Walk traverses the directory tree starting from root and builds a Node tree.
// The root path should be a directory.
func Walk(root string, opts WalkOptions) (*Node, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		// Single file, return as a node
		return &Node{
			Name:    filepath.Base(absRoot),
			Path:    filepath.Base(absRoot),
			AbsPath: absRoot,
			IsDir:   false,
			Size:    info.Size(),
		}, nil
	}

	rootNode := &Node{
		Name:    filepath.Base(absRoot),
		Path:    ".",
		AbsPath: absRoot,
		IsDir:   true,
	}

	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, err
	}
	w := &walker{
		opts: opts,
		root: resolvedRoot,
		// resolved paths of directories on the current recursion chain,
		// used to detect symbolic link cycles
		onChain: map[string]struct{}{resolvedRoot: {}},
	}
	if err := w.walkDir(absRoot, resolvedRoot, ".", rootNode); err != nil {
		return nil, err
	}

	// Remove empty directories if not preserving them
	if !opts.IncludeEmpty {
		pruneEmpty(rootNode)
	}

	return rootNode, nil
}

// walker carries the state needed to enforce the symlink policy during a walk.
type walker struct {
	opts    WalkOptions
	root    string
	onChain map[string]struct{}
}

// walkDir recursively walks a directory and builds the tree.
// resolvedPath is absPath with all symbolic links resolved.
func (w *walker) walkDir(absPath, resolvedPath, relPath string, parent *Node) error {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return err
	}

	// Sort entries for consistent ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		childRelPath := filepath.Join(relPath, name)
		childAbsPath := filepath.Join(absPath, name)

		var info fs.FileInfo
		childResolved := filepath.Join(resolvedPath, name)
		if entry.Type()&fs.ModeSymlink != 0 {
			if !w.opts.FollowSymlinks {
				// Never hand a symlink onward as a regular file: loading it
				// would open the target, which may live outside the root.
				if w.opts.OnSkip != nil {
					w.opts.OnSkip(filepath.ToSlash(childRelPath))
				}
				continue
			}
			info, err = os.Stat(childAbsPath)
			if err != nil {
				return fmt.Errorf("failed to stat %q: %w", childAbsPath, err)
			}
			childResolved, err = filepath.EvalSymlinks(childAbsPath)
			if err != nil {
				return fmt.Errorf("failed to resolve symbolic link %q: %w", childAbsPath, err)
			}
			if !isWithin(w.root, childResolved) {
				return fmt.Errorf("symbolic link %q resolves to %q outside the directory being pushed", filepath.ToSlash(childRelPath), childResolved)
			}
		} else {
			info, err = entry.Info()
			if err != nil {
				return fmt.Errorf("failed to stat %q: %w", childAbsPath, err)
			}
		}

		switch {
		case info.IsDir():
			if _, ok := w.onChain[childResolved]; ok {
				return fmt.Errorf("symbolic link cycle detected: %q resolves to %q which is already being walked", filepath.ToSlash(childRelPath), childResolved)
			}
			child := &Node{
				Name:    name,
				Path:    filepath.ToSlash(childRelPath),
				AbsPath: childAbsPath,
				IsDir:   true,
				Parent:  parent,
			}
			w.onChain[childResolved] = struct{}{}
			err := w.walkDir(childAbsPath, childResolved, childRelPath, child)
			delete(w.onChain, childResolved)
			if err != nil {
				return err
			}
			parent.Children = append(parent.Children, child)
		case info.Mode().IsRegular():
			parent.Children = append(parent.Children, &Node{
				Name:    name,
				Path:    filepath.ToSlash(childRelPath),
				AbsPath: childAbsPath,
				Size:    info.Size(),
				Parent:  parent,
			})
		default:
			// FIFOs, sockets, and device files cannot be represented as blobs;
			// opening a FIFO without a writer would block indefinitely.
			return fmt.Errorf("unsupported file type (%s) at %q: only regular files, directories, and symbolic links are supported", info.Mode().Type(), filepath.ToSlash(childRelPath))
		}
	}

	return nil
}

// isWithin reports whether path is root or lies under root.
// Both paths must be absolute and symlink-resolved.
func isWithin(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// pruneEmpty removes empty directories from the tree.
func pruneEmpty(node *Node) bool {
	if !node.IsDir {
		return false
	}

	// Recursively prune children
	var nonEmpty []*Node
	for _, child := range node.Children {
		if child.IsDir {
			if !pruneEmpty(child) {
				nonEmpty = append(nonEmpty, child)
			}
		} else {
			nonEmpty = append(nonEmpty, child)
		}
	}
	node.Children = nonEmpty

	// Return true if this directory is now empty
	return len(node.Children) == 0
}

// FlattenFiles returns all file nodes in the tree as a flat slice.
func FlattenFiles(root *Node) []*Node {
	var files []*Node
	flattenFilesRecursive(root, &files)
	return files
}

func flattenFilesRecursive(node *Node, files *[]*Node) {
	if !node.IsDir {
		*files = append(*files, node)
		return
	}
	for _, child := range node.Children {
		flattenFilesRecursive(child, files)
	}
}
