package manifest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	devfs "github.com/devctllabs/go-libs/filesystem"
	"gopkg.in/yaml.v3"
)

// FilesystemRepo maps canonical manifest YAML to and from project domain data.
type FilesystemRepo struct{}

func NewFilesystemRepo() *FilesystemRepo { return &FilesystemRepo{} }

// Load decodes selectedPath, returning syntax and type issues as data rather than execution errors.
func (r *FilesystemRepo) Load(ctx context.Context, selectedPath string) (projectdomain.LoadManifestResult, error) {
	if err := ctx.Err(); err != nil {
		return projectdomain.LoadManifestResult{}, fmt.Errorf("ctx.Err: %w", err)
	}
	manifestPath := filepath.Clean(selectedPath)
	data, err := readManifestFile(manifestPath)
	if err != nil {
		return projectdomain.LoadManifestResult{}, fmt.Errorf("readManifestFile: %w", err)
	}
	document, issues, parseErr := parse(data)
	project := projectdomain.Project{Root: filepath.Dir(manifestPath), ManifestPath: manifestPath}
	if len(issues) > 0 {
		return projectdomain.LoadManifestResult{Project: project, Issues: issues}, nil
	}
	if parseErr != nil {
		return projectdomain.LoadManifestResult{}, fmt.Errorf("parse: %w", parseErr)
	}
	project.Manifest = toProjectSpec(document)
	return projectdomain.LoadManifestResult{Project: project}, nil
}

// Save canonically encodes and atomically publishes project.Manifest at project.ManifestPath.
func (r *FilesystemRepo) Save(ctx context.Context, project projectdomain.Project) (bool, error) {
	data, err := encode(project.Manifest)
	if err != nil {
		return false, fmt.Errorf("encode: %w", err)
	}
	changed, err := publish(ctx, project.ManifestPath, data)
	if err != nil {
		return false, fmt.Errorf("publish: %w", err)
	}
	return changed, nil
}

func encode(manifest projectdomain.Manifest) ([]byte, error) {
	var buffer strings.Builder
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(fromProjectManifest(manifest)); err != nil {
		return nil, fmt.Errorf("encoder.Encode: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encoder.Close: %w", err)
	}
	return []byte(buffer.String()), nil
}

func publish(ctx context.Context, path string, data []byte) (bool, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, fmt.Errorf("os.MkdirAll: %w", err)
	}
	disk, err := devfs.Open(directory)
	if err != nil {
		return false, fmt.Errorf("filesystem.Open: %w", err)
	}
	changed, publishErr := disk.PublishFile(ctx, filepath.Base(path), devfs.File{Content: data, Mode: 0o644})
	if publishErr != nil {
		publishErr = fmt.Errorf("filesystem.PublishFile: %w", publishErr)
	}
	closeErr := disk.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("filesystem.Close: %w", closeErr)
	}
	return changed, errors.Join(publishErr, closeErr)
}

func readManifestFile(path string) ([]byte, error) {
	rootFS, err := devfs.Open(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("filesystem.Open: %w", err)
	}
	defer func() { _ = rootFS.Close() }()
	name := filepath.Base(path)
	info, err := rootFS.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("filesystem.Lstat: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular non-symlink file", name)
	}
	data, err := fs.ReadFile(rootFS, name)
	if err != nil {
		return nil, fmt.Errorf("filesystem.ReadFile: %w", err)
	}
	return data, nil
}
