package project

import (
	"context"
	"fmt"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

// Validate collects all available structural, semantic, and project-readiness issues.
// Invalid project data is returned as issues; access and persistence failures are errors.
func (s *Service) Validate(ctx context.Context, query projectdomain.ValidateQuery) (projectdomain.ValidationResult, error) {
	manifestPath, err := s.resolveManifestPath(ctx, query.ManifestPath)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationLoadManifest, query.ManifestPath, manifestAccessFailure(err), err)
		return projectdomain.ValidationResult{}, fmt.Errorf("s.resolveManifestPath: %w", operationErr)
	}
	loaded, err := s.manifests.Load(ctx, manifestPath)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationLoadManifest, manifestPath, manifestAccessFailure(err), err)
		return projectdomain.ValidationResult{}, fmt.Errorf("manifests.Load: %w", operationErr)
	}
	if len(loaded.Issues) > 0 {
		issues := validationIssues(selectedManifestPath(loaded.Project.ManifestPath, manifestPath), loaded.Issues)
		return projectdomain.ValidationResult{Issues: issues}, nil
	}

	issues := projectdomain.Validate(loaded.Project)
	readinessIssues, err := s.readiness.Check(ctx, loaded.Project)
	if err != nil {
		return projectdomain.ValidationResult{}, fmt.Errorf("readiness.Check: %w", err)
	}
	issues = append(issues, readinessIssues...)
	return projectdomain.ValidationResult{Issues: issues}, nil
}
