package project

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

func (s *Service) loadValidProject(ctx context.Context, manifestPath string) (projectdomain.Project, error) {
	selectedPath, err := s.resolveManifestPath(ctx, manifestPath)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationLoadManifest, manifestPath, manifestAccessFailure(err), err)
		return projectdomain.Project{}, fmt.Errorf("s.resolveManifestPath: %w", operationErr)
	}
	loaded, err := s.manifests.Load(ctx, selectedPath)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationLoadManifest, selectedPath, manifestAccessFailure(err), err)
		return projectdomain.Project{}, fmt.Errorf("manifests.Load: %w", operationErr)
	}
	if len(loaded.Issues) > 0 {
		path := selectedManifestPath(loaded.Project.ManifestPath, selectedPath)
		return projectdomain.Project{}, &projectdomain.InvalidManifestError{
			Path:   path,
			Issues: validationIssues(path, loaded.Issues),
		}
	}
	issues := projectdomain.Validate(loaded.Project)
	if len(issues) > 0 {
		return projectdomain.Project{}, &projectdomain.InvalidManifestError{Path: loaded.Project.ManifestPath, Issues: issues}
	}
	return loaded.Project, nil
}

func (s *Service) resolveManifestPath(ctx context.Context, selected string) (string, error) {
	if filepath.IsAbs(selected) {
		return filepath.Clean(selected), nil
	}
	directory, err := s.locator.WorkingDirectory(ctx)
	if err != nil {
		return "", fmt.Errorf("workspace.WorkingDirectory: %w", err)
	}
	if selected != "" {
		return filepath.Join(directory, selected), nil
	}
	for {
		exists, err := s.locator.RegularFile(ctx, directory, manifestFilename)
		if err != nil {
			return "", fmt.Errorf("workspace.RegularFile: %w", err)
		}
		if exists {
			return filepath.Join(directory, manifestFilename), nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("%s not found: %w", manifestFilename, fs.ErrNotExist)
		}
		directory = parent
	}
}

// LoadProject resolves a structurally and semantically valid project for other
// capability services. Readiness checks remain exclusive to Validate.
func (s *Service) LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error) {
	return s.loadValidProject(ctx, manifestPath)
}

func validationIssues(path string, documentIssues []projectdomain.DecodeIssue) []projectdomain.Issue {
	issues := make([]projectdomain.Issue, 0, len(documentIssues))
	for _, documentIssue := range documentIssues {
		issues = append(issues, projectdomain.Issue{
			Code: documentIssueCode(documentIssue.Kind), Path: path, Field: documentIssue.Field,
			Line: documentIssue.Line, Column: documentIssue.Column,
		})
	}
	return issues
}

func documentIssueCode(kind projectdomain.DecodeIssueKind) projectdomain.IssueCode {
	switch kind {
	case projectdomain.DecodeYAMLInvalid:
		return projectdomain.IssueYAMLInvalid
	case projectdomain.DecodeSchemaInvalid:
		return projectdomain.IssueSchemaInvalid
	case projectdomain.DecodeDuplicateKey:
		return projectdomain.IssueYAMLDuplicateKey
	case projectdomain.DecodeUnknownField:
		return projectdomain.IssueSchemaUnknownField
	default:
		return projectdomain.IssueSchemaInvalid
	}
}

func selectedManifestPath(selectedPath, requestedPath string) string {
	if selectedPath != "" {
		return selectedPath
	}
	return requestedPath
}
