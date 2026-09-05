package project

import (
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
)

func TestDefaultClickHouseVariantPlansIndependentMigrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		command        projectdomain.AddDBCommand
		wantMigrations *projectdomain.DBMigrations
	}{
		{
			name:    "default path",
			command: projectdomain.AddDBCommand{Name: "analytics", Kind: "clickhouse"},
			wantMigrations: &projectdomain.DBMigrations{
				Path: "migrations/analytics/clickhouse", DatabaseEnv: "DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL",
			},
		},
		{
			name:    "explicit path",
			command: projectdomain.AddDBCommand{Name: "analytics", Kind: "clickhouse", MigrationsPath: "db/analytics"},
			wantMigrations: &projectdomain.DBMigrations{
				Path: "db/analytics", DatabaseEnv: "DB_ANALYTICS_CLICKHOUSE_MIGRATIONS_URL",
			},
		},
		{
			name:    "opt out",
			command: projectdomain.AddDBCommand{Name: "analytics", Kind: "clickhouse", NoMigrations: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			variant := defaultDBVariant(test.command)
			require.Equal(t, "clickhouse://localhost:9000/default", variant.DSNDefault)
			require.Equal(t, "DB_ANALYTICS_CLICKHOUSE_DSN", variant.DSNEnv)
			require.True(t, variant.Secret)
			require.Equal(t, test.wantMigrations, variant.Migrations)
		})
	}
}
