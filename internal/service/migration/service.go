package migration

import (
	"context"
	"fmt"
	"strings"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

//go:generate go tool mockgen -destination mocks/service.go -package mocks -typed . ProjectRepository,DatabaseConfigResolver,MigrationRepository

type ProjectRepository interface {
	// LoadProject returns the semantically valid Project without global readiness checks.
	LoadProject(ctx context.Context, manifestPath string) (projectdomain.Project, error)
}

type DatabaseConfigResolver interface {
	// ResolveDatabaseURL resolves migration-only sources without mutating process environment.
	ResolveDatabaseURL(ctx context.Context, config migrationdomain.DatabaseConfig) (string, error)
}

type MigrationRepository interface {
	// Execute runs an operation and returns observed state, including partial progress on failure.
	Execute(ctx context.Context, execution migrationdomain.Execution) (migrationdomain.Result, error)
	// Create creates a new SQL pair without overwriting existing files or connecting to a database.
	Create(ctx context.Context, creation migrationdomain.Creation) (migrationdomain.Result, error)
}

type Service struct {
	projects   ProjectRepository
	config     DatabaseConfigResolver
	migrations MigrationRepository
}

func New(projects ProjectRepository, config DatabaseConfigResolver, migrations MigrationRepository) *Service {
	return &Service{projects: projects, config: config, migrations: migrations}
}

// Migrate operates on one selected Migration Target.
func (s *Service) Migrate(ctx context.Context, command migrationdomain.Command) (migrationdomain.Result, error) {
	if err := ctx.Err(); err != nil {
		return migrationdomain.Result{}, fmt.Errorf("ctx.Err: %w", err)
	}
	if err := validateCommand(command); err != nil {
		return migrationdomain.Result{}, err
	}
	selected, err := s.projects.LoadProject(ctx, command.ManifestPath)
	if err != nil {
		return migrationdomain.Result{}, fmt.Errorf("projects.LoadProject: %w", err)
	}
	target, err := selectTarget(selected.Manifest, command.Target)
	if err != nil {
		return migrationdomain.Result{}, err
	}
	if command.Operation == migrationdomain.Create {
		result, err := s.migrations.Create(ctx, migrationdomain.Creation{Root: selected.Root, Target: target, Name: command.Name})
		result.Target = target.ID()
		if err != nil {
			return result, fmt.Errorf("migrations.Create: %w", err)
		}
		return result, nil
	}
	databaseURL, err := s.config.ResolveDatabaseURL(ctx, migrationdomain.DatabaseConfig{
		Kind: target.Kind, EnvKey: target.DatabaseEnv, Default: target.DatabaseDefault, EnvFile: command.EnvFile,
	})
	if err != nil {
		return migrationdomain.Result{}, fmt.Errorf("config.ResolveDatabaseURL: %w", err)
	}
	result, err := s.migrations.Execute(ctx, migrationdomain.Execution{
		Root: selected.Root, Target: target, Operation: command.Operation, DatabaseURL: databaseURL, Steps: command.Steps,
	})
	result.Target = target.ID()
	if err != nil {
		return result, fmt.Errorf("migrations.Execute: %w", err)
	}
	return result, nil
}

func validateCommand(command migrationdomain.Command) error {
	switch command.Operation {
	case migrationdomain.Create:
		if !migrationdomain.ValidName(command.Name) {
			return &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "invalid_name"}
		}
	case migrationdomain.Up:
		if command.Steps < 0 {
			return &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "invalid_steps"}
		}
	case migrationdomain.Down:
		if command.Steps <= 0 {
			return &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "invalid_steps"}
		}
	case migrationdomain.Status:
	default:
		return &migrationdomain.Error{Kind: failure.Unsupported, Reason: "unsupported_operation"}
	}
	return nil
}

func selectTarget(manifest projectdomain.Manifest, id string) (projectdomain.MigrationTarget, error) {
	targets := projectdomain.MigrationTargets(manifest)
	if id == "" && len(targets) == 1 {
		return targets[0], nil
	}
	for _, target := range targets {
		if target.ID() == id {
			return target, nil
		}
	}
	if id == "" && len(targets) > 1 {
		candidates := make([]string, len(targets))
		for index, target := range targets {
			candidates[index] = target.ID()
		}
		return projectdomain.MigrationTarget{}, &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "target_required", Candidates: candidates}
	}
	if disabledTarget(manifest, id) {
		return projectdomain.MigrationTarget{}, &migrationdomain.Error{Kind: failure.Unsupported, Reason: "migrations_disabled", Target: id}
	}
	return projectdomain.MigrationTarget{}, &migrationdomain.Error{Kind: failure.NotFound, Reason: "target_not_found", Target: id}
}

func disabledTarget(manifest projectdomain.Manifest, id string) bool {
	if manifest.Components.DB == nil {
		return false
	}
	connectionName, variantName, _ := strings.Cut(id, ":")
	for _, connection := range manifest.Components.DB.Connections {
		if connection.Name != connectionName {
			continue
		}
		for _, variant := range connection.Variants {
			if variant.Name == variantName && variant.Migrations == nil {
				return true
			}
		}
	}
	return false
}
