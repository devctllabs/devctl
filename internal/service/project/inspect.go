package project

import (
	"context"
	"fmt"
	"path"
	"sort"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
)

// Inspect returns an effective project view with defaults applied to a valid selected manifest.
func (s *Service) Inspect(ctx context.Context, query projectdomain.InspectQuery) (projectdomain.InspectResult, error) {
	selected, err := s.loadValidProject(ctx, query.ManifestPath)
	if err != nil {
		return projectdomain.InspectResult{}, err
	}
	targets := projectdomain.NewTargetCatalog(selected.Manifest).All()
	inspection, err := inspectProject(selected, targets)
	if err != nil {
		return projectdomain.InspectResult{}, err
	}
	inspection.Targets = s.inspectionTargets(ctx, selected, targets)
	return projectdomain.InspectResult{Project: inspection}, nil
}

func inspectProject(selected projectdomain.Project, targets []projectdomain.Target) (projectdomain.Inspection, error) {
	manifest := selected.Manifest
	catalog, err := projectdomain.NewRuntimeConfigCatalog(manifest)
	if err != nil {
		return projectdomain.Inspection{}, fmt.Errorf("project.NewRuntimeConfigCatalog: %w", err)
	}
	return projectdomain.Inspection{
		Root: selected.Root, ManifestPath: selected.ManifestPath,
		Name: manifest.Project.Name, Language: manifest.Project.Language,
		Module: manifest.Languages.Go.Module, EnvPrefix: catalog.Prefix(),
		Paths: effectivePaths(manifest), Targets: inspectionTargets(targets),
		Env: effectiveEnv(catalog), Resources: effectiveResources(manifest, catalog.Prefix()),
	}, nil
}

func effectivePaths(manifest projectdomain.Manifest) projectdomain.Paths {
	paths := projectdomain.Paths{ExternalContracts: "api/external", ConfigOut: "gen/config", ServerOut: "gen/serverhttp", ClientOut: "gen/clienthttp"}
	if manifest.Paths.ExternalContracts != "" {
		paths.ExternalContracts = manifest.Paths.ExternalContracts
	}
	for _, target := range projectdomain.NewTargetCatalog(manifest).All() {
		switch {
		case target.ID == "config":
			paths.ConfigOut = target.OutputDir
		case target.ID == "http-server":
			paths.ServerOut = target.OutputDir
		case target.Family == "http" && target.Role == "client":
			paths.ClientOut = path.Dir(target.OutputDir)
		}
	}
	return paths
}

func inspectionTargets(targets []projectdomain.Target) []projectdomain.InspectionTarget {
	result := make([]projectdomain.InspectionTarget, len(targets))
	for index, target := range targets {
		result[index] = projectdomain.InspectionTarget{
			ID: target.ID, Family: target.Family, Format: target.Format,
			Input: target.Input, Config: target.Config, Output: target.OutputDir,
		}
	}
	return result
}

func (s *Service) inspectionTargets(
	ctx context.Context,
	selected projectdomain.Project,
	targets []projectdomain.Target,
) []projectdomain.InspectionTarget {
	result := inspectionTargets(targets)
	for index, target := range targets {
		if target.Source.Type != projectdomain.SourceDevctl || target.Family != "grpc" && target.Family != "kafka" {
			continue
		}
		resolved, err := s.inputs.Resolve(ctx, selected, target)
		if err == nil {
			result[index].ResolvedInput = resolved.Input
		}
	}
	return result
}

func effectiveResources(manifest projectdomain.Manifest, envPrefix string) projectdomain.InspectionResources {
	resources := projectdomain.InspectionResources{}
	appendDBResources(&resources, manifest.Components.DB, envPrefix)
	if manifest.Components.Redis != nil {
		for _, value := range manifest.Components.Redis.Connections {
			resources.RedisConnections = append(resources.RedisConnections, value.Name)
		}
	}
	if manifest.Components.S3 != nil {
		for _, value := range manifest.Components.S3.Connections {
			resources.S3Connections = append(resources.S3Connections, value.Name)
		}
		for _, value := range manifest.Components.S3.Buckets {
			resources.S3Buckets = append(resources.S3Buckets, value.Name)
		}
	}
	sort.Strings(resources.DBConnections)
	sort.Strings(resources.RedisConnections)
	sort.Strings(resources.S3Connections)
	sort.Strings(resources.S3Buckets)
	sort.Slice(resources.Migrations, func(i, j int) bool {
		left, right := resources.Migrations[i], resources.Migrations[j]
		if left.Connection != right.Connection {
			return left.Connection < right.Connection
		}
		return left.Variant < right.Variant
	})
	return resources
}

func appendDBResources(resources *projectdomain.InspectionResources, database *projectdomain.DB, envPrefix string) {
	if database == nil {
		return
	}
	for _, connection := range database.Connections {
		resources.DBConnections = append(resources.DBConnections, connection.Name)
		appendMigrations(resources, connection, envPrefix)
	}
}

func appendMigrations(resources *projectdomain.InspectionResources, connection projectdomain.DBConnection, envPrefix string) {
	for _, variant := range connection.Variants {
		migrations := variant.Migrations
		if migrations == nil {
			continue
		}
		resources.Migrations = append(resources.Migrations, projectdomain.InspectionMigration{
			Connection: connection.Name, Variant: variant.Name, Kind: variant.Kind,
			Path: migrations.Path, DatabaseEnv: envPrefix + migrations.DatabaseEnv,
		})
	}
}
