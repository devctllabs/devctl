package generate

import (
	"context"
	"fmt"
	"path"
	"sort"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

func (s *Service) publish(ctx context.Context, project projectdomain.Project, target projectdomain.Target, output generatedomain.Output) ([]generatedomain.Change, error) {
	var changes []generatedomain.Change
	published, err := s.workspace.PublishDirectory(ctx, project.Root, target.OutputDir, output.Directory)
	if err != nil {
		return changes, fmt.Errorf("workspace.PublishDirectory: %w", err)
	}
	for _, change := range published.Changes {
		changes = append(changes, generatedomain.Change{
			Target: target.ID, Path: path.Join(target.OutputDir, change.Path),
			Action: generatedomain.ChangeAction(change.Action),
		})
	}
	for _, file := range sortedArtifacts(output.Files) {
		published, err = s.workspace.PublishFile(ctx, project.Root, file.Path, file.Content)
		if err != nil {
			return changes, fmt.Errorf("workspace.PublishFile: %w", err)
		}
		changes = append(changes, generatedomain.Change{
			Target: target.ID, Path: file.Path, Action: generatedomain.ChangeAction(published.Action),
		})
	}
	return changes, nil
}

func sortedArtifacts(tree artifact.Tree) []artifact.File {
	files := append([]artifact.File(nil), tree.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}
