package generate

import (
	"context"
	"fmt"

	"github.com/devctllabs/devctl/internal/domain/artifact"
	generatedomain "github.com/devctllabs/devctl/internal/domain/generate"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"go.uber.org/zap"
)

//go:generate go tool mockgen -destination mocks/service.go -package mocks -typed . ProjectRepository,TargetResolver,GeneratorClient,WorkspaceRepository

// ProjectRepository resolves the valid project selected for generation.
type ProjectRepository interface {
	// LoadProject returns a structurally and semantically valid project or an execution error.
	LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error)
}

// TargetResolver attaches the concrete input required to execute one Target.
type TargetResolver interface {
	// Resolve attaches the concrete input required to execute target in selected Project.
	Resolve(ctx context.Context, selected projectdomain.Project, target projectdomain.Target) (projectdomain.Target, error)
}

// GeneratorClient generates unpublished Managed Output for one supported Target.
type GeneratorClient interface {
	// Generate returns unpublished managed output; it must not write into the project workspace.
	Generate(ctx context.Context, project projectdomain.Project, target projectdomain.Target) (generatedomain.Output, error)
}

// WorkspaceRepository atomically publishes managed generation output.
type WorkspaceRepository interface {
	// PublishFile atomically publishes one contained auxiliary file and reports whether bytes changed.
	PublishFile(ctx context.Context, root, target string, content []byte) (artifact.PublishResult, error)
	// PublishDirectory atomically replaces one contained target with the complete tree and reports whether content changed.
	PublishDirectory(ctx context.Context, root, target string, tree artifact.Tree) (artifact.PublishResult, error)
}

type Service struct {
	logger    *zap.Logger
	projects  ProjectRepository
	inputs    TargetResolver
	generator GeneratorClient
	workspace WorkspaceRepository
}

// Dependencies names the required generation capabilities passed to New.
type Dependencies struct {
	Projects  ProjectRepository
	Inputs    TargetResolver
	Generator GeneratorClient
	Workspace WorkspaceRepository
}

func New(logger *zap.Logger, dependencies Dependencies) *Service {
	return &Service{
		logger: logger, projects: dependencies.Projects,
		inputs: dependencies.Inputs, generator: dependencies.Generator, workspace: dependencies.Workspace,
	}
}

// Generate executes selected targets sequentially in deterministic order and publishes each target atomically.
// Dry-run invokes neither generator nor publisher; a failure returns completed changes without rollback.
func (s *Service) Generate(ctx context.Context, command generatedomain.Command) (generatedomain.Result, error) {
	result := generatedomain.Result{Targets: []string{}, Changes: []generatedomain.Change{}, DryRun: command.DryRun}
	project, err := s.projects.LoadProject(ctx, command.ManifestPath)
	if err != nil {
		return result, fmt.Errorf("projects.LoadProject: %w", err)
	}
	targets, err := generationTargets(project.Manifest, command.Family, command.Target)
	if err != nil {
		return result, fmt.Errorf("generationTargets: %w", err)
	}
	for _, target := range targets {
		changes, targetErr := s.generateOne(ctx, project, target, command.DryRun)
		result.Changes = append(result.Changes, changes...)
		if targetErr != nil {
			return result, targetErr
		}
		result.Targets = append(result.Targets, target.ID)
	}
	s.logger.Debug("generation completed", zap.Int("targets", len(result.Targets)))
	return result, nil
}

func (s *Service) generateOne(
	ctx context.Context,
	project projectdomain.Project,
	target projectdomain.Target,
	dryRun bool,
) ([]generatedomain.Change, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("ctx.Err: %w", err)
	}
	if dryRun {
		return plannedGeneration(target), nil
	}
	resolved := target
	if target.Family != "config" {
		var err error
		resolved, err = s.inputs.Resolve(ctx, project, target)
		if err != nil {
			if target.Family == "http" {
				operationErr := &generatedomain.OperationError{Operation: generatedomain.OperationLocateContract, Target: target.ID, Path: target.Location.Entrypoint, Kind: generatedomain.FailureUnavailable, Cause: err}
				return nil, fmt.Errorf("inputs.Resolve: %w", operationErr)
			}
			return nil, fmt.Errorf("inputs.Resolve: %w", err)
		}
	}
	if resolved.Family == "kafka" && resolved.Format == "raw" {
		return nil, nil
	}
	output, err := s.generateTarget(ctx, project, resolved)
	if err != nil {
		operationErr := &generatedomain.OperationError{Operation: generatedomain.OperationRunGenerator, Target: resolved.ID, Path: resolved.OutputDir, Kind: generatedomain.FailureUnavailable, Cause: err}
		return nil, fmt.Errorf("s.generateTarget: %w", operationErr)
	}
	changes, err := s.publish(ctx, project, resolved, output)
	if err != nil {
		operationErr := &generatedomain.OperationError{Operation: generatedomain.OperationPublishOutput, Target: resolved.ID, Path: resolved.OutputDir, Kind: generatedomain.FailureUnavailable, Cause: err}
		return changes, fmt.Errorf("s.publish: %w", operationErr)
	}
	return changes, nil
}

func (s *Service) generateTarget(ctx context.Context, project projectdomain.Project, target projectdomain.Target) (generatedomain.Output, error) {
	if target.Family == "config" {
		return generateConfig(project.Manifest)
	}
	output, err := s.generator.Generate(ctx, project, target)
	if err != nil {
		return generatedomain.Output{}, fmt.Errorf("generator.Generate: %w", err)
	}
	return output, nil
}
