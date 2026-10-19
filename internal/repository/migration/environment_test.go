package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/stretchr/testify/require"
)

func TestMigrationURLPrecedenceAndPresence(t *testing.T) {
	key := "DEVCTL_MIGRATION_TEST_URL"
	fileOnlyKey := "DEVCTL_MIGRATION_FILE_ONLY_TEST"
	before, beforeExists := os.LookupEnv(fileOnlyKey)
	file := filepath.Join(t.TempDir(), "migration.env")
	require.NoError(t, os.WriteFile(file, []byte(key+"='postgres://file/db'\n"+fileOnlyKey+"=from-file\n"), 0o600))
	config := migrationdomain.DatabaseConfig{Kind: "postgres", EnvKey: key, EnvFile: file, Default: "postgres://default/db"}
	environment := NewEnvironment()
	t.Setenv(key, "postgres://process/db")
	value, err := environment.ResolveDatabaseURL(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, "postgres://process/db", value)
	t.Setenv(key, "")
	_, err = environment.ResolveDatabaseURL(context.Background(), config)
	require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
	var configErr *migrationdomain.Error
	require.ErrorAs(t, err, &configErr)
	require.Equal(t, "database_url_empty", configErr.Reason)
	require.NoError(t, os.Unsetenv(key))
	value, err = environment.ResolveDatabaseURL(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, "postgres://file/db", value)
	after, afterExists := os.LookupEnv(fileOnlyKey)
	require.Equal(t, beforeExists, afterExists)
	require.Equal(t, before, after)
	config.EnvFile = ""
	value, err = environment.ResolveDatabaseURL(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, config.Default, value)
	config.Default = ""
	_, err = environment.ResolveDatabaseURL(context.Background(), config)
	require.ErrorAs(t, err, &configErr)
	require.Equal(t, "database_url_missing", configErr.Reason)
}

func TestMigrationURLRejectsInvalidSourcesWithoutLeakingSecrets(t *testing.T) {
	t.Setenv("DEVCTL_MIGRATION_TEST_URL", "postgres://process/db")
	for _, content := range []string{"DEVCTL_MIGRATION_TEST_URL=", "DEVCTL_MIGRATION_TEST_URL=clickhouse://secret@host/db", "DEVCTL_MIGRATION_TEST_URL=postgres://user:SECRET_TOKEN@host/%zz", "invalid SECRET_TOKEN ?"} {
		file := filepath.Join(t.TempDir(), "migration.env")
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
		require.NoError(t, os.Unsetenv("DEVCTL_MIGRATION_TEST_URL"))
		_, err := NewEnvironment().ResolveDatabaseURL(context.Background(), migrationdomain.DatabaseConfig{Kind: "postgres", EnvKey: "DEVCTL_MIGRATION_TEST_URL", EnvFile: file, Default: "postgres://default/db"})
		require.Error(t, err)
		require.Equal(t, failure.InvalidInput, failure.CategoryOf(err))
		require.NotContains(t, err.Error(), "SECRET_TOKEN")
		require.NotContains(t, err.Error(), "secret@host")
	}
	t.Setenv("DEVCTL_MIGRATION_TEST_URL", "postgres://process/db")
	_, err := NewEnvironment().ResolveDatabaseURL(context.Background(), migrationdomain.DatabaseConfig{Kind: "postgres", EnvFile: filepath.Join(t.TempDir(), "missing.env"), Default: "postgres://default/db"})
	require.Error(t, err)
}

func TestMigrationURLPreservesSQLitePathsAndPostgresAlias(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ kind, value string }{
		{"sqlite", "sqlite://./data/app.db?_pragma=foreign_keys%281%29"},
		{"postgres", "postgresql://user:password@host/db"},
		{"clickhouse", "clickhouse://host/db?x-multi-statement=true"},
	} {
		value, err := NewEnvironment().ResolveDatabaseURL(context.Background(), migrationdomain.DatabaseConfig{Kind: test.kind, EnvKey: "DEVCTL_UNUSED_MIGRATION_URL", Default: test.value})
		require.NoError(t, err)
		require.Equal(t, test.value, value)
	}
}
