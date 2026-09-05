package project_test

import (
	"testing"

	"github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigCatalogBuildsCanonicalScopedFields(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Project: project.Identity{Name: "sample-api", Language: "go"},
		Components: project.Components{
			HTTP: &project.HTTP{
				Server: &project.HTTPServer{Start: &project.Start{}},
				Env:    project.ComponentEnv{System: []project.EnvVar{{Key: "HTTP_ADDR", Type: "string", Default: ":8088"}}},
			},
			Telemetry: &project.Telemetry{Env: project.ComponentEnv{System: []project.EnvVar{{
				Key: "OTEL_SERVICE_NAME", Type: "string", Default: "sample-api",
			}}}},
			DB: &project.DB{Connections: []project.DBConnection{{
				Name: "primary",
				Variants: []project.DBVariant{{
					Name: "postgres", Kind: "postgres", DSNDefault: "postgres://secret", Secret: true,
					Migrations: &project.DBMigrations{DatabaseEnv: "DB_PRIMARY_MIGRATIONS_URL", DatabaseDefault: "postgres://migration-secret"},
				}},
			}}},
		},
	}

	catalog, err := project.NewRuntimeConfigCatalog(manifest)
	require.NoError(t, err)
	require.Equal(t, "SAMPLE_API_", catalog.Prefix())
	require.Equal(t, []project.RuntimeConfigField{
		{Group: "Telemetry", Name: "ServiceName", Key: "OTEL_SERVICE_NAME", Type: project.RuntimeConfigString, Default: "sample-api", HasDefault: true},
		{Group: "DBPrimary", Name: "Kind", Key: "SAMPLE_API_DB_PRIMARY_KIND", Type: project.RuntimeConfigString, Default: "postgres", HasDefault: true},
		{Group: "DBPrimary", Name: "PostgresDSN", Key: "SAMPLE_API_DB_PRIMARY_POSTGRES_DSN", Type: project.RuntimeConfigString, Secret: true},
		{Group: "Telemetry", Name: "DeploymentEnvironment", Key: "SAMPLE_API_DEPLOYMENT_ENVIRONMENT", Type: project.RuntimeConfigString, Default: "development", HasDefault: true},
		{Group: "HTTP", Name: "Address", Key: "SAMPLE_API_HTTP_ADDR", Type: project.RuntimeConfigString, Default: ":8088", HasDefault: true},
		{Group: "HTTP", Name: "Enabled", Key: "SAMPLE_API_HTTP_SERVER_ENABLED", Type: project.RuntimeConfigBool, Default: false, HasDefault: true},
		{Group: "Telemetry", Name: "ServiceVersion", Key: "SAMPLE_API_SERVICE_VERSION", Type: project.RuntimeConfigString, Default: "dev", HasDefault: true},
	}, catalog.Entries(project.RuntimeConfigRuntime))
	require.Equal(t, []project.RuntimeConfigField{
		{Group: "Migrations", Name: "DBPrimaryPostgresMigrationsURL", Key: "SAMPLE_API_DB_PRIMARY_MIGRATIONS_URL", Type: project.RuntimeConfigString, Secret: true},
	}, onlyMigrationFields(catalog.Entries(project.RuntimeConfigExample)))

	fields := catalog.Entries(project.RuntimeConfigRuntime)
	originalName := fields[0].Name
	fields[0].Name = "Changed"
	require.Equal(t, originalName, catalog.Entries(project.RuntimeConfigRuntime)[0].Name)
}

func TestRuntimeConfigCatalogRejectsConflictingExplicitDeclarations(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Project: project.Identity{Name: "sample", Language: "go"},
		Env: project.Env{Custom: []project.EnvGroup{
			{Group: "First", Vars: []project.EnvVar{{Key: "SHARED", Type: "string", Default: "one"}}},
			{Group: "Second", Vars: []project.EnvVar{{Key: "SHARED", Type: "string", Default: "two"}}},
		}},
	}

	_, err := project.NewRuntimeConfigCatalog(manifest)

	var conflict *project.RuntimeConfigConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "SAMPLE_SHARED", conflict.Key)
	require.NotContains(t, err.Error(), "one")
	require.NotContains(t, err.Error(), "two")
}

func TestRuntimeConfigCatalogSeparatesClickHouseRuntimeAndMigrationURLs(t *testing.T) {
	t.Parallel()

	manifest := project.Manifest{
		Project: project.Identity{Name: "sample", Language: "go"},
		Components: project.Components{DB: &project.DB{Connections: []project.DBConnection{{
			Name: "analytics", Default: "clickhouse", Variants: []project.DBVariant{{
				Name: "clickhouse", Kind: "clickhouse", DSNDefault: "clickhouse://localhost:9000/default", Secret: true,
				Migrations: &project.DBMigrations{Path: "migrations/analytics/clickhouse", DatabaseEnv: "DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL"},
			}},
		}}}},
	}

	catalog, err := project.NewRuntimeConfigCatalog(manifest)
	require.NoError(t, err)
	require.Contains(t, catalog.Entries(project.RuntimeConfigRuntime), project.RuntimeConfigField{
		Group: "DBAnalytics", Name: "ClickhouseDSN", Key: "SAMPLE_DB_ANALYTICS_CLICKHOUSE_DSN",
		Type: project.RuntimeConfigString, Secret: true,
	})
	require.NotContains(t, catalog.Entries(project.RuntimeConfigRuntime), project.RuntimeConfigField{
		Group: "Migrations", Name: "DBAnalyticsClickhouseMigrationsURL", Key: "SAMPLE_DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL",
		Type: project.RuntimeConfigString, Secret: true,
	})
	require.Contains(t, catalog.Entries(project.RuntimeConfigExample), project.RuntimeConfigField{
		Group: "Migrations", Name: "DBAnalyticsClickhouseMigrationsURL", Key: "SAMPLE_DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL",
		Type: project.RuntimeConfigString, Secret: true,
	})
}

func onlyMigrationFields(fields []project.RuntimeConfigField) []project.RuntimeConfigField {
	result := make([]project.RuntimeConfigField, 0, len(fields))
	for _, field := range fields {
		if field.Group == "Migrations" {
			result = append(result, field)
		}
	}
	return result
}
