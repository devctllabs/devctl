package migration

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/devctllabs/devctl/internal/domain/failure"
	migrationdomain "github.com/devctllabs/devctl/internal/domain/migration"
	"github.com/joho/godotenv"
)

// Environment resolves migration-only configuration without changing process state.
type Environment struct{}

func NewEnvironment() *Environment { return &Environment{} }

// ResolveDatabaseURL preserves key presence and validates a backend-specific URL.
func (e *Environment) ResolveDatabaseURL(ctx context.Context, config migrationdomain.DatabaseConfig) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("ctx.Err: %w", err)
	}
	var values map[string]string
	if config.EnvFile != "" {
		var err error
		values, err = godotenv.Read(config.EnvFile)
		if err != nil {
			return "", &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "env_file_invalid", Path: config.EnvFile, Cause: err}
		}
	}
	value, exists := os.LookupEnv(config.EnvKey)
	if !exists {
		value, exists = values[config.EnvKey]
	}
	if !exists {
		value = config.Default
	}
	if value == "" {
		reason := "database_url_missing"
		if exists {
			reason = "database_url_empty"
		}
		return "", &migrationdomain.Error{Kind: failure.InvalidInput, Reason: reason, EnvKey: config.EnvKey}
	}
	parsed, err := url.Parse(value)
	if err != nil || !matchesKind(parsed, config.Kind) {
		return "", &migrationdomain.Error{Kind: failure.InvalidInput, Reason: "database_url_invalid", EnvKey: config.EnvKey, Cause: err}
	}
	return value, nil
}

func matchesKind(value *url.URL, kind string) bool {
	if value == nil {
		return false
	}
	if kind == "postgres" {
		return value.Scheme == "postgres" || value.Scheme == "postgresql"
	}
	return (kind == "sqlite" || kind == "clickhouse") && value.Scheme == kind
}
