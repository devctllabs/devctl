package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

// InitManifest creates a canonical manifest, preserves identical content, and requires Force to replace a conflict.
func (s *Service) InitManifest(ctx context.Context, command projectdomain.InitManifestCommand) (projectdomain.ManifestResult, error) {
	if err := validateInitManifest(command); err != nil {
		return projectdomain.ManifestResult{}, err
	}
	destination, err := s.manifestDestination(ctx, command.Destination)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationInitManifest, command.Destination, projectdomain.FailureUnavailable, err)
		return projectdomain.ManifestResult{}, fmt.Errorf("s.manifestDestination: %w", operationErr)
	}
	desired := projectdomain.Project{
		Root: filepath.Dir(destination), ManifestPath: destination, Manifest: initialManifest(command),
	}
	existing, loadErr := s.manifests.Load(ctx, destination)
	existed := loadErr == nil
	switch {
	case loadErr == nil && len(existing.Issues) == 0 && reflect.DeepEqual(existing.Project.Manifest, desired.Manifest):
	case loadErr == nil && !command.Force:
		return projectdomain.ManifestResult{Manifest: destination}, projectOperationError(projectdomain.OperationInitManifest, destination, projectdomain.FailureConflict, errors.New("different content"))
	case loadErr != nil && !errors.Is(loadErr, fs.ErrNotExist):
		operationErr := projectOperationError(projectdomain.OperationLoadManifest, destination, projectdomain.FailureUnavailable, loadErr)
		return projectdomain.ManifestResult{Manifest: destination}, fmt.Errorf("manifests.Load: %w", operationErr)
	}
	changed, err := s.manifests.Save(ctx, desired)
	if err != nil {
		operationErr := projectOperationError(projectdomain.OperationSaveManifest, destination, projectdomain.FailureUnavailable, err)
		return projectdomain.ManifestResult{Manifest: destination}, fmt.Errorf("manifests.Save: %w", operationErr)
	}
	action := projectdomain.ChangeUnchanged
	if !existed {
		action = projectdomain.ChangeCreated
	} else if changed {
		action = projectdomain.ChangeUpdated
	}
	return projectdomain.ManifestResult{Manifest: destination, Change: action}, nil
}

func validateInitManifest(command projectdomain.InitManifestCommand) error {
	if command.Language != "go" || (command.Preset != "cli" && command.Preset != "http-service") || !kebabCase.MatchString(command.Name) || command.Module == "" {
		return projectOperationError(projectdomain.OperationInitManifest, command.Destination, projectdomain.FailureInvalid, errors.New("invalid command"))
	}
	return nil
}

func (s *Service) manifestDestination(ctx context.Context, selected string) (string, error) {
	if selected != "" && filepath.IsAbs(selected) {
		return filepath.Clean(selected), nil
	}
	directory, err := s.locator.WorkingDirectory(ctx)
	if err != nil {
		return "", fmt.Errorf("workspace.WorkingDirectory: %w", err)
	}
	if selected == "" {
		selected = manifestFilename
	}
	return filepath.Join(directory, selected), nil
}

func initialManifest(command projectdomain.InitManifestCommand) projectdomain.Manifest {
	manifest := projectdomain.Manifest{
		Version: 1, Project: projectdomain.Identity{Name: command.Name, Language: "go"},
		Env: projectdomain.Env{}, Paths: projectdomain.ManifestPaths{ExternalContracts: "api/external"},
		Sources: map[string]projectdomain.Source{}, Exports: map[string]projectdomain.Export{},
		Components: projectdomain.Components{Logging: &projectdomain.Logging{Env: projectdomain.ComponentEnv{System: []projectdomain.EnvVar{{Key: "LOG_LEVEL", Type: "string", Default: "info"}}}}},
		Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: command.Module}},
	}
	if command.Preset == "http-service" {
		trueValue, falseValue := true, false
		manifest.Components.HTTP = &projectdomain.HTTP{Server: &projectdomain.HTTPServer{OpenAPI: "api/openapi/swagger.yaml", Start: &projectdomain.Start{Env: "HTTP_SERVER_ENABLED", Default: &trueValue}}, Env: projectdomain.ComponentEnv{System: []projectdomain.EnvVar{{Key: "HTTP_ADDR", Type: "string", Default: ":8080"}}}}
		manifest.Components.Health = &projectdomain.Health{Server: &projectdomain.HealthServer{Start: &projectdomain.Start{Env: "HEALTH_SERVER_ENABLED", Default: &trueValue}}, Env: projectdomain.ComponentEnv{System: []projectdomain.EnvVar{{Key: "HEALTH_ADDR", Type: "string", Default: ":8081"}}}}
		manifest.Components.Telemetry = &projectdomain.Telemetry{Start: &projectdomain.Start{Env: "TELEMETRY_ENABLED", Default: &falseValue}}
		manifest.Languages.Go.Components.Pprof = &projectdomain.Pprof{Server: &projectdomain.PprofServer{Start: &projectdomain.Start{Env: "PPROF_ENABLED", Default: &falseValue}}, Env: projectdomain.ComponentEnv{System: []projectdomain.EnvVar{{Key: "PPROF_ADDR", Type: "string", Default: "127.0.0.1:6060"}}}}
		manifest.Languages.Go.Generators.HTTP = &projectdomain.HTTPGenerator{OAPIConfig: "tools/oapi/server.yaml", ServerOut: "gen/serverhttp", ClientOut: "gen/clienthttp"}
	}
	return manifest
}
