// Package toolsafety contains shared filesystem and diagnostic safety rules for generator tools.
package toolsafety

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	devfs "github.com/devctllabs/go-libs/filesystem"
)

// File is a contained regular file read from a tool workspace.
type File struct {
	Content []byte
	Mode    fs.FileMode
}

// ReadRegularFile reads name below root without accepting symlinks or non-regular files.
func ReadRegularFile(root, name string) (File, error) {
	contained, err := containedName(root, name)
	if err != nil {
		return File{}, err
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return File{}, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()
	info, err := disk.Lstat(contained)
	if err != nil {
		return File{}, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("%w: path is not a regular non-symlink file: %s", fs.ErrInvalid, name)
	}
	content, err := fs.ReadFile(disk, contained)
	if err != nil {
		return File{}, fmt.Errorf("filesystem.ReadFile: %w", err)
	}
	return File{Content: content, Mode: info.Mode().Perm()}, nil
}

// RequireDirectory requires name to be a real directory contained below root.
func RequireDirectory(root, name string) error {
	contained, err := containedName(root, name)
	if err != nil {
		return err
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()
	info, err := disk.Lstat(contained)
	if err != nil {
		return fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: path is not a real directory: %s", fs.ErrInvalid, name)
	}
	return nil
}

// ReadRegularTree reads a contained tree of regular files using slash-separated relative paths.
func ReadRegularTree(root, directory string) (artifact.Tree, error) {
	contained, err := containedName(root, directory)
	if err != nil {
		return artifact.Tree{}, err
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return artifact.Tree{}, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()
	info, err := disk.Lstat(contained)
	if err != nil {
		return artifact.Tree{}, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return artifact.Tree{}, fmt.Errorf("%w: path is not a real directory: %s", fs.ErrInvalid, directory)
	}

	var tree artifact.Tree
	err = fs.WalkDir(disk, contained, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == contained {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: generated output is a symlink: %s", fs.ErrInvalid, name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("entry.Info: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: generated output is not a regular file: %s", fs.ErrInvalid, name)
		}
		content, err := fs.ReadFile(disk, name)
		if err != nil {
			return fmt.Errorf("filesystem.ReadFile: %w", err)
		}
		relative := strings.TrimPrefix(name, contained+"/")
		tree.Files = append(tree.Files, artifact.File{
			Path: relative, Content: content, Mode: uint32(info.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return artifact.Tree{}, fmt.Errorf("filesystem.WalkDir: %w", err)
	}
	sort.Slice(tree.Files, func(i, j int) bool { return tree.Files[i].Path < tree.Files[j].Path })
	return tree, nil
}

// BoundedOutput trims tool output and limits it to 4096 bytes.
func BoundedOutput(output []byte) string {
	const limit = 4 << 10
	output = bytes.TrimSpace(output)
	if len(output) > limit {
		output = output[:limit]
	}
	return string(output)
}

func containedName(root, name string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("filepath.Abs root: %w", err)
	}
	absoluteName := name
	if !filepath.IsAbs(absoluteName) {
		absoluteName = filepath.Join(absoluteRoot, filepath.FromSlash(name))
	}
	absoluteName, err = filepath.Abs(absoluteName)
	if err != nil {
		return "", fmt.Errorf("filepath.Abs name: %w", err)
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteName)
	if err != nil {
		return "", fmt.Errorf("filepath.Rel: %w", err)
	}
	relative = filepath.ToSlash(relative)
	if !fs.ValidPath(relative) {
		return "", fmt.Errorf("%w: path %q is outside root", fs.ErrInvalid, name)
	}
	return relative, nil
}
