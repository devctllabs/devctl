package materialize

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/contract"
	materializedomain "github.com/devctllabs/devctl/internal/domain/materialize"
	"gopkg.in/yaml.v3"
)

// referenceClosure follows contained local $ref values from entrypoint and ignores document fragments and remote references.
func referenceClosure(ctx context.Context, reader FileReader, root, entrypoint string) (contract.Snapshot, error) {
	if !safeRelative(entrypoint) {
		return contract.Snapshot{}, &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, Path: entrypoint, Kind: materializedomain.FailureInvalid}
	}
	entrypoint = path.Clean(entrypoint)
	queue := []string{entrypoint}
	files := make([]contract.File, 0)
	seen := make(map[string]struct{})
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, exists := seen[current]; exists {
			continue
		}
		if err := ctx.Err(); err != nil {
			return contract.Snapshot{}, fmt.Errorf("ctx.Err: %w", err)
		}
		file, err := reader.ReadFile(ctx, root, current)
		if err != nil {
			operationErr := &materializedomain.OperationError{Operation: materializedomain.OperationReadFile, Path: current, Kind: materializedomain.FailureUnavailable, Cause: err}
			return contract.Snapshot{}, fmt.Errorf("reader.ReadFile: %w", operationErr)
		}
		file.Path = current
		files = append(files, file)
		seen[current] = struct{}{}
		references, err := localReferences(current, file.Content)
		if err != nil {
			return contract.Snapshot{}, err
		}
		queue = append(queue, references...)
	}
	return newSnapshot(entrypoint, files)
}

func localReferences(current string, data []byte) ([]string, error) {
	resolved := make([]string, 0)
	for _, reference := range collectReferences(data) {
		relative := strings.SplitN(reference, "#", 2)[0]
		if relative == "" || strings.Contains(relative, "://") {
			continue
		}
		name := path.Clean(path.Join(path.Dir(current), relative))
		if !safeRelative(name) {
			return nil, &materializedomain.OperationError{Operation: materializedomain.OperationValidateSource, Path: reference, Kind: materializedomain.FailureInvalid}
		}
		resolved = append(resolved, name)
	}
	return resolved, nil
}

func collectReferences(data []byte) []string {
	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil {
		return nil
	}
	var references []string
	visitYAML(&document, func(key, value *yaml.Node) {
		if key.Value == "$ref" && value.Kind == yaml.ScalarNode {
			references = append(references, value.Value)
		}
	})
	return references
}

func visitYAML(node *yaml.Node, visit func(key, value *yaml.Node)) {
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			visit(node.Content[index], node.Content[index+1])
		}
	}
	for _, child := range node.Content {
		visitYAML(child, visit)
	}
}

func safeRelative(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
