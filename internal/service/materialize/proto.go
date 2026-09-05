package materialize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
)

type protoTreeRequest struct {
	root      string
	reference contract.Reference
	bufConfig string
}

type bufFilesRequest struct {
	root       string
	files      []contract.File
	configPath string
}

func protoTree(ctx context.Context, reader FileReader, request protoTreeRequest) (contract.Snapshot, error) {
	root := request.root
	reference := request.reference
	protoRoot := reference.ProtoRoot
	if protoRoot == "" {
		protoRoot = path.Dir(reference.Entrypoint)
	}
	if !safeProtoSelection(protoRoot, reference.Entrypoint) {
		return contract.Snapshot{}, &materializedomain.OperationError{
			Operation: materializedomain.OperationValidateSource,
			Path:      reference.Entrypoint,
			Kind:      materializedomain.FailureInvalid,
		}
	}
	files, err := reader.ReadTree(ctx, root, protoRoot)
	if err != nil {
		operationErr := &materializedomain.OperationError{Operation: materializedomain.OperationReadFile, Path: protoRoot, Kind: materializedomain.FailureUnavailable, Cause: err}
		return contract.Snapshot{}, fmt.Errorf("reader.ReadTree: %w", operationErr)
	}
	if request.bufConfig != "" {
		files, err = includeBufFiles(ctx, reader, bufFilesRequest{
			root: root, files: files, configPath: request.bufConfig,
		})
		if err != nil {
			return contract.Snapshot{}, err
		}
	}
	return newProtoSnapshot(protoRoot, reference.Entrypoint, files)
}

func includeBufFiles(
	ctx context.Context,
	reader FileReader,
	request bufFilesRequest,
) ([]contract.File, error) {
	root, files, configPath := request.root, request.files, request.configPath
	if !safeRelative(configPath) {
		return nil, &materializedomain.OperationError{
			Operation: materializedomain.OperationValidateSource,
			Path:      configPath,
			Kind:      materializedomain.FailureInvalid,
		}
	}
	config, err := reader.ReadFile(ctx, root, configPath)
	if err != nil {
		return nil, bufReadError(configPath, err)
	}
	config.Path = configPath
	files = replaceFile(files, config)

	lockPath := path.Join(path.Dir(configPath), "buf.lock")
	lock, err := reader.ReadFile(ctx, root, lockPath)
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	if err != nil {
		return nil, bufReadError(lockPath, err)
	}
	lock.Path = lockPath
	return replaceFile(files, lock), nil
}

func replaceFile(files []contract.File, replacement contract.File) []contract.File {
	result := make([]contract.File, 0, len(files)+1)
	for _, file := range files {
		if cleanSnapshotPath(file.Path) != replacement.Path {
			result = append(result, file)
		}
	}
	return append(result, replacement)
}

func bufReadError(name string, cause error) error {
	kind := materializedomain.FailureUnavailable
	if errors.Is(cause, fs.ErrNotExist) {
		kind = materializedomain.FailureNotFound
	}
	operationErr := &materializedomain.OperationError{
		Operation: materializedomain.OperationReadFile,
		Path:      name,
		Kind:      kind,
		Cause:     cause,
	}
	return fmt.Errorf("reader.ReadFile: %w", operationErr)
}

func safeProtoSelection(root, entrypoint string) bool {
	if root != "." && !safeRelative(root) {
		return false
	}
	if !safeRelative(entrypoint) {
		return false
	}
	root = path.Clean(root)
	entrypoint = path.Clean(entrypoint)
	return root == "." || entrypoint == root || strings.HasPrefix(entrypoint, root+"/")
}
