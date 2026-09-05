package contractsnapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
)

const metadataFilename = ".devctl-contract.json"

//go:generate go tool mockgen -destination mocks/loader.go -package mocks -typed . Reader

// Reader provides the contained committed files needed to reconstruct a Snapshot.
type Reader interface {
	// ReadFile returns a regular non-symlink file below root with its relative path and permission bits.
	ReadFile(ctx context.Context, root, name string) (contract.File, error)
	// ReadTree returns every regular non-symlink file below a contained directory in deterministic order.
	ReadTree(ctx context.Context, root, directory string) ([]contract.File, error)
}

// Loader reconstructs and validates committed Contract Snapshots.
type Loader struct {
	reader Reader
}

func New(reader Reader) *Loader {
	return &Loader{reader: reader}
}

// Load reconstructs the committed Snapshot rooted at treeRoot and validates it against expected.
func (l *Loader) Load(
	ctx context.Context,
	root string,
	treeRoot string,
	expected contract.MetadataExpectation,
) (contract.Snapshot, error) {
	metadataFile, err := l.reader.ReadFile(ctx, root, path.Join(treeRoot, metadataFilename))
	if err != nil {
		reason := contract.MetadataNotRegular
		if errors.Is(err, fs.ErrNotExist) {
			reason = contract.MetadataRequired
		}
		return contract.Snapshot{}, metadataError(metadataFilename, reason, err)
	}
	metadata, err := contract.DecodeMetadata(metadataFile.Content)
	if err != nil {
		return contract.Snapshot{}, fmt.Errorf("contract.DecodeMetadata: %w", err)
	}
	if err := contract.ValidateMetadata(metadata, expected); err != nil {
		return contract.Snapshot{}, fmt.Errorf("contract.ValidateMetadata: %w", err)
	}
	if err := l.validateReferences(ctx, root, treeRoot, metadata); err != nil {
		return contract.Snapshot{}, err
	}
	files, err := l.reader.ReadTree(ctx, root, treeRoot)
	if err != nil {
		return contract.Snapshot{}, metadataError("files", contract.MetadataNotRegular, err)
	}
	snapshot := contract.Snapshot{
		ModuleRoot: metadata.ModuleRoot,
		Entrypoint: metadata.Entrypoint,
		Files:      rebaseFiles(files, treeRoot),
		Metadata:   &metadata,
	}
	if err := contract.ValidateSnapshot(snapshot, expected); err != nil {
		return contract.Snapshot{}, fmt.Errorf("contract.ValidateSnapshot: %w", err)
	}
	return snapshot, nil
}

func (l *Loader) validateReferences(
	ctx context.Context,
	root string,
	treeRoot string,
	metadata contract.Metadata,
) error {
	for _, reference := range []struct{ field, name string }{
		{field: "entrypoint", name: metadata.Entrypoint},
		{field: "buf_config", name: metadata.BufConfig},
	} {
		if reference.name == "" {
			continue
		}
		if _, err := l.reader.ReadFile(ctx, root, path.Join(treeRoot, reference.name)); err != nil {
			return referenceError(reference.field, err)
		}
	}
	if metadata.ModuleRoot == "" {
		return nil
	}
	if _, err := l.reader.ReadTree(ctx, root, path.Join(treeRoot, metadata.ModuleRoot)); err != nil {
		return referenceError("module_root", err)
	}
	return nil
}

func referenceError(field string, err error) error {
	reason := contract.MetadataNotRegular
	if errors.Is(err, fs.ErrNotExist) {
		reason = contract.MetadataNotFound
	}
	return metadataError(field, reason, err)
}

func rebaseFiles(files []contract.File, treeRoot string) []contract.File {
	prefix := strings.TrimSuffix(path.Clean(treeRoot), "/") + "/"
	rebased := make([]contract.File, 0, len(files))
	for _, file := range files {
		file.Path = strings.TrimPrefix(file.Path, prefix)
		if file.Path != metadataFilename {
			rebased = append(rebased, file)
		}
	}
	sort.Slice(rebased, func(i, j int) bool { return rebased[i].Path < rebased[j].Path })
	return rebased
}

func metadataError(field string, reason contract.MetadataInvalidReason, cause error) error {
	return &contract.SnapshotMetadataError{Field: field, Reason: reason, Hint: "devctl sync", Cause: cause}
}
