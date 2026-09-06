package project_test

import (
	"context"
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/project/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceValidateDBShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		database projectdomain.DB
		issues   []projectdomain.Issue
	}{
		{
			name:     "empty connection list",
			database: projectdomain.DB{},
			issues: []projectdomain.Issue{{
				Code: projectdomain.IssueDBConnectionInvalid, Path: "/project/devctl.yaml", Field: "components.db.connections",
			}},
		},
		{
			name: "single variant with mismatched explicit default",
			database: projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "primary", Default: "postgres", Variants: []projectdomain.DBVariant{{Name: "sqlite", Kind: "sqlite"}},
			}}},
			issues: []projectdomain.Issue{{
				Code: projectdomain.IssueDBDefaultInvalid, Path: "/project/devctl.yaml", Field: "components.db.connections.primary.default",
			}},
		},
		{
			name: "single variant with inferred default",
			database: projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "primary", Variants: []projectdomain.DBVariant{{Name: "sqlite", Kind: "sqlite"}},
			}}},
		},
		{
			name: "single variant with matching explicit default",
			database: projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "primary", Default: "sqlite", Variants: []projectdomain.DBVariant{{Name: "sqlite", Kind: "sqlite"}},
			}}},
		},
		{
			name: "single clickhouse variant",
			database: projectdomain.DB{Connections: []projectdomain.DBConnection{{
				Name: "analytics", Default: "clickhouse", Variants: []projectdomain.DBVariant{{Name: "clickhouse", Kind: "clickhouse"}},
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			manifests := mocks.NewMockManifestRepository(ctrl)
			workspace := mocks.NewMockManifestLocator(ctrl)
			selected := projectdomain.Project{
				Root: "/project", ManifestPath: "/project/devctl.yaml",
				Manifest: projectdomain.Manifest{
					Version:    1,
					Project:    projectdomain.Identity{Name: "example", Language: "go"},
					Components: projectdomain.Components{DB: &test.database},
					Languages:  projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
				},
			}
			gomock.InOrder(
				workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
				manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
			)

			result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
				context.Background(),
				projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
			)

			require.NoError(t, err)
			require.Equal(t, len(test.issues) == 0, result.IsValid())
			require.Equal(t, test.issues, result.Issues)
		})
	}
}

func TestServiceValidateRejectsInvalidMigrations(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	manifests := mocks.NewMockManifestRepository(ctrl)
	workspace := mocks.NewMockManifestLocator(ctrl)
	selected := projectdomain.Project{
		Root: "/project", ManifestPath: "/project/devctl.yaml",
		Manifest: projectdomain.Manifest{
			Version: 1, Project: projectdomain.Identity{Name: "example", Language: "go"},
			Components: projectdomain.Components{DB: &projectdomain.DB{Connections: []projectdomain.DBConnection{
				{Name: "primary", Default: "sqlite", Variants: []projectdomain.DBVariant{{
					Name: "sqlite", Kind: "sqlite", Migrations: &projectdomain.DBMigrations{
						Path: "../outside", DatabaseEnv: "bad-env", DatabaseDefault: "postgres://localhost/app",
					},
				}}},
				{Name: "analytics", Default: "clickhouse", Variants: []projectdomain.DBVariant{{
					Name: "clickhouse", Kind: "clickhouse", Migrations: &projectdomain.DBMigrations{
						Path: "migrations/analytics/clickhouse", DatabaseEnv: "DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL", DatabaseDefault: "postgres://localhost/wrong",
					},
				}}},
			}}},
			Languages: projectdomain.Languages{Go: projectdomain.GoLanguage{Module: "example.test/example"}},
		},
	}
	gomock.InOrder(
		workspace.EXPECT().WorkingDirectory(gomock.Any()).Return("/project", nil),
		manifests.EXPECT().Load(gomock.Any(), "/project/devctl.yaml").Return(projectdomain.LoadManifestResult{Project: selected}, nil),
	)

	result, err := newValidationService(ctrl, manifests, workspace, selected).Validate(
		context.Background(), projectdomain.ValidateQuery{ManifestPath: "devctl.yaml"},
	)

	require.NoError(t, err)
	require.Equal(t, []projectdomain.Issue{
		{Code: "db_migrations_invalid", Path: "/project/devctl.yaml", Field: "components.db.connections.primary.variants.sqlite.migrations"},
		{Code: "db_migrations_invalid", Path: "/project/devctl.yaml", Field: "components.db.connections.analytics.variants.clickhouse.migrations"},
	}, result.Issues)
}
