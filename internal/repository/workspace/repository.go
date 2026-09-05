package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	"github.com/devctllabs/devctl/internal/domain/contract"
	devfs "github.com/devctllabs/go-libs/filesystem"
)

// FilesystemRepo implements contained project workspace mechanics over the operating-system filesystem.
type FilesystemRepo struct {
	publication sync.Mutex
}

func NewFilesystemRepo() *FilesystemRepo { return &FilesystemRepo{} }

// WorkingDirectory returns the process working directory unless ctx is already cancelled.
func (r *FilesystemRepo) WorkingDirectory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("ctx.Err: %w", err)
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("os.Getwd: %w", err)
	}
	return directory, nil
}

// Walk visits entries below root, propagates cancellation, and closes the rooted filesystem before returning.
func (r *FilesystemRepo) Walk(ctx context.Context, root string, visit fs.WalkDirFunc) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return fmt.Errorf("filesystem.Open: %w", err)
	}
	walkErr := fs.WalkDir(disk, ".", func(name string, entry fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if err := visit(name, entry, err); err != nil {
			return fmt.Errorf("visit: %w", err)
		}
		return nil
	})
	if walkErr != nil {
		walkErr = fmt.Errorf("filesystem.WalkDir: %w", walkErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return errors.Join(walkErr, closeErr)
}

// Lstat reports contained path metadata without following the final symlink.
func (r *FilesystemRepo) Lstat(ctx context.Context, root, name string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	info, statErr := disk.Lstat(name)
	if statErr != nil {
		statErr = fmt.Errorf("filesystem.Lstat: %w", statErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return info, errors.Join(statErr, closeErr)
}

// RegularFile reports false for missing paths, symlinks, directories, and other non-regular entries.
func (r *FilesystemRepo) RegularFile(ctx context.Context, root, name string) (bool, error) {
	info, err := r.Lstat(ctx, root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("r.Lstat: %w", err)
	}
	return info.Mode().IsRegular() && info.Mode()&fs.ModeSymlink == 0, nil
}

// Directory reports false for missing paths, symlinks, and non-directory entries.
func (r *FilesystemRepo) Directory(ctx context.Context, root, name string) (bool, error) {
	info, err := r.Lstat(ctx, root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("r.Lstat: %w", err)
	}
	return info.IsDir() && info.Mode()&fs.ModeSymlink == 0, nil
}

// ReadBytes reads a path through a filesystem rooted at root and closes that filesystem before returning.
func (r *FilesystemRepo) ReadBytes(ctx context.Context, root, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	data, readErr := fs.ReadFile(disk, name)
	if readErr != nil {
		readErr = fmt.Errorf("filesystem.ReadFile: %w", readErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return data, errors.Join(readErr, closeErr)
}

// ReadFile returns a regular non-symlink file below root with its relative path and permission bits.
func (r *FilesystemRepo) ReadFile(ctx context.Context, root, name string) (contract.File, error) {
	if err := ctx.Err(); err != nil {
		return contract.File{}, fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return contract.File{}, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()
	info, err := disk.Lstat(name)
	if err != nil {
		return contract.File{}, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return contract.File{}, fmt.Errorf("path is not a regular non-symlink file: %s", name)
	}
	content, err := fs.ReadFile(disk, name)
	if err != nil {
		return contract.File{}, fmt.Errorf("filesystem.ReadFile: %w", err)
	}
	return contract.File{Path: name, Content: content, Mode: uint32(info.Mode().Perm())}, nil
}

// ReadTree returns all regular non-symlink files below a contained directory.
func (r *FilesystemRepo) ReadTree(ctx context.Context, root, directory string) ([]contract.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = disk.Close() }()

	var files []contract.File
	err = fs.WalkDir(disk, directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink in contract tree: %s", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("entry.Info: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("path is not a regular file: %s", name)
		}
		content, err := fs.ReadFile(disk, name)
		if err != nil {
			return fmt.Errorf("filesystem.ReadFile: %w", err)
		}
		files = append(files, contract.File{Path: path.Clean(name), Content: content, Mode: uint32(info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("filesystem.WalkDir: %w", err)
	}
	return files, nil
}

// PublishFile atomically publishes content below root and reports creation, replacement, or equality.
func (r *FilesystemRepo) PublishFile(
	ctx context.Context,
	root, target string,
	content []byte,
) (artifact.PublishResult, error) {
	r.publication.Lock()
	defer r.publication.Unlock()

	disk, err := devfs.Open(root)
	if err != nil {
		return artifact.PublishResult{}, fmt.Errorf("filesystem.Open: %w", err)
	}
	_, statErr := disk.Lstat(target)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		_ = disk.Close()
		return artifact.PublishResult{}, fmt.Errorf("filesystem.Lstat: %w", statErr)
	}
	changed, publishErr := disk.PublishFile(ctx, target, devfs.File{Content: content, Mode: 0o644})
	if publishErr != nil {
		publishErr = fmt.Errorf("filesystem.PublishFile: %w", publishErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	result := artifact.PublishResult{Action: publishAction(exists, changed)}
	return result, errors.Join(publishErr, closeErr)
}

// PublishDirectory atomically replaces target with the complete tree and reports precise file effects.
func (r *FilesystemRepo) PublishDirectory(
	ctx context.Context,
	root, target string,
	tree artifact.Tree,
) (artifact.PublishResult, error) {
	r.publication.Lock()
	defer r.publication.Unlock()

	disk, err := devfs.Open(root)
	if err != nil {
		return artifact.PublishResult{}, fmt.Errorf("filesystem.Open: %w", err)
	}
	snapshot := make(devfs.Snapshot, len(tree.Files))
	for _, file := range tree.Files {
		mode := fs.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		snapshot[file.Path] = devfs.File{Content: file.Content, Mode: mode}
	}
	previous, exists, inspectErr := inspectPublishedTree(ctx, disk, target)
	if inspectErr != nil {
		_ = disk.Close()
		return artifact.PublishResult{}, fmt.Errorf("inspectPublishedTree: %w", inspectErr)
	}
	changed, publishErr := disk.PublishDirectory(ctx, target, snapshot)
	if publishErr != nil {
		publishErr = fmt.Errorf("filesystem.PublishDirectory: %w", publishErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	result := artifact.PublishResult{
		Action:  publishAction(exists, changed),
		Changes: publicationChanges(previous, snapshot),
	}
	return result, errors.Join(publishErr, closeErr)
}

func publishAction(existed, changed bool) artifact.PublishAction {
	if !changed {
		return artifact.PublishUnchanged
	}
	if existed {
		return artifact.PublishUpdated
	}
	return artifact.PublishCreated
}

func inspectPublishedTree(
	ctx context.Context,
	disk *devfs.OS,
	target string,
) (devfs.Snapshot, bool, error) {
	info, err := disk.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return devfs.Snapshot{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return nil, true, &fs.PathError{Op: "publishdirectory", Path: target, Err: fs.ErrInvalid}
	}
	result := devfs.Snapshot{}
	err = fs.WalkDir(disk, target, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ctx.Err: %w", err)
		}
		if name == target || entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return &fs.PathError{Op: "publishdirectory", Path: name, Err: fs.ErrInvalid}
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("entry.Info: %w", err)
		}
		if !info.Mode().IsRegular() {
			return &fs.PathError{Op: "publishdirectory", Path: name, Err: fs.ErrInvalid}
		}
		content, err := fs.ReadFile(disk, name)
		if err != nil {
			return fmt.Errorf("filesystem.ReadFile: %w", err)
		}
		relative := strings.TrimPrefix(name, strings.TrimSuffix(target, "/")+"/")
		result[relative] = devfs.File{Content: content, Mode: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		return nil, true, fmt.Errorf("filesystem.WalkDir: %w", err)
	}
	return result, true, nil
}

func publicationChanges(previous, next devfs.Snapshot) []artifact.PublishChange {
	changes := make([]artifact.PublishChange, 0, len(previous)+len(next))
	for name, expected := range next {
		action := artifact.PublishCreated
		if current, exists := previous[name]; exists {
			action = artifact.PublishUpdated
			if current.Mode.Perm() == expected.Mode.Perm() && bytes.Equal(current.Content, expected.Content) {
				action = artifact.PublishUnchanged
			}
		}
		changes = append(changes, artifact.PublishChange{Path: name, Action: action})
	}
	for name := range previous {
		if _, exists := next[name]; !exists {
			changes = append(changes, artifact.PublishChange{Path: name, Action: artifact.PublishRemoved})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// PruneDirectories removes regular child directories absent from keep and returns their names in lexical order.
func (r *FilesystemRepo) PruneDirectories(
	ctx context.Context,
	root, parent string,
	keep []string,
) ([]string, error) {
	r.publication.Lock()
	defer r.publication.Unlock()

	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	removed, pruneErr := prunableDirectories(ctx, disk, parent, keep)
	if pruneErr == nil {
		for _, name := range removed {
			if err := disk.RemoveAll(ctx, path.Join(parent, name)); err != nil {
				pruneErr = fmt.Errorf("filesystem.RemoveAll: %w", err)
				break
			}
		}
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return removed, errors.Join(pruneErr, closeErr)
}

// PreviewPruneDirectories returns the validated stale child directories without removing them.
func (r *FilesystemRepo) PreviewPruneDirectories(
	ctx context.Context,
	root, parent string,
	keep []string,
) ([]string, error) {
	r.publication.Lock()
	defer r.publication.Unlock()

	disk, err := devfs.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	removed, previewErr := prunableDirectories(ctx, disk, parent, keep)
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return removed, errors.Join(previewErr, closeErr)
}

func prunableDirectories(ctx context.Context, disk *devfs.OS, parent string, keep []string) ([]string, error) {
	info, err := disk.Lstat(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return nil, &fs.PathError{Op: "prune", Path: parent, Err: fs.ErrInvalid}
	}
	entries, err := fs.ReadDir(disk, parent)
	if err != nil {
		return nil, fmt.Errorf("filesystem.ReadDir: %w", err)
	}
	kept := make(map[string]struct{}, len(keep))
	for _, name := range keep {
		kept[name] = struct{}{}
	}
	removed := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("ctx.Err: %w", err)
		}
		if _, ok := kept[entry.Name()]; ok {
			continue
		}
		target := path.Join(parent, entry.Name())
		info, err := disk.Lstat(target)
		if err != nil {
			return nil, fmt.Errorf("filesystem.Lstat: %w", err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return nil, &fs.PathError{Op: "prune", Path: target, Err: fs.ErrInvalid}
		}
		removed = append(removed, entry.Name())
	}
	return removed, nil
}
