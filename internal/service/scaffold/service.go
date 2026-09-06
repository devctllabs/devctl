package scaffold

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	scaffolddomain "github.com/devctllabs/devctl/internal/domain/scaffold"
	"go.uber.org/zap"
)

type Service struct {
	logger    *zap.Logger
	projects  ProjectRepository
	workspace WorkspaceRepository
}

func New(logger *zap.Logger, projects ProjectRepository, workspace WorkspaceRepository) *Service {
	return &Service{logger: logger, projects: projects, workspace: workspace}
}

// Scaffold preflights the complete artifact plan, then publishes files in deterministic order.
// A publication error returns changes for earlier files without rolling them back.
func (s *Service) Scaffold(ctx context.Context, command scaffolddomain.Command) (scaffolddomain.Result, error) {
	result := scaffolddomain.Result{Files: []scaffolddomain.FileChange{}}
	project, err := s.projects.LoadProject(ctx, command.ManifestPath)
	if err != nil {
		return result, fmt.Errorf("projects.LoadProject: %w", err)
	}
	artifacts, err := plan(project.Manifest)
	if err != nil {
		return result, &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPlan, Kind: scaffolddomain.FailureInternal, Cause: err}
	}
	conflicts, err := preflight(ctx, s.workspace, preflightRequest{root: project.Root, artifacts: artifacts})
	if err != nil {
		operationErr := &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPreflight, Kind: scaffolddomain.FailureUnavailable, Cause: err}
		return result, fmt.Errorf("preflight: %w", operationErr)
	}
	if len(conflicts) > 0 {
		return result, &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPreflight, Path: conflicts[0].path, Kind: scaffolddomain.FailureConflict}
	}
	result.Files, err = s.publish(ctx, project.Root, artifacts)
	if err != nil {
		if ctx.Err() != nil {
			return result, errors.Join(fmt.Errorf("ctx.Err: %w", ctx.Err()), err)
		}
		return result, err
	}
	s.logger.Debug("scaffold completed", zap.Int("files", len(result.Files)))
	return result, nil
}

func (s *Service) publish(ctx context.Context, root string, artifacts []Artifact) ([]scaffolddomain.FileChange, error) {
	changes := make([]scaffolddomain.FileChange, 0, len(artifacts))
	for _, artifact := range artifacts {
		change, err := s.publishArtifact(ctx, root, artifact)
		if err != nil {
			return changes, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func (s *Service) publishArtifact(ctx context.Context, root string, artifact Artifact) (scaffolddomain.FileChange, error) {
	if err := ctx.Err(); err != nil {
		return scaffolddomain.FileChange{}, fmt.Errorf("ctx.Err: %w", err)
	}
	info, statErr := s.workspace.Lstat(ctx, root, artifact.Path)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		operationErr := &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPreflight, Path: artifact.Path, Kind: scaffolddomain.FailureUnavailable, Cause: statErr}
		return scaffolddomain.FileChange{}, fmt.Errorf("workspace.Lstat: %w", operationErr)
	}
	if exists && info.Mode().IsRegular() {
		existing, readErr := s.workspace.ReadBytes(ctx, root, artifact.Path)
		if readErr != nil {
			operationErr := &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPreflight, Path: artifact.Path, Kind: scaffolddomain.FailureUnavailable, Cause: readErr}
			return scaffolddomain.FileChange{}, fmt.Errorf("workspace.ReadBytes: %w", operationErr)
		}
		if bytes.Equal(existing, artifact.Content) || artifact.CreateOnly {
			return scaffolddomain.FileChange{Path: artifact.Path, Action: scaffolddomain.FileUnchanged}, nil
		}
	}
	published, err := s.workspace.PublishFile(ctx, root, artifact.Path, artifact.Content)
	if err != nil {
		operationErr := &scaffolddomain.OperationError{Operation: scaffolddomain.OperationPublish, Path: artifact.Path, Kind: scaffolddomain.FailureUnavailable, Cause: err}
		return scaffolddomain.FileChange{}, fmt.Errorf("workspace.PublishFile: %w", operationErr)
	}
	action := scaffolddomain.FileAction(published.Action)
	return scaffolddomain.FileChange{Path: artifact.Path, Action: action}, nil
}
