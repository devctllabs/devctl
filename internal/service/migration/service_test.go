package migration

import (
	"context"
	"errors"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/migration/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func migrationProject() projectdomain.Project {
	return projectdomain.Project{Root: "/project", Manifest: projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "orders-api"},
		Components: projectdomain.Components{DB: &projectdomain.DB{Connections: []projectdomain.DBConnection{{
			Name: "primary", Default: "sqlite", Variants: []projectdomain.DBVariant{
				{Name: "sqlite", Kind: "sqlite"},
				{Name: "postgres", Kind: "postgres", Migrations: &projectdomain.DBMigrations{
					Path: "db/migrations", DatabaseEnv: "MIGRATIONS_URL", DatabaseDefault: "postgres://default/db",
				}},
			},
		}}}},
	}}
}

func TestMigrateSelectsOnlyMigrationVariantAndDispatchesOperations(t *testing.T) {
	t.Parallel()
	for _, operation := range []migrationdomain.Operation{migrationdomain.Create, migrationdomain.Up, migrationdomain.Down, migrationdomain.Status} {
		t.Run(string(operation), func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects, config, repository := mocks.NewMockProjectRepository(ctrl), mocks.NewMockDatabaseConfigResolver(ctrl), mocks.NewMockMigrationRepository(ctrl)
			selected := migrationProject()
			target := projectdomain.MigrationTarget{Connection: "primary", Variant: "postgres", Kind: "postgres", Path: "db/migrations", DatabaseEnv: "ORDERS_API_MIGRATIONS_URL", DatabaseDefault: "postgres://default/db"}
			projects.EXPECT().LoadProject(gomock.Any(), "custom.yaml").Return(selected, nil)
			input := migrationdomain.Command{ManifestPath: "custom.yaml", Operation: operation, Name: "create_orders", EnvFile: ".env.ci", Steps: 2}
			if operation == migrationdomain.Status {
				input.Target = target.ID()
			}
			output := migrationdomain.Result{State: &migrationdomain.State{}}
			if operation == migrationdomain.Create {
				output = migrationdomain.Result{Files: []string{"db/migrations/1_create_orders.up.sql", "db/migrations/1_create_orders.down.sql"}, Changed: true}
				repository.EXPECT().Create(gomock.Any(), migrationdomain.Creation{Root: selected.Root, Target: target, Name: input.Name}).Return(output, nil)
			} else {
				config.EXPECT().ResolveDatabaseURL(gomock.Any(), migrationdomain.DatabaseConfig{Kind: "postgres", EnvKey: target.DatabaseEnv, Default: target.DatabaseDefault, EnvFile: ".env.ci"}).Return("postgres://ci/db", nil)
				repository.EXPECT().Execute(gomock.Any(), migrationdomain.Execution{Root: selected.Root, Target: target, Operation: operation, DatabaseURL: "postgres://ci/db", Steps: 2}).Return(output, nil)
			}
			result, err := New(projects, config, repository).Migrate(context.Background(), input)
			require.NoError(t, err)
			output.Target = target.ID()
			require.Equal(t, output, result)
		})
	}
}

func TestMigrateSelectionErrorsDoNotResolveDatabase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		target   string
		multiple bool
		empty    bool
		kind     failure.Category
		reason   string
	}{
		{name: "ambiguous", multiple: true, kind: failure.InvalidInput, reason: "target_required"},
		{name: "unknown", target: "missing:postgres", kind: failure.NotFound, reason: "target_not_found"},
		{name: "disabled", target: "primary:sqlite", kind: failure.Unsupported, reason: "migrations_disabled"},
		{name: "no targets", empty: true, kind: failure.NotFound, reason: "target_not_found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			projects, config, repository := mocks.NewMockProjectRepository(ctrl), mocks.NewMockDatabaseConfigResolver(ctrl), mocks.NewMockMigrationRepository(ctrl)
			selected := migrationProject()
			if test.multiple {
				selected.Manifest.Components.DB.Connections[0].Variants[0].Migrations = &projectdomain.DBMigrations{Path: "migrations/sqlite", DatabaseEnv: "SQLITE_URL"}
			}
			if test.empty {
				selected.Manifest.Components.DB = nil
			}
			projects.EXPECT().LoadProject(gomock.Any(), "").Return(selected, nil)
			_, err := New(projects, config, repository).Migrate(context.Background(), migrationdomain.Command{Operation: migrationdomain.Up, Target: test.target})
			require.Error(t, err)
			require.Equal(t, test.kind, failure.CategoryOf(err))
			var selectedErr *migrationdomain.Error
			require.ErrorAs(t, err, &selectedErr)
			require.Equal(t, test.reason, selectedErr.Reason)
			if test.multiple {
				require.Equal(t, []string{"primary:postgres", "primary:sqlite"}, selectedErr.Candidates)
			}
		})
	}
}

func TestMigratePreservesFailuresAndPartialResults(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	projects, config, repository := mocks.NewMockProjectRepository(ctrl), mocks.NewMockDatabaseConfigResolver(ctrl), mocks.NewMockMigrationRepository(ctrl)
	projects.EXPECT().LoadProject(gomock.Any(), "").Return(migrationProject(), nil)
	config.EXPECT().ResolveDatabaseURL(gomock.Any(), gomock.Any()).Return("postgres://ci/db", nil)
	cause := errors.New("sql failed")
	version := uint(2)
	partial := migrationdomain.Result{State: &migrationdomain.State{Version: &version, Dirty: true}, Changed: true}
	repository.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(partial, cause)
	result, err := New(projects, config, repository).Migrate(context.Background(), migrationdomain.Command{Operation: migrationdomain.Up})
	require.ErrorIs(t, err, cause)
	partial.Target = "primary:postgres"
	require.Equal(t, partial, result)
}

func TestMigrateRejectsNegativeUpSteps(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	projects, config, repository := mocks.NewMockProjectRepository(ctrl), mocks.NewMockDatabaseConfigResolver(ctrl), mocks.NewMockMigrationRepository(ctrl)

	_, err := New(projects, config, repository).Migrate(context.Background(), migrationdomain.Command{
		Operation: migrationdomain.Up,
		Steps:     -1,
	})

	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	var migrationErr *migrationdomain.Error
	require.ErrorAs(t, err, &migrationErr)
	require.Equal(t, "invalid_steps", migrationErr.Reason)
}
