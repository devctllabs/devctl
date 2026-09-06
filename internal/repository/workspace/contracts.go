package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	devfs "github.com/devctllabs/go-libs/filesystem"
	"gopkg.in/yaml.v3"
)

// ReadContract reads exact bytes from a resolved regular non-symlink contract path.
func (r *FilesystemRepo) ReadContract(ctx context.Context, contractPath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	info, err := os.Lstat(contractPath)
	if err != nil {
		return nil, fmt.Errorf("os.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("contract is not a regular non-symlink file: %s", contractPath)
	}
	data, err := os.ReadFile(contractPath)
	if err != nil {
		return nil, fmt.Errorf("os.ReadFile: %w", err)
	}
	return data, nil
}

// ListProtoFiles returns sorted project-relative Proto files below relativeRoot.
func (r *FilesystemRepo) ListProtoFiles(ctx context.Context, root, relativeRoot string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()
	var files []string
	err = fs.WalkDir(disk, relativeRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink in Proto tree: %s", name)
		}
		if !entry.IsDir() && path.Ext(name) == ".proto" {
			files = append(files, path.Clean(name))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("filesystem.WalkDir: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

// ResolveContract selects one regular non-symlink entrypoint within location.Root.
// When Entrypoint is empty, resolution succeeds only if exactly one OpenAPI 3.x document is found.
func (r *FilesystemRepo) ResolveContract(ctx context.Context, location contract.Location) (string, error) {
	disk, err := devfs.Open(location.Root)
	if err != nil {
		return "", fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()

	relative := location.RelativePath
	if location.Local {
		if location.Entrypoint != location.RelativePath {
			info, statErr := disk.Lstat(relative)
			if statErr != nil {
				return "", fmt.Errorf("filesystem.Lstat: %w", statErr)
			}
			if info.IsDir() {
				relative = path.Join(relative, location.Entrypoint)
			}
		}
	} else if location.Entrypoint != "" {
		relative = path.Join(relative, location.Entrypoint)
	}
	if location.Entrypoint == "" {
		relative, err = findOpenAPI(ctx, disk, relative)
		if err != nil {
			return "", fmt.Errorf("findOpenAPI: %w", err)
		}
	}
	info, err := disk.Lstat(relative)
	if err != nil {
		return "", fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("contract is not a regular non-symlink file: %s", relative)
	}
	return filepath.Join(location.Root, filepath.FromSlash(relative)), nil
}

func findOpenAPI(ctx context.Context, disk *devfs.OS, root string) (string, error) {
	var candidates []string
	err := fs.WalkDir(disk, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink in materialized target: %s", name)
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(disk, name)
		if err != nil {
			return fmt.Errorf("filesystem.ReadFile: %w", err)
		}
		var document yaml.Node
		if yaml.Unmarshal(data, &document) == nil && strings.HasPrefix(mappingScalar(documentRoot(&document), "openapi"), "3.") {
			candidates = append(candidates, name)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("filesystem.WalkDir: %w", err)
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("expected exactly one OpenAPI entrypoint in %s, found %d", root, len(candidates))
	}
	return candidates[0], nil
}

func documentRoot(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

func mappingScalar(node *yaml.Node, key string) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1].Value
		}
	}
	return ""
}
