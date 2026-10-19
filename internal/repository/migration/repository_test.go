package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	upstream "github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"
)

func sqliteExecution(t *testing.T) migrationdomain.Execution {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "migrations"), 0o755))
	for name, sql := range map[string]string{
		"1_create_entries.up.sql":   "CREATE TABLE entries (id INTEGER PRIMARY KEY);",
		"1_create_entries.down.sql": "DROP TABLE entries;",
		"2_create_orders.up.sql":    "CREATE TABLE orders (id INTEGER PRIMARY KEY);",
		"2_create_orders.down.sql":  "DROP TABLE orders;",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "migrations", name), []byte(sql), 0o644))
	}
	return migrationdomain.Execution{Root: root, Target: projectdomain.MigrationTarget{Connection: "primary", Variant: "sqlite", Kind: "sqlite", Path: "migrations"}, DatabaseURL: "sqlite://" + filepath.Join(root, "app.db"), Operation: migrationdomain.Status, Steps: 1}
}

func TestSQLiteMigrationLifecycle(t *testing.T) {
	t.Parallel()
	execution := sqliteExecution(t)
	repository := NewRepository()
	result, err := repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.Equal(t, &migrationdomain.State{}, result.State)
	execution.Operation, execution.Steps = migrationdomain.Up, 0
	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.NotNil(t, result.State)
	require.Equal(t, uint(2), *result.State.Version)
	require.False(t, result.State.Dirty)
	db, err := sql.Open("sqlite", filepath.Join(execution.Root, "app.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT count(*) FROM orders").Scan(&count))
	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.True(t, result.NoChange)
	require.False(t, result.Changed)
	execution.Operation, execution.Steps = migrationdomain.Down, 1
	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.Equal(t, uint(1), *result.State.Version)
	require.Error(t, db.QueryRowContext(context.Background(), "SELECT count(*) FROM orders").Scan(&count))
	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.Nil(t, result.State.Version)
	_, err = repository.Execute(context.Background(), execution)
	require.Error(t, err)
}

func TestSQLiteMigrationUpSteps(t *testing.T) {
	t.Parallel()
	execution := sqliteExecution(t)
	execution.Operation, execution.Steps = migrationdomain.Up, 1
	repository := NewRepository()

	result, err := repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.Equal(t, uint(1), *result.State.Version)
	require.True(t, result.Changed)

	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.Equal(t, uint(2), *result.State.Version)
	require.True(t, result.Changed)

	execution.Steps = 0
	result, err = repository.Execute(context.Background(), execution)
	require.NoError(t, err)
	require.True(t, result.NoChange)
	require.False(t, result.Changed)

	execution = sqliteExecution(t)
	execution.Operation, execution.Steps = migrationdomain.Up, 5
	result, err = repository.Execute(context.Background(), execution)
	require.Error(t, err)
	var shortage upstream.ErrShortLimit
	require.ErrorAs(t, err, &shortage)
	require.Equal(t, uint(2), *result.State.Version)
	require.True(t, result.Changed)
}

func TestSQLiteDirtyAndPartialRollbackResults(t *testing.T) {
	t.Parallel()
	t.Run("failed SQL", func(t *testing.T) {
		t.Parallel()
		execution := sqliteExecution(t)
		require.NoError(t, os.WriteFile(filepath.Join(execution.Root, "migrations/2_create_orders.up.sql"), []byte("INVALID_SQL SUPER_SECRET_TOKEN"), 0o644))
		execution.Operation, execution.Steps = migrationdomain.Up, 0
		result, err := NewRepository().Execute(context.Background(), execution)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SUPER_SECRET_TOKEN")
		require.NotNil(t, result.State)
		require.Equal(t, uint(2), *result.State.Version)
		require.True(t, result.State.Dirty)
		require.True(t, result.Changed)
		result, err = NewRepository().Execute(context.Background(), execution)
		require.Equal(t, failure.Conflict, failure.CategoryOf(err))
		require.False(t, result.Changed)
		var dirty upstream.ErrDirty
		require.ErrorAs(t, err, &dirty)
		execution.Operation = migrationdomain.Status
		result, err = NewRepository().Execute(context.Background(), execution)
		require.NoError(t, err)
		require.True(t, result.State.Dirty)
	})
	t.Run("short rollback", func(t *testing.T) {
		t.Parallel()
		execution := sqliteExecution(t)
		execution.Operation = migrationdomain.Up
		_, err := NewRepository().Execute(context.Background(), execution)
		require.NoError(t, err)
		execution.Operation, execution.Steps = migrationdomain.Down, 5
		result, err := NewRepository().Execute(context.Background(), execution)
		require.Error(t, err)
		var shortage upstream.ErrShortLimit
		require.ErrorAs(t, err, &shortage)
		require.True(t, result.Changed)
		require.Nil(t, result.State.Version)
	})
}

func TestMigrationMissingEmptyAndCancelledSources(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing", "empty", "cancelled", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			execution := sqliteExecution(t)
			execution.Operation = migrationdomain.Up
			ctx := context.Background()
			switch mode {
			case "missing":
				execution.Target.Path = "missing"
			case "empty":
				execution.Target.Path = "empty"
				require.NoError(t, os.Mkdir(filepath.Join(execution.Root, "empty"), 0o755))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "symlink":
				execution.Target.Path = "outside"
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(execution.Root, "outside")))
			}
			_, err := NewRepository().Execute(ctx, execution)
			require.Error(t, err)
			if mode == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if mode != "empty" {
				_, statErr := os.Stat(filepath.Join(execution.Root, "app.db"))
				require.ErrorIs(t, statErr, os.ErrNotExist)
			}
		})
	}
}

func TestCreateMigrationPairAndRejectVersionCollision(t *testing.T) {
	t.Parallel()
	repository := NewRepository()
	repository.now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("offset", 3600)) }
	creation := migrationdomain.Creation{Root: t.TempDir(), Target: projectdomain.MigrationTarget{Path: "db/migrations"}, Name: "create_orders"}
	result, err := repository.Create(context.Background(), creation)
	require.NoError(t, err)
	require.Equal(t, []string{"db/migrations/20261019110000_create_orders.up.sql", "db/migrations/20261019110000_create_orders.down.sql"}, result.Files)
	for _, file := range result.Files {
		data, readErr := os.ReadFile(filepath.Join(creation.Root, file))
		require.NoError(t, readErr)
		require.Empty(t, data)
	}
	creation.Name = "another_name"
	_, err = repository.Create(context.Background(), creation)
	require.Equal(t, failure.Conflict, failure.CategoryOf(err))
	entries, err := os.ReadDir(filepath.Join(creation.Root, creation.Target.Path))
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

func TestCreateMigrationCleansUpFirstFileAndRejectsEscapes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := NewRepository()
	repository.now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	creation := migrationdomain.Creation{Root: root, Target: projectdomain.MigrationTarget{Path: "migrations"}, Name: "create_orders"}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "migrations/20261019120000_create_orders.down.sql"), 0o755))
	_, err := repository.Create(context.Background(), creation)
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(root, "migrations/20261019120000_create_orders.up.sql"))
	require.ErrorIs(t, err, os.ErrNotExist)
	creation.Target.Path = "../outside"
	_, err = repository.Create(context.Background(), creation)
	require.Error(t, err)
	creation.Target.Path = "linked"
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "linked")))
	_, err = repository.Create(context.Background(), creation)
	require.Error(t, err)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = repository.Create(cancelled, creation)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConcurrentCreatesReserveOneMigrationVersion(t *testing.T) {
	t.Parallel()
	repository := NewRepository()
	repository.now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	creation := migrationdomain.Creation{Root: t.TempDir(), Target: projectdomain.MigrationTarget{Path: "migrations"}}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, name := range []string{"one", "two"} {
		go func() {
			<-start
			input := creation
			input.Name = name
			_, err := repository.Create(context.Background(), input)
			results <- err
		}()
	}
	close(start)
	succeeded := 0
	for range 2 {
		if err := <-results; err != nil {
			require.Equal(t, failure.Conflict, failure.CategoryOf(err))
		} else {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded)
	entries, err := os.ReadDir(filepath.Join(creation.Root, "migrations"))
	require.NoError(t, err)
	require.Len(t, entries, 2)
}
